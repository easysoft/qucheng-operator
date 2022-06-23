package main

import (
	"context"
	"gitlab.zcorp.cc/pangu/cne-operator/controllers/qucheng"
	clientset "gitlab.zcorp.cc/pangu/cne-operator/pkg/client/clientset/versioned"
	informers "gitlab.zcorp.cc/pangu/cne-operator/pkg/client/informers/externalversions"
	ctrl "sigs.k8s.io/controller-runtime"
)

func main() {
	ctx, cancelFunc := context.WithCancel(context.Background())
	config := ctrl.GetConfigOrDie()
	c, err := clientset.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	sharedInformerFactory := informers.NewSharedInformerFactory(c, 0)

	b := qucheng.NewBackupController(sharedInformerFactory.Qucheng().V1beta1().Backups())
	sharedInformerFactory.Start(ctx.Done())

	err = b.Run(ctx, 4)
	if err != nil {
		cancelFunc()
		panic(err)
	}
}
