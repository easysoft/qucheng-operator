// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	"github.com/easysoft/qucheng-operator/controllers/base"
	"github.com/easysoft/qucheng-operator/controllers/qucheng"
	clientset "github.com/easysoft/qucheng-operator/pkg/client/clientset/versioned"
	informers "github.com/easysoft/qucheng-operator/pkg/client/informers/externalversions"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	restConfigQPS   = flag.Int("rest-config-qps", 30, "QPS of rest config.")
	restConfigBurst = flag.Int("rest-config-burst", 50, "Burst of rest config.")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(quchengv1beta1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr, pprofAddr, probeAddr string
	var enableLeaderElection, enablePprof bool
	var leaderElectionNamespace string
	var syncPeriodStr string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "cne-system",
		"This determines the namespace in which the leader election configmap will be created, it will use in-cluster namespace if empty.")
	flag.BoolVar(&enablePprof, "enable-pprof", true, "Enable pprof for controller manager.")
	flag.StringVar(&pprofAddr, "pprof-addr", ":8090", "The address the pprof binds to.")
	flag.StringVar(&syncPeriodStr, "sync-period", "", "Determines the minimum frequency at which watched resources are reconciled.")

	logger := logrus.New()
	logger.SetOutput(os.Stdout)
	logger.SetFormatter(&logrus.TextFormatter{
		ForceQuote:       true,
		TimestampFormat:  time.RFC3339,
		FullTimestamp:    true,
		QuoteEmptyFields: true,
	})

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if enablePprof {
		go func() {
			if err := http.ListenAndServe(pprofAddr, nil); err != nil {
				setupLog.Error(err, "unable to start pprof")
			}
		}()
	}

	ctx, cancelFunc := context.WithCancel(context.Background())

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
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      metricsAddr,
		Port:                    9443,
		HealthProbeBindAddress:  probeAddr,
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "8041c7a8.easycorp.io",
		LeaderElectionNamespace: leaderElectionNamespace,
		SyncPeriod:              syncPeriod,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(2)
	}

	c, err := clientset.NewForConfig(config)
	if err != nil {
		setupLog.Error(err, "unable to create clientset")
		os.Exit(2)
	}
	kbClient := mgr.GetClient()
	sharedInformerFactory := informers.NewSharedInformerFactory(c, 0)

	controllers := make(map[string]base.Controller)
	quchenginf := sharedInformerFactory.Qucheng().V1beta1()
	b := qucheng.NewBackupController(quchenginf.Backups(), kbClient, c, logger)
	r := qucheng.NewRestoreController(quchenginf.Restores(), kbClient, c, logger)

	controllers["backup"] = b
	controllers["restore"] = r

	sharedInformerFactory.Start(ctx.Done())

	for name, controller := range controllers {
		go func(n string, c base.Controller) {
			logger.Infof("start controller %s", n)
			err = c.Run(ctx, 4)
			if err != nil {
				cancelFunc()
			}
		}(name, controller)
	}

	logger.Infoln("start mgr")
	err = mgr.Start(ctx)
	if err != nil {
		logger.WithError(err).Error("mgr stopped")
		cancelFunc()
		os.Exit(2)
	}
}

func setRestConfig(c *rest.Config) {
	if *restConfigQPS > 0 {
		c.QPS = float32(*restConfigQPS)
	}
	if *restConfigBurst > 0 {
		c.Burst = *restConfigBurst
	}
}
