package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/bombsimon/logrusr"
	"github.com/vmware-tanzu/velero/pkg/restic"
	"github.com/vmware-tanzu/velero/pkg/util/filesystem"
	"gitlab.zcorp.cc/pangu/cne-operator/pkg/credentials"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	veleroctrls "github.com/vmware-tanzu/velero/pkg/controller"
	veleroclientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	veleroinformers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions"
	quchengv1beta1 "gitlab.zcorp.cc/pangu/cne-operator/apis/qucheng/v1beta1"
	"gitlab.zcorp.cc/pangu/cne-operator/controllers/base"
	"gitlab.zcorp.cc/pangu/cne-operator/controllers/qucheng"
	clientset "gitlab.zcorp.cc/pangu/cne-operator/pkg/client/clientset/versioned"
	informers "gitlab.zcorp.cc/pangu/cne-operator/pkg/client/informers/externalversions"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	scheme               = runtime.NewScheme()
	setupLog             = ctrl.Log.WithName("setup")
	metricsAddr          string
	enableLeaderElection bool
	probeAddr            string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(quchengv1beta1.AddToScheme(scheme))
	utilruntime.Must(velerov1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	s, err := newServer()
	if err != nil {
		s.cancelFunc()
		panic(err)
	}

	resticRepoRec := veleroctrls.NewResticRepoReconciler("cne-system", s.logger, s.mgr.GetClient(), restic.DefaultMaintenanceFrequency, s.resticManager)
	if err = resticRepoRec.SetupWithManager(s.mgr); err != nil {
		panic(err)
	}

	controllers := make(map[string]base.Controller)
	b := qucheng.NewBackupController(s.quickonInf.Qucheng().V1beta1().Backups(), s.mgr.GetClient(), s.quickonClient, s.logger)
	r := qucheng.NewRestoreController(s.quickonInf.Qucheng().V1beta1().Restores(), s.mgr.GetClient(), s.quickonClient, s.logger)

	controllers["backup"] = b
	controllers["restore"] = r

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

func newServer() (*server, error) {
	namespace := "cne-system"

	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetFormatter(&logrus.TextFormatter{
		ForceQuote:       true,
		TimestampFormat:  time.RFC3339,
		FullTimestamp:    true,
		QuoteEmptyFields: true,
	})

	ctrl.SetLogger(logrusr.NewLogger(logger))

	config := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "8041c7a8.easycorp.io",
	})

	quchengClient, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	veleroClient, err := veleroclientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	kbClient, _ := kubernetes.NewForConfig(config)

	ctx, cancelFunc := context.WithCancel(context.Background())
	s := &server{
		ctx: ctx, cancelFunc: cancelFunc,
		namespace:        namespace,
		kubeClientConfig: config, kubeClient: kbClient,
		quickonClient: quchengClient, veleroClient: veleroClient,
		veleroInf:  veleroinformers.NewSharedInformerFactoryWithOptions(veleroClient, 0, veleroinformers.WithNamespace(namespace)),
		quickonInf: informers.NewSharedInformerFactory(quchengClient, 0),
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
