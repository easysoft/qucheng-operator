// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package dbservice

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	controllerName             = "dbservice-controller"
	gdbCreationDelayAfterReady = time.Second * 30
	minRequeueDuration         = time.Second * 5
)

func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetEventRecorderFor(controllerName)
	return &DbServiceReconciler{
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
	r.Logger.Info("start reconcile for dbsvc")
	// fetch dbsvc
	dbsvc := &quchengv1beta1.DbService{}
	err := r.Get(ctx, req.NamespacedName, dbsvc)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("failed to get dbsvc %s: %v", req.NamespacedName.Name, err)
		}
		dbsvc = nil
	}
	if dbsvc == nil || dbsvc.DeletionTimestamp != nil {
		r.Logger.Info("dbsvc is deleted")
		return reconcile.Result{}, nil
	}

	// if dbsvc is not exist, we should create it
	// if dbsvc.Spec.State == "new" || dbsvc.Spec.Service.Name == "" {
	// 	r.Logger.Infof("dbsvc %s is new will create", dbsvc.Name)
	// 	if len(dbsvc.Spec.Account.Password.Value) == 0 {
	// 		// TODO gen password
	// 		dbsvc.Spec.Account.Password.Value = "password"
	// 	}
	// 	if isReady, delay := r.getDbServiceReadyAndDelaytime(dbsvc); !isReady {
	// 		r.Logger.Infof("skip for dbsvc %s has not ready yet.", req.Name)
	// 		return reconcile.Result{}, nil
	// 	} else if delay > 0 {
	// 		r.Logger.Infof("skip for dbsvc %s waiting for ready %s.", req.Name, delay)
	// 		return reconcile.Result{RequeueAfter: delay}, nil
	// 	}
	// 	return reconcile.Result{}, nil
	// }

	if err := r.updateDbServiceStatus(dbsvc); err != nil {
		return reconcile.Result{}, err
	}
	return ctrl.Result{RequeueAfter: minRequeueDuration}, nil
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

func (r *DbServiceReconciler) updateDbService(dbsvc *quchengv1beta1.DbService) error {
	dbsvc.Spec.Service = quchengv1beta1.Service{
		Name:      fmt.Sprintf("%s-%s", dbsvc.Name, dbsvc.Spec.Type),
		Namespace: dbsvc.Namespace,
		Port:      intstr.FromInt(3306), // TODO
	}
	dbsvc.Spec.Account = quchengv1beta1.Account{
		User: quchengv1beta1.AccountUser{
			Value: "root",
		},
		Password: quchengv1beta1.AccountPassword{
			Value: "password",
		},
	}
	// dbsvc.Spec.State = "exist"
	return r.Update(context.TODO(), dbsvc)
}

func (r *DbServiceReconciler) updateDbServiceStatus(dbsvc *quchengv1beta1.DbService) error {
	// update dbsvc status
	var dbsvcstatus quchengv1beta1.DbServiceStatus

	// check network
	dbtool := util.NewDB(dbsvc)
	if dbtool == nil {
		dbsvcstatus.Auth = false
		dbsvcstatus.Ready = false
		r.EventRecorder.Eventf(dbsvc, corev1.EventTypeWarning, "NotSupport", "Not NotSupport %v", dbsvc.Spec.Type)
	} else {
		dbsvcstatus.Address = fmt.Sprintf("%s:%s", dbtool.GenHost(), dbtool.GenPort())
		if err := dbtool.CheckNetWork(); err != nil {
			dbsvcstatus.Network = false
			dbsvcstatus.Ready = false
			r.EventRecorder.Eventf(dbsvc, corev1.EventTypeWarning, "NetworkUnreachable", "Failed to conn %s for %v", dbsvcstatus.Address, err)
		} else {
			dbsvcstatus.Network = true
			if err := r.FakeUserPass(dbsvc); err != nil {
				r.Logger.Errorf("fake user pass error for %v", err)
			}
			if err := dbtool.CheckAuth(); err != nil {
				dbsvcstatus.Auth = false
				dbsvcstatus.Ready = false
				r.EventRecorder.Eventf(dbsvc, corev1.EventTypeWarning, "AuthFailed", "Failed to auth %s for %v", dbsvcstatus.Address, err)
			} else {
				dbsvcstatus.Auth = true
				dbsvcstatus.Ready = true
				r.EventRecorder.Eventf(dbsvc, corev1.EventTypeNormal, "Success", "Success to check %s network & auth", dbsvcstatus.Address)
			}
		}
	}
	dbsvc.Status = dbsvcstatus
	return r.Status().Update(context.TODO(), dbsvc)
}

