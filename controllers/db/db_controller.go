// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	"github.com/easysoft/qucheng-operator/controllers/db/util"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero/pkg/util/kube"
)

const (
	controllerName             = "db-controller"
)

func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetEventRecorderFor(controllerName)
	return &DbReconciler{
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
	// Watch for changes to DbService
	err = c.Watch(&source.Kind{Type: &quchengv1beta1.DbService{}}, &handler.EnqueueRequestForObject{})
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
		db = nil
	}
	if db == nil || db.DeletionTimestamp != nil {
		// TODO clean db
		r.Logger.Infof("db %s is deleted", req.Name)
		return reconcile.Result{}, nil
	}

	// fetch dbsvc
	dbsvc := &quchengv1beta1.DbService{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: db.Spec.TargetService.Name, Namespace: db.Spec.TargetService.Namespace}, dbsvc)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("failed to get dbsvc %s: %v", db.Name, err)
		}
		dbsvc = nil
	}

	if dbsvc == nil {
		// TODO dbsvc not found
		r.Logger.Warnf("dbsvc %s not found, should create one", db.Spec.TargetService.Name)
		return reconcile.Result{}, nil
	}

	if err := r.FakeUserPass(dbsvc); err != nil {
		r.Logger.Errorf("failed to fake root user pass: %v", err)
		return reconcile.Result{}, err
	}

	if err := r.FakeChildUserPass(db); err != nil {
		r.Logger.Errorf("failed to fake child user pass: %v", err)
		return reconcile.Result{}, err
	}

	dbmeta := util.DBMeta{
		Address:   dbsvc.Status.Address,
		RootUser:  dbsvc.Spec.Account.User.Value,
		RootPass:  dbsvc.Spec.Account.Password.Value,
		ChildName: db.Spec.DbName,
		ChildUser: db.Spec.Account.User.Value,
		ChildPass: db.Spec.Account.Password.Value,
	}

	if err := r.updateDbStatus(db, dbmeta); err != nil {
		return reconcile.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DbReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&quchengv1beta1.DbService{}).
		Complete(r)
}

func (r *DbReconciler) FakeUserPass(dbsvc *quchengv1beta1.DbService) error {
	if dbsvc.Spec.Account.User.Value == "" {
		user, err := kube.GetSecretKey(r.Client, dbsvc.Namespace, &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: dbsvc.Spec.Account.Password.ValueFrom.SecretKeyRef.Name,
			},
			Key: dbsvc.Spec.Account.User.ValueFrom.SecretKeyRef.Key,
		})
		if err != nil {
			return err
		}
		dbsvc.Spec.Account.User.Value = string(user)
	}
	if dbsvc.Spec.Account.Password.Value == "" {
		password, err := kube.GetSecretKey(r.Client, dbsvc.Namespace, &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: dbsvc.Spec.Account.Password.ValueFrom.SecretKeyRef.Name,
			},
			Key: dbsvc.Spec.Account.Password.ValueFrom.SecretKeyRef.Key,
		})
		if err != nil {
			return err
		}
		dbsvc.Spec.Account.Password.Value = string(password)
	}
	return nil
}

func (r *DbReconciler) FakeChildUserPass(db *quchengv1beta1.Db) error {
	if db.Spec.Account.User.Value == "" {
		user, err := kube.GetSecretKey(r.Client, db.Namespace, &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: db.Spec.Account.Password.ValueFrom.SecretKeyRef.Name,
			},
			Key: db.Spec.Account.User.ValueFrom.SecretKeyRef.Key,
		})
		if err != nil {
			return err
		}
		db.Spec.Account.User.Value = string(user)
	}
	if db.Spec.Account.Password.Value == "" {
		password, err := kube.GetSecretKey(r.Client, db.Namespace, &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: db.Spec.Account.Password.ValueFrom.SecretKeyRef.Name,
			},
			Key: db.Spec.Account.Password.ValueFrom.SecretKeyRef.Key,
		})
		if err != nil {
			return err
		}
		db.Spec.Account.Password.Value = string(password)
	}
	return nil
}

func (r *DbReconciler) updateDbStatus(db *quchengv1beta1.Db, dbmeta util.DBMeta) error {
	// update db status
	var dbstatus quchengv1beta1.DbStatus

	// check network
	dbtool := util.NewDB(dbmeta)
	if dbtool == nil {
		dbstatus.Auth = false
		dbstatus.Ready = false
		r.EventRecorder.Eventf(db, corev1.EventTypeWarning, "NotSupport", "Not NotSupport %v", dbmeta.Type)
	} else {
		dbstatus.Address = dbmeta.Address
		dbstatus.Network = true
		if err := dbtool.CheckChildAuth(); err != nil {
			dbstatus.Auth = false
			dbstatus.Ready = false
			r.EventRecorder.Eventf(db, corev1.EventTypeWarning, "AuthFailed", "Failed to auth %s for %v", dbstatus.Address, err)
		} else {
			dbstatus.Auth = true
			dbstatus.Ready = true
			r.EventRecorder.Eventf(db, corev1.EventTypeNormal, "Success", "Success to check %s network & auth", dbstatus.Address)
		}
	}
	db.Status = dbstatus
	return r.Status().Update(context.TODO(), db)
}
