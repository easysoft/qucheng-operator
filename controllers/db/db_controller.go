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

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	dbmanage "github.com/easysoft/qucheng-operator/pkg/db/manage"
	"github.com/easysoft/qucheng-operator/pkg/logging"
	"github.com/easysoft/qucheng-operator/pkg/util/ptrtool"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
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
			if obj.Spec.ReclaimPolicy != nil && *obj.Spec.ReclaimPolicy == quchengv1beta1.DbReclaimRetain {
				logrus.Info("remove db, but retain the database")
				return true
			}
			logrus.Infof("receive db delete %+v", obj)
			e := recycleDB(mgr.GetClient(), obj, logging.DefaultLogger())
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
	db.Status.Ready = ptrtool.Bool(false)
	db.Status.Auth = ptrtool.Bool(false)
	db.Status.Network = ptrtool.Bool(false)

	m, dbMeta, err := dbmanage.ParseDB(ctx, r.Client, db, r.Logger)
	if err != nil {
		return ctrl.Result{RequeueAfter: minRequeueDuration}, r.compareAndPatchStatus(ctx, db, original)
	}
	db.Status.Network = ptrtool.Bool(true)

	if err = m.IsValid(dbMeta); err != nil {
		r.Logger.WithError(err).Error("db auth invalid")
		if err = m.CreateDB(dbMeta); err != nil {
			r.Logger.WithError(err).Error("create db failed")
			return ctrl.Result{RequeueAfter: minRequeueDuration}, r.compareAndPatchStatus(ctx, db, original)
		}
	}

	db.Status.Address = m.ServerInfo().Address()
	db.Status.Auth = ptrtool.Bool(true)
	db.Status.Ready = ptrtool.Bool(true)
	return ctrl.Result{RequeueAfter: 60 * minRequeueDuration}, r.compareAndPatchStatus(ctx, db, original)
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
