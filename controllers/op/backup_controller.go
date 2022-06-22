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

package op

import (
	"context"
	dbv1beta1 "gitlab.zcorp.cc/pangu/cne-operator/apis/db/v1beta1"
	"gitlab.zcorp.cc/pangu/cne-operator/pkg/db/mysql"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	opv1beta1 "gitlab.zcorp.cc/pangu/cne-operator/apis/op/v1beta1"
)

// BackupReconciler reconciles a Backup object
type BackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=op.qucheng.io,resources=backups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=op.qucheng.io,resources=backups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=op.qucheng.io,resources=backups/finalizers,verbs=update

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

	instance := &opv1beta1.Backup{}
	objKey := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	err := r.Get(ctx, objKey, instance)
	if err != nil {
		return reconcile.Result{}, err
	}

	dbs, err := r.filterDbList(instance.Namespace, instance.Spec.Selector)
	klog.Infoln(dbs.Items)
	klog.Infoln(err)
	for _, o := range dbs.Items {
		p := mysql.NewParser(r.Client, &o)
		klog.Infoln(p.ParseAccessInfo())
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&opv1beta1.Backup{}).
		Complete(r)
}

func (r *BackupReconciler) filterDbList(namespace string, selector client.MatchingLabels) (*dbv1beta1.DbList, error) {
	list := dbv1beta1.DbList{}

	ns := client.InNamespace(namespace)
	err := r.List(context.TODO(), &list, ns, selector)
	return &list, err
}
