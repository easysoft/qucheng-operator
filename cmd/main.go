// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/easysoft/qucheng-operator/pkg/logging"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/easysoft/qucheng-operator/pkg/credentials"
	"github.com/vmware-tanzu/velero/pkg/restic"
	"github.com/vmware-tanzu/velero/pkg/util/filesystem"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	customcontrollers "github.com/easysoft/qucheng-operator/controllers"
	"github.com/easysoft/qucheng-operator/controllers/base"
	"github.com/easysoft/qucheng-operator/controllers/qucheng"
	clientset "github.com/easysoft/qucheng-operator/pkg/client/clientset/versioned"
	informers "github.com/easysoft/qucheng-operator/pkg/client/informers/externalversions"
	"github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	veleroctrls "github.com/vmware-tanzu/velero/pkg/controller"
	veleroclientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	veleroinformers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	metricsAddr, pprofAddr, probeAddr string
	enableLeaderElection, enablePprof bool
	leaderElectionNamespace           string
	syncPeriodStr                     string
	restConfigQPS                     = flag.Int("rest-config-qps", 30, "QPS of rest config.")
	restConfigBurst                   = flag.Int("rest-config-burst", 50, "Burst of rest config.")

	logLevel string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(quchengv1beta1.AddToScheme(scheme))
	utilruntime.Must(velerov1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	pflag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	pflag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	pflag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	pflag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "cne-system",
		"This determines the namespace in which the leader election configmap will be created, it will use in-cluster namespace if empty.")
	pflag.BoolVar(&enablePprof, "enable-pprof", true, "Enable pprof for controller manager.")
	pflag.StringVar(&pprofAddr, "pprof-addr", ":8090", "The address the pprof binds to.")
	pflag.StringVar(&syncPeriodStr, "sync-period", "1m", "Determines the minimum frequency at which watched resources are reconciled.")
	pflag.String(logging.FlagLogLevel, "info", "loglevel")
	viper.BindPFlag(logging.FlagLogLevel, pflag.Lookup(logging.FlagLogLevel))

	viper.BindPFlag("pod-namespace", pflag.Lookup("leader-election-namespace"))
	pflag.Parse()

	s, err := newServer(viper.GetString("pod-namespace"))
	if err != nil {
		s.cancelFunc()
		setupLog.Error(err, "init server failed")
		os.Exit(2)
	}

	if err = ensureDefaultBackupStorageLocation(s.veleroClient); err != nil {
		setupLog.Error(err, "init default bsl failed")
		os.Exit(3)
	}

	resticRepoRec := veleroctrls.NewResticRepoReconciler(leaderElectionNamespace, s.logger, s.mgr.GetClient(), restic.DefaultMaintenanceFrequency, s.resticManager)
	if err = resticRepoRec.SetupWithManager(s.mgr); err != nil {
		setupLog.Error(err, "init restic repo reconciler failed")
		os.Exit(2)
	}
	if err = qucheng.NewDbBackupReconciler(s.mgr.GetClient(), s.mgr.GetScheme(), s.logger).SetupWithManager(s.mgr); err != nil {
		setupLog.Error(err, "init dbBackup reconciler failed")
	}
	if err = qucheng.NewDbRestoreReconciler(s.mgr.GetClient(), s.mgr.GetScheme(), s.logger).SetupWithManager(s.mgr); err != nil {
		setupLog.Error(err, "init dbRestore reconciler failed")
	}

	controllers := make(map[string]base.Controller)
	b := qucheng.NewBackupController(s.ctx, s.namespace, scheme, s.quickonInf.Qucheng().V1beta1().Backups(), s.mgr.GetClient(), s.quickonClient, s.veleroClient,
		s.veleroInf, s.logger, s.resticManager)
	r := qucheng.NewRestoreController(s.ctx, s.namespace, scheme, s.quickonInf.Qucheng().V1beta1().Restores(), s.mgr.GetClient(), s.quickonClient, s.veleroClient, s.veleroInf, s.logger)

	controllers["backup"] = b
	controllers["restore"] = r

	// start informers cache
	s.veleroInf.Start(s.ctx.Done())
	s.quickonInf.Start(s.ctx.Done())

	for name, controller := range controllers {
		go func(n string, c base.Controller) {
			s.logger.Infof("start controller %s", n)
			err = c.Run(s.ctx, 4)
			if err != nil {
				s.cancelFunc()
			}
		}(name, controller)
	}

	// custom controller
	go func() {
		s.logger.Info("setup customcontrollers")
		if err = customcontrollers.SetupWithManager(s.mgr); err != nil {
			s.logger.Error(err, "unable to setup customcontrollers")
			os.Exit(1)
		}
	}()

	s.logger.Infoln("start mgr")
	err = s.mgr.Start(s.ctx)
	if err != nil {
		s.logger.WithError(err).Error("mgr stopped")
		s.cancelFunc()
		os.Exit(2)
	}
}

