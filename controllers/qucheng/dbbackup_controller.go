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
	"os"
	"time"

	dbmanage "github.com/easysoft/qucheng-operator/pkg/db/manage"

	"github.com/easysoft/qucheng-operator/pkg/storage"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/clock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
)

// DbBackupReconciler reconciles a DbBackup object
type DbBackupReconciler struct {
	logger logrus.FieldLogger
	client.Client
	clock  clock.Clock
	Scheme *runtime.Scheme
}

func NewDbBackupReconciler(client client.Client, schema *runtime.Scheme, logger logrus.FieldLogger) *DbBackupReconciler {
	r := DbBackupReconciler{
		Client: client, Scheme: schema,
		logger: logger.WithField("controller", "dbbackup"), clock: clock.RealClock{},
	}
	return &r
}

//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=dbbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=dbbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=dbbackups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DbBackup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *DbBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.logger

	var dbb quchengv1beta1.DbBackup
	if err := r.Get(ctx, req.NamespacedName, &dbb); err != nil {
		if apierrors.IsNotFound(err) {
			log.Debug("Unable to find DbBackup")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, errors.Wrap(err, "getting DbBackup")
	}

	log = log.WithField("dbb", dbb.Name)
	r.logger = log

	switch dbb.Status.Phase {
	case "", quchengv1beta1.DbBackupPhasePhaseNew:
		// Only process new items.
	default:
		log.Debug("DbBackup is not new, not processing")
		return ctrl.Result{}, nil
	}

	//original := dbb.DeepCopy()
	dbb.Status.Phase = quchengv1beta1.DbBackupPhasePhaseInProgress
	dbb.Status.StartTimestamp = &metav1.Time{Time: r.clock.Now()}
	if err := r.Status().Update(ctx, &dbb); err != nil {
		log.WithError(err).Error("error updating DbBackup status")
		return ctrl.Result{}, err
	}

	dbRef := dbb.Spec.Db
	var db quchengv1beta1.Db
	if err := r.Client.Get(ctx, client.ObjectKey{Name: dbRef.Name, Namespace: dbRef.Namespace}, &db); err != nil {
		return r.updateStatusToFailed(ctx, &dbb, err, "db not found", log)
	}

	m, dbMeta, err := dbmanage.ParseDB(ctx, r.Client, &db)
	if err != nil {
		return r.updateStatusToFailed(ctx, &dbb, err, "parse dbmanager failed", log)
	}

	backupFd, err := m.Dump(dbMeta)
	if err != nil {
		return r.updateStatusToFailed(ctx, &dbb, err, "execute db backup failed", log)
	}

	appName := dbb.Labels[quchengv1beta1.ApplicationNameLabel]
	path := r.genPersistentPath(appName, string(m.DbType()), &db)

	size, err := r.processPersistent(path, backupFd)
	if err != nil {
		return r.updateStatusToFailed(ctx, &dbb, err, "put backup file to persistent storage failed", log)
	}

	dbb.Status.Phase = quchengv1beta1.DbBackupPhasePhaseCompleted
	dbb.Status.CompletionTimestamp = &metav1.Time{Time: r.clock.Now()}
	dbb.Status.Path = path
	dbb.Status.Size = resource.NewQuantity(size, resource.BinarySI)
	if err := r.Status().Update(ctx, &dbb); err != nil {
		log.WithError(err).Error("error updating DbBackup status")
		return ctrl.Result{}, err
	}
	log.WithFields(logrus.Fields{"phase": dbb.Status.Phase}).Info("update DbBackup status to completed")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DbBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&quchengv1beta1.DbBackup{}).
		Complete(r)
}

func (r *DbBackupReconciler) updateStatusToFailed(ctx context.Context, dbb *quchengv1beta1.DbBackup, err error, msg string, log logrus.FieldLogger) (ctrl.Result, error) {
	original := dbb.DeepCopy()
	dbb.Status.Phase = quchengv1beta1.DbBackupPhasePhaseFailed
	dbb.Status.Message = errors.WithMessage(err, msg).Error()
	dbb.Status.CompletionTimestamp = &metav1.Time{Time: r.clock.Now()}

	log.WithError(err).Error(msg)

	if err = r.Status().Patch(ctx, dbb, client.MergeFrom(original)); err != nil {
		log.WithError(err).Error("error updating Backup status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}

//func (r *DbBackupReconciler) processDump(db *quchengv1beta1.Db) (*db.AccessInfo, *os.File, error) {
//	log := r.logger
//	p := mysql.NewParser(r.Client, db, r.logger)
//	access, err := p.ParseAccessInfo()
//	if err != nil {
//		log.WithError(err).Error("parse db access info failed")
//		return nil, nil, err
//	}
//
//	backupReq := mysql.NewBackupRequest(access, db.Spec.DbName)
//	err = backupReq.Run()
//	if err != nil {
//		return nil, nil, errors.New(backupReq.Errors())
//	}
//
//	return access, backupReq.BackupFile, nil
//}

func (r *DbBackupReconciler) processPersistent(absPath string, fd *os.File) (int64, error) {
	store := storage.NewFileStorage()
	defer func() {
		fd.Close()
		os.Remove(fd.Name())
	}()
	size, err := store.PutBackup(absPath, fd)
	return size, err
}

func (r *DbBackupReconciler) genPersistentPath(appName, dbType string, db *quchengv1beta1.Db) string {
	var suffix = "dump"
	switch dbType {
	case quchengv1beta1.DbTypeMysql:
		suffix = "sql"
	}
	return fmt.Sprintf("%s/%s/%s.%s.%s.%s", db.Namespace, appName, dbType, db.Spec.DbName,
		time.Now().Format("20060102150405"), suffix)
}
