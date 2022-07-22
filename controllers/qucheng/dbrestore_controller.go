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

	dbmanage "github.com/easysoft/qucheng-operator/pkg/db/manage"

	"github.com/easysoft/qucheng-operator/pkg/storage"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/clock"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DbRestoreReconciler reconciles a DbRestore object
type DbRestoreReconciler struct {
	client.Client
	logger logrus.FieldLogger
	clock  clock.Clock
	Scheme *runtime.Scheme
}

func NewDbRestoreReconciler(client client.Client, schema *runtime.Scheme, logger logrus.FieldLogger) *DbRestoreReconciler {
	r := DbRestoreReconciler{
		Client: client, Scheme: schema,
		logger: logger.WithField("controller", "dbrestore"), clock: clock.RealClock{},
	}
	return &r
}

//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=dbrestores,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=dbrestores/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=dbrestores/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DbRestore object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *DbRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.logger

	var dbr quchengv1beta1.DbRestore
	if err := r.Get(ctx, req.NamespacedName, &dbr); err != nil {
		if apierrors.IsNotFound(err) {
			log.Debug("Unable to find DbRestore")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrap(err, "getting DbRestore")
	}

	log = log.WithField("dbr", dbr.Name)
	r.logger = log

	switch dbr.Status.Phase {
	case "", quchengv1beta1.DbRestorePhaseNew:
		// Only process new items.
	default:
		log.Debug("DbRestore is not new, not processing")
		return ctrl.Result{}, nil
	}

	dbr.Status.Phase = quchengv1beta1.DbRestorePhaseInProgress
	dbr.Status.StartTimestamp = &metav1.Time{Time: r.clock.Now()}
	if err := r.Status().Update(ctx, &dbr); err != nil {
		log.WithError(err).Error("error updating DbRestore status")
		return ctrl.Result{}, err
	}

	var db quchengv1beta1.Db
	if err := r.Client.Get(ctx, client.ObjectKey{Name: dbr.Spec.Db.Name, Namespace: dbr.Spec.Db.Namespace}, &db); err != nil {
		return r.updateStatusToFailed(ctx, &dbr, err, "db not found", log)
	}

	m, dbMeta, err := dbmanage.ParseDB(ctx, r.Client, &db)
	if err != nil {
		return r.updateStatusToFailed(ctx, &dbr, err, "parse db access info failed", log)
	}

	store := storage.NewFileStorage()
	restoreFd, err := store.PullBackup(dbr.Spec.Path)
	if err != nil {
		return r.updateStatusToFailed(ctx, &dbr, err, "download restore file failed", log)
	}
	// todo: file store won't delete source file
	// 		object store should clean temp files
	defer restoreFd.Close()

	err = m.Restore(dbMeta, restoreFd)
	if err != nil {
		return r.updateStatusToFailed(ctx, &dbr, err, "restore execute failed", log)
	}

	dbr.Status.Phase = quchengv1beta1.DbRestorePhaseCompleted
	dbr.Status.CompletionTimestamp = &metav1.Time{Time: r.clock.Now()}
	if err := r.Status().Update(ctx, &dbr); err != nil {
		log.WithError(err).Error("error updating DbRestore status")
		return ctrl.Result{}, err
	}
	log.WithFields(logrus.Fields{"phase": dbr.Status.Phase}).Info("update DbRestore status to completed")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DbRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&quchengv1beta1.DbRestore{}).
		Complete(r)
}

func (r *DbRestoreReconciler) updateStatusToFailed(ctx context.Context, dbr *quchengv1beta1.DbRestore, err error, msg string, log logrus.FieldLogger) (ctrl.Result, error) {
	original := dbr.DeepCopy()
	dbr.Status.Phase = quchengv1beta1.DbRestorePhaseFailed
	dbr.Status.Message = errors.WithMessage(err, msg).Error()
	dbr.Status.CompletionTimestamp = &metav1.Time{Time: r.clock.Now()}

	log.WithError(err).Error(msg)

	if err = r.Status().Patch(ctx, dbr, client.MergeFrom(original)); err != nil {
		log.WithError(err).Error("error updating DbRestore status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}