type server struct {
	ctx                 context.Context
	cancelFunc          context.CancelFunc
	namespace           string
	kubeClientConfig    *rest.Config
	kubeClient          kubernetes.Interface
	veleroClient        veleroclientset.Interface
	quickonClient       clientset.Interface
	veleroInf           veleroinformers.SharedInformerFactory
	quickonInf          informers.SharedInformerFactory
	mgr                 manager.Manager
	logger              logrus.FieldLogger
	logLevel            logrus.Level
	credentialFileStore credentials.FileStore
	resticManager       restic.RepositoryManager
}

func newServer(namespace string) (*server, error) {

	fmt.Printf("server namespace is %s/%s", namespace, leaderElectionNamespace)
	logger := logging.DefaultLogger()

	//opts := zap.Options{
	//	Development: true,
	//}
	//opts.BindFlags(flag.CommandLine)
	//flag.Parse()
	//
	//ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if enablePprof {
		go func() {
			if err := http.ListenAndServe(pprofAddr, nil); err != nil {
				setupLog.Error(err, "unable to start pprof")
			}
		}()
	}

	config := ctrl.GetConfigOrDie()
	setRestConfig(config)
	config.UserAgent = "qucheng-manager"
	var syncPeriod *time.Duration
	if syncPeriodStr != "" {
		d, err := time.ParseDuration(syncPeriodStr)
		if err != nil {
			setupLog.Error(err, "invalid sync period flag")
		} else {
			syncPeriod = &d
		}
	}
	logger.Infof("leader namespace is %s", namespace)
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      metricsAddr,
		Port:                    9443,
		HealthProbeBindAddress:  probeAddr,
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "8041c7a8.easycorp.io",
		LeaderElectionNamespace: namespace,
		SyncPeriod:              syncPeriod,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return nil, err
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		return nil, err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		return nil, err
	}

	quickonClient, err := clientset.NewForConfig(config)
	if err != nil {
		setupLog.Error(err, "unable to create quickon clientset")
		return nil, err

	}

	veleroClient, err := veleroclientset.NewForConfig(config)
	if err != nil {
		setupLog.Error(err, "unable to create velero clientset")
		return nil, err
	}

	kbClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		setupLog.Error(err, "unable to create kubernetes clientset")
		return nil, err
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	s := &server{
		ctx: ctx, cancelFunc: cancelFunc,
		namespace:        namespace,
		kubeClientConfig: config, kubeClient: kbClient,
		quickonClient: quickonClient, veleroClient: veleroClient,
		veleroInf:  veleroinformers.NewSharedInformerFactoryWithOptions(veleroClient, 0, veleroinformers.WithNamespace(namespace)),
		quickonInf: informers.NewSharedInformerFactory(quickonClient, 0),
		mgr:        mgr,
		logger:     logger,
	}

	credentialFileStore, err := credentials.NewNamespacedFileStore(
		mgr.GetClient(),
		namespace,
		"/tmp/credentials",
		filesystem.NewFileSystem(),
	)
	if err != nil {
		cancelFunc()
		return nil, err
	}
	s.credentialFileStore = credentialFileStore

	if err := s.initRestic(); err != nil {
		cancelFunc()
		return nil, err
	}

	return s, nil
}

func (s *server) initRestic() error {
	res, err := restic.NewRepositoryManager(
		s.ctx,
		s.namespace,
		s.veleroClient,
		s.veleroInf.Velero().V1().ResticRepositories(),
		s.veleroClient.VeleroV1(),
		s.mgr.GetClient(),
		s.kubeClient.CoreV1(),
		s.kubeClient.CoreV1(),
		s.credentialFileStore,
		s.logger,
	)
	if err != nil {
		return err
	}
	s.resticManager = res
	return nil
}

func setRestConfig(c *rest.Config) {
	if *restConfigQPS > 0 {
		c.QPS = float32(*restConfigQPS)
	}
	if *restConfigBurst > 0 {
		c.Burst = *restConfigBurst
	}
}

func ensureDefaultBackupStorageLocation(c veleroclientset.Interface) error {
	bslName := "minio"
	namespace := viper.GetString("pod-namespace")

	_, err := c.VeleroV1().BackupStorageLocations(namespace).Get(context.TODO(), bslName, metav1.GetOptions{})
	if err != nil {
		bsl := velerov1.BackupStorageLocation{
			ObjectMeta: metav1.ObjectMeta{
				Name: bslName, Namespace: namespace,
			},
			Spec: velerov1.BackupStorageLocationSpec{
				Provider: "aws",
				Config: map[string]string{
					"bucket":           "volumes",
					"profile":          "default",
					"region":           "volumes",
					"s3ForcePathStyle": "true",
					"s3Url":            "http://cne-operator-minio:9000",
				},
				Credential: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: "minio-credentials",
					},
					Key: "minio",
				},
				StorageType: velerov1.StorageType{
					ObjectStorage: &velerov1.ObjectStorageLocation{
						Bucket: "volumes",
					},
				},
				Default:             true,
				AccessMode:          "",
				BackupSyncPeriod:    nil,
				ValidationFrequency: nil,
			},
		}
		_, err = c.VeleroV1().BackupStorageLocations(namespace).Create(context.TODO(), &bsl, metav1.CreateOptions{})
	}
	return err
}
