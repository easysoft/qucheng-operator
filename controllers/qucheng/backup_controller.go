/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package qucheng

import (
	"context"
	"fmt"
	"gitlab.zcorp.cc/pangu/cne-operator/controllers/base"
	quchenginformers "gitlab.zcorp.cc/pangu/cne-operator/pkg/client/informers/externalversions/qucheng/v1beta1"
	"gitlab.zcorp.cc/pangu/cne-operator/pkg/db/mysql"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	quchengv1beta1 "gitlab.zcorp.cc/pangu/cne-operator/apis/qucheng/v1beta1"
)

// BackupReconciler reconciles a Backup object
type BackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=backups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=backups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=backups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Backup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	instance := &quchengv1beta1.Backup{}
	objKey := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	err := r.Get(ctx, objKey, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	dbs, err := r.filterDbList(instance.Namespace, instance.Spec.Selector)

	for _, o := range dbs.Items {
		p := mysql.NewParser(r.Client, &o)
		klog.Infoln(p.ParseAccessInfo())
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&quchengv1beta1.Backup{}).
		Complete(r)
}

func (r *BackupReconciler) filterDbList(namespace string, selector client.MatchingLabels) (*quchengv1beta1.DbList, error) {
	list := quchengv1beta1.DbList{}

	ns := client.InNamespace(namespace)
	err := r.List(context.TODO(), &list, ns, selector)
	return &list, err
}

type BackupController struct {
	*base.GenericController
	informer quchenginformers.BackupInformer
}

func NewBackupController(informer quchenginformers.BackupInformer) base.Controller {
	c := &BackupController{
		GenericController: base.NewGenericController(backupControllerName),
	}

	informer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				backup := obj.(*quchengv1beta1.Backup)

				switch backup.Status.Phase {
				case "", quchengv1beta1.BackupPhaseNew:
					// only process new backups
				default:
					fmt.Println("Backup is not new, skipping")
					//c.logger.WithFields(logrus.Fields{
					//	"backup": kubeutil.NamespaceAndName(backup),
					//	"phase":  backup.Status.Phase,
					//}).Debug("Backup is not new, skipping")
					return
				}

				key, err := cache.MetaNamespaceKeyFunc(backup)
				if err != nil {
					fmt.Println("Error creating queue key, item not added to queue")
					return
				}
				c.Queue.Add(key)
				fmt.Println("added", key)
			},
		},
	)

	c.SyncHandler = c.process
	return c
}

func (c *BackupController) process(key string) error {
	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}
	fmt.Println(ns, name)
	return nil
}
