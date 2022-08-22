// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/easysoft/qucheng-operator/pkg/logging"

	dbmanage "github.com/easysoft/qucheng-operator/pkg/db/manage"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

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
	"github.com/sirupsen/logrus"
)

const (
	controllerName     = "db-controller"
	minRequeueDuration = time.Second * 5
)

func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetEventRecorderFor(controllerName)
	return &DbReconciler{
		Logger:        logging.DefaultLogger().WithField("controller", controllerName),
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
	// Watch for changes to Db
	err = c.Watch(&source.Kind{Type: &quchengv1beta1.Db{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		DeleteFunc: func(event event.DeleteEvent) bool {
			obj := event.Object.(*quchengv1beta1.Db)
			logrus.Infof("receive db delete %+v", obj)
			e := recycleDB(mgr.GetClient(), obj)
			fmt.Printf("recycle db failed, %v", e)
			return e != nil
		},
	})
	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &DbReconciler{}

// DbReconciler reconciles a Db object
type DbReconciler struct {
	client.Client
	Logger        *logrus.Entry
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GlobalDB object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *DbReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Logger.Infof("start reconcile for db %s", req.Name)
	// fetch db
	db := &quchengv1beta1.Db{}
	err := r.Get(context.TODO(), req.NamespacedName, db)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("failed to get db %s: %v", req.NamespacedName.Name, err)
		}
		r.Logger.Errorf("db %s not found, err: %v", req.NamespacedName.Name, err)
		return reconcile.Result{}, nil
	}

	original := db.DeepCopy()
	db.Status.Ready = false
	db.Status.Auth = false
	db.Status.Network = false

	m, dbMeta, err := dbmanage.ParseDB(ctx, r.Client, db)
	if err != nil {
		return ctrl.Result{}, r.compareAndPatchStatus(ctx, db, original)
	}
	db.Status.Network = true

	if err = m.IsValid(dbMeta); err != nil {
		r.Logger.WithError(err).Error("db auth invalid")
		if err = m.CreateDB(dbMeta); err != nil {
			return ctrl.Result{}, r.compareAndPatchStatus(ctx, db, original)
		}

	}

	db.Status.Address = m.ServerInfo().Address()
	db.Status.Auth = true
	db.Status.Ready = true
	return ctrl.Result{RequeueAfter: 12 * minRequeueDuration}, r.compareAndPatchStatus(ctx, db, original)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DbReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&quchengv1beta1.DbService{}).
		Complete(r)
}

func (r *DbReconciler) compareAndPatchStatus(ctx context.Context, obj, origin *quchengv1beta1.Db) error {
	if reflect.DeepEqual(obj, origin) {
		return nil
	}
	return r.Status().Patch(ctx, obj, client.MergeFrom(origin))
}
