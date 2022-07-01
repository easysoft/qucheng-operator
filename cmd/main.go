package main

import (
	"context"
	"flag"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"time"

	"github.com/sirupsen/logrus"
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
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(quchengv1beta1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

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

	ctx, cancelFunc := context.WithCancel(context.Background())

	config := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "8041c7a8.easycorp.io",
	})

	c, err := clientset.NewForConfig(config)
	if err != nil {
		panic(err)
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
