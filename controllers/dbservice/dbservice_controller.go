// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package dbservice

import (
	"context"
	"fmt"
	"reflect"
	"time"

	dbmanage "github.com/easysoft/qucheng-operator/pkg/db/manage"
	"github.com/easysoft/qucheng-operator/pkg/logging"

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
	"github.com/easysoft/qucheng-operator/controllers/dbservice/util"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero/pkg/util/kube"
)

const (
	controllerName = "dbservice-controller"
	//gdbCreationDelayAfterReady = time.Second * 30
	minRequeueDuration = time.Second * 5
)

func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetEventRecorderFor(controllerName)
	return &DbServiceReconciler{
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
	// Watch for changes to DbService
	err = c.Watch(&source.Kind{Type: &quchengv1beta1.DbService{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &DbServiceReconciler{}

// DbServiceReconciler reconciles a DbService object
type DbServiceReconciler struct {
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
func (r *DbServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Logger.Infof("start reconcile for dbsvc: %s", req.Name)
	logger := r.Logger.WithField("key", req.NamespacedName)
	// fetch dbsvc
	dbsvc := &quchengv1beta1.DbService{}
	err := r.Get(ctx, req.NamespacedName, dbsvc)
	if err != nil {
		logger.WithError(err).Errorf("reconcile get req failed")
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("failed to get dbsvc %s: %v", req.NamespacedName.Name, err)
		}
		dbsvc = nil
	}
	if dbsvc == nil || dbsvc.DeletionTimestamp != nil {
		logger.Info("dbService is deleted")
		return reconcile.Result{}, nil
	}
	original := dbsvc.DeepCopy()

	logger.Debugln("start parse status")
	if err = r.updateDbServiceStatus(dbsvc, logger); err != nil {
		logger.WithError(err).Error("update dbService status failed")
		return reconcile.Result{}, err
	}

	if !reflect.DeepEqual(dbsvc.Status, original.Status) {
		logger.Info("status changed, update dbservice status")
		err = r.Status().Patch(context.TODO(), dbsvc, client.MergeFrom(original))
		return reconcile.Result{}, err
	}

	logger.Info("status not changed")
	return ctrl.Result{RequeueAfter: minRequeueDuration}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *DbServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&quchengv1beta1.DbService{}).
		Complete(r)
}

func (r *DbServiceReconciler) FakeUserPass(dbsvc *quchengv1beta1.DbService) error {
	if dbsvc.Spec.Account.User.Value == "" {
		user, err := kube.GetSecretKey(r.Client, dbsvc.Namespace, &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: dbsvc.Spec.Account.User.ValueFrom.SecretKeyRef.Name,
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

func (r *DbServiceReconciler) updateDbServiceStatus(dbsvc *quchengv1beta1.DbService, logger logrus.FieldLogger) error {
	// update dbsvc status
	var dbsvcstatus quchengv1beta1.DbServiceStatus

	m, err := dbmanage.ParseDbService(context.TODO(), r.Client, dbsvc)
	if err != nil {
		return err
	}

	dbsvc.Status.Auth = false
	dbsvc.Status.Network = false
	dbsvc.Status.Ready = false
	dbsvc.Status.Global = false

	// create db tool
	dbtool := util.NewDB(dbsvc)
	if dbtool == nil {
		r.EventRecorder.Eventf(dbsvc, corev1.EventTypeWarning, "NotSupport", "Not NotSupport %v", dbsvc.Spec.Type)
		return nil
	}

	dbsvc.Status.Global = util.VaildGlobalDatabase(dbsvc.Labels)
	logger.Debugf("set status.global to %v", dbsvc.Status.Global)

	// check network
	dbsvc.Status.Address = fmt.Sprintf("%s:%s", dbtool.GenHost(), dbtool.GenPort())
	if err = dbtool.CheckNetWork(); err != nil {
		r.EventRecorder.Eventf(dbsvc, corev1.EventTypeWarning, "NetworkUnreachable", "Failed to conn %s for %v", dbsvcstatus.Address, err)
		return nil
	}
	dbsvc.Status.Network = true
	logger.Debugf("set status.network to %v", dbsvc.Status.Network)

	// check auth
	err = m.IsValid(&dbmanage.DbMeta{
		User:     m.ServerInfo().AdminUser(),
		Password: m.ServerInfo().AdminPassword(),
	})
	if err != nil {
		r.EventRecorder.Eventf(dbsvc, corev1.EventTypeWarning, "AuthFailed", "Failed to auth %s for %v", dbsvcstatus.Address, err)
		return nil
	}
	dbsvc.Status.Auth = true
	dbsvc.Status.Ready = true
	logger.Debugf("set status.auth to %v", dbsvc.Status.Auth)
	logger.Debugf("set status.ready to %v", dbsvc.Status.Ready)
	r.EventRecorder.Eventf(dbsvc, corev1.EventTypeNormal, "Success", "Success to check %s network & auth", dbsvcstatus.Address)

	return nil
}
