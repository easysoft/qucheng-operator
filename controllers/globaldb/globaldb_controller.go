// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package globaldb

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
	controllerName             = "globaldb-controller"
	gdbCreationDelayAfterReady = time.Second * 30
	minRequeueDuration         = time.Second * 5
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
	// if gdb is not exist, we should create it
	if gdb.Spec.State == "new" {
		r.Logger.Infof("gdb %s is new will create", gdb.Name)
		if len(gdb.Spec.Source.Pass) == 0 {
			// TODO gen password
			gdb.Spec.Source.Pass = "password"
		}
		if isReady, delay := r.getGDBReadyAndDelaytime(gdb); !isReady {
			r.Logger.Infof("skip for gdb %s has not ready yet.", req.Name)
			return reconcile.Result{}, nil
		} else if delay > 0 {
			r.Logger.Infof("skip for gdb %s waiting for ready %s.", req.Name, delay)
			return reconcile.Result{RequeueAfter: delay}, nil
		}
		return reconcile.Result{}, nil
	}
	if err := r.updateGDBStatus(gdb); err != nil {
		return reconcile.Result{}, err
	}
	return ctrl.Result{RequeueAfter: minRequeueDuration}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GlobalDBReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&quchengv1beta1.GlobalDB{}).
		Complete(r)
}

func (r *GlobalDBReconciler) updateGDB(gdb *quchengv1beta1.GlobalDB, host string) error {
	gdb.Spec.Source.Host = host
	gdb.Spec.Source.User = "root"
	gdb.Spec.Source.Port = 3306
	gdb.Spec.State = "exist"
	return r.Update(context.TODO(), gdb)
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
	gstatus.Address = fmt.Sprintf("%s:%d", gdb.Spec.Source.Host, gdb.Spec.Source.Port)
	gdb.Status = gstatus
	return r.Status().Update(context.TODO(), gdb)
}

func (r *GlobalDBReconciler) getGDBReadyAndDelaytime(gdb *quchengv1beta1.GlobalDB) (bool, time.Duration) {
	created, done, err := r.checkOrCreateGDBJob(gdb)
	if err != nil {
		r.Logger.Errorf("check gdb job %s failed for %v", gdb.Name, err)
	}
	if !created {
		return false, 0
	}
	if !done {
		delay := gdbCreationDelayAfterReady - time.Since(gdb.CreationTimestamp.Time)
		if delay > 0 {
			return false, delay
		}
	}
	if err := r.updateGDB(gdb, fmt.Sprintf("%s-mysql.%s.svc", gdb.Name, gdb.Namespace)); err != nil {
		r.Logger.Errorf("update gdb %s status failed for %v", gdb.Name, err)
		r.EventRecorder.Eventf(gdb, corev1.EventTypeWarning, "UpdateStatusFailed", "Failed to update gdb %s status", gdb.Name)
	} else {
		r.EventRecorder.Eventf(gdb, corev1.EventTypeNormal, "Success", "Success to update gdb %s status", gdb.Name)
	}

	return true, 0
}

func (r *GlobalDBReconciler) checkOrCreateGDBJob(gdb *quchengv1beta1.GlobalDB) (bool, bool, error) {
	// create gdb job
	gdbJob := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Job",
			APIVersion: "batch/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      gdb.Name,
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
							Name:  gdb.Name,
							Image: "hub.qucheng.com/platform/helmtool:mysql",
							// Command: []string{"sleep", "36000"},
							Env: []corev1.EnvVar{
								{
									Name:  "GDB_NAME",
									Value: gdb.Name,
								},
								{
									Name:  "NAMESPACE",
									Value: gdb.Namespace,
								},
								{
									Name:  "GDB_PASS",
									Value: gdb.Spec.Source.Pass,
								},
							},
							Resources: corev1.ResourceRequirements{},
						},
					},
				},
			},
		},
	}
	if err := r.Get(context.TODO(), types.NamespacedName{Name: gdb.Name, Namespace: "cne-system"}, gdbJob); err != nil {
		if errors.IsNotFound(err) {
			if err := r.Create(context.TODO(), gdbJob); err != nil {
				return false, false, err
			}
			r.Logger.Infof("create gdb job %s success", gdbJob.Name)
			r.EventRecorder.Eventf(gdb, corev1.EventTypeNormal, "Success", "Success to create gdb job %s", gdbJob.Name)
			return true, false, nil
		}
		return false, false, err
	}
	if getGDBJobStatus(&gdbJob.Status) {
		return true, true, nil
	}
	return true, false, nil
}

func getGDBJobStatus(status *batchv1.JobStatus) bool {
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