func (r *DbServiceReconciler) getDbServiceReadyAndDelaytime(dbsvc *quchengv1beta1.DbService) (bool, time.Duration) {
	created, done, err := r.checkOrCreateDbServiceJob(dbsvc)
	if err != nil {
		r.Logger.Errorf("check dbsvc job %s failed for %v", dbsvc.Name, err)
	}
	if !created {
		return false, 0
	}
	if !done {
		delay := gdbCreationDelayAfterReady - time.Since(dbsvc.CreationTimestamp.Time)
		if delay > 0 {
			return false, delay
		}
	}
	if err := r.updateDbService(dbsvc); err != nil {
		r.Logger.Errorf("update dbsvc %s status failed for %v", dbsvc.Name, err)
		r.EventRecorder.Eventf(dbsvc, corev1.EventTypeWarning, "UpdateStatusFailed", "Failed to update gdb %s status", dbsvc.Name)
	} else {
		r.EventRecorder.Eventf(dbsvc, corev1.EventTypeNormal, "Success", "Success to update gdb %s status", dbsvc.Name)
	}

	return true, 0
}

func (r *DbServiceReconciler) checkOrCreateDbServiceJob(dbsvc *quchengv1beta1.DbService) (bool, bool, error) {
	// create dbsvc job
	dbsvcJob := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbsvc.Name,
			Namespace: "cne-system",
		},
		Spec: batchv1.JobSpec{
			Parallelism:             ptrint32(1),
			Completions:             ptrint32(1),
			BackoffLimit:            ptrint32(1),
			TTLSecondsAfterFinished: ptrint32(120),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					ServiceAccountName: "qucheng-controller-manager",
					RestartPolicy:      corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{
						{
							Name:  dbsvc.Name,
							Image: "hub.qucheng.com/platform/helmtool:mysql",
							// Command: []string{"sleep", "36000"},
							Env: []corev1.EnvVar{
								{
									Name:  "GDB_NAME",
									Value: dbsvc.Name,
								},
								{
									Name:  "NAMESPACE",
									Value: dbsvc.Namespace,
								},
								{
									Name:  "GDB_PASS",
									Value: dbsvc.Spec.Account.Password.Value,
								},
							},
							Resources: corev1.ResourceRequirements{},
						},
					},
				},
			},
		},
	}
	if err := r.Get(context.TODO(), types.NamespacedName{Name: dbsvc.Name, Namespace: "cne-system"}, dbsvcJob); err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(context.TODO(), dbsvcJob); err != nil {
				return false, false, err
			}
			r.Logger.Infof("create dbsvc job %s success", dbsvcJob.Name)
			r.EventRecorder.Eventf(dbsvc, corev1.EventTypeNormal, "Success", "Success to create dbsvc job %s", dbsvcJob.Name)
			return true, false, nil
		}
		return false, false, err
	}
	if getDbServiceJobStatus(&dbsvcJob.Status) {
		return true, true, nil
	}
	return true, false, nil
}

func getDbServiceJobStatus(status *batchv1.JobStatus) bool {
	if status == nil {
		return false
	}
	for i := range status.Conditions {
		if status.Conditions[i].Type == batchv1.JobComplete && status.Conditions[i].Status == corev1.ConditionTrue {
			return true
		}
	}
	if status.Succeeded > 0 {
		return true
	}
	return false
}

func ptrint32(p int32) *int32 {
	return &p
}
