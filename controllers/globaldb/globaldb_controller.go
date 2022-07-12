// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package globaldb

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	"github.com/easysoft/qucheng-operator/controllers/globaldb/util"
	"github.com/sirupsen/logrus"
)

const (
	controllerName = "globaldb-controller"
)

func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetEventRecorderFor(controllerName)
	return &GlobalDBReconciler{
		Logger:        logrus.New().WithField("controller", controllerName),
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: recorder,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	// Watch for changes to GlobalDB
	err = c.Watch(&source.Kind{Type: &quchengv1beta1.GlobalDB{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &GlobalDBReconciler{}

// GlobalDBReconciler reconciles a GlobalDB object
type GlobalDBReconciler struct {
	client.Client
	Logger        *logrus.Entry
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
}

//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=globaldbs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=globaldbs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=globaldbs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GlobalDB object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *GlobalDBReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// TODO(user): your logic here
	r.Logger.Info("start reconcile for gdb")
	// fetch gdb
	gdb := &quchengv1beta1.GlobalDB{}
	err := r.Get(ctx, req.NamespacedName, gdb)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("failed to get gdb %s: %v", req.NamespacedName.Name, err)
		}
		gdb = nil
	}
	if gdb == nil || gdb.DeletionTimestamp != nil {
		r.Logger.Info("gdb is deleted")
		return reconcile.Result{}, nil
	}
	if err := r.updateGDBStatus(gdb); err != nil {
		return reconcile.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GlobalDBReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&quchengv1beta1.GlobalDB{}).
		Complete(r)
}

func (r *GlobalDBReconciler) updateGDBStatus(gdb *quchengv1beta1.GlobalDB) error {
	// update gdb status
	var gstatus quchengv1beta1.GlobalDBStatus

	// check network
	dbtool := util.NewDB(gdb.Spec)
	if dbtool == nil {
		gstatus.Auth = false
		gstatus.Ready = false
		r.EventRecorder.Eventf(gdb, corev1.EventTypeWarning, "NotSupport", "Not NotSupport %v", gdb.Spec.Type)
	} else {
		if err := dbtool.CheckNetWork(); err != nil {
			gstatus.Network = false
			gstatus.Ready = false
			r.EventRecorder.Eventf(gdb, corev1.EventTypeWarning, "NetworkUnreachable", "Failed to conn %s:%v for %v", gdb.Spec.Source.Host, gdb.Spec.Source.Port, err)
		} else {
			gstatus.Network = true
			if err := dbtool.CheckAuth(); err != nil {
				gstatus.Auth = false
				gstatus.Ready = false
				r.EventRecorder.Eventf(gdb, corev1.EventTypeWarning, "AuthFailed", "Failed to auth %s:%v for %v", gdb.Spec.Source.Host, gdb.Spec.Source.Port, err)
			} else {
				gstatus.Auth = true
				gstatus.Ready = true
				r.EventRecorder.Eventf(gdb, corev1.EventTypeNormal, "Success", "Success to check %s:%v network & auth", gdb.Spec.Source.Host, gdb.Spec.Source.Port)
			}
		}
	}
	gdb.Status = gstatus
	return r.Status().Update(context.TODO(), gdb)
}
