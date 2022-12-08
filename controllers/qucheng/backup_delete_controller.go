// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package qucheng

import (
	"context"
	"strings"
	"time"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	"github.com/easysoft/qucheng-operator/pkg/storage"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/label"
	"github.com/vmware-tanzu/velero/pkg/restic"
	"github.com/vmware-tanzu/velero/pkg/util/kube"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	kubeerrs "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	resticTimeout             = time.Minute
	deleteBackupRequestMaxAge = 24 * time.Hour
)

type backupDeletionReconciler struct {
	client.Client
	logger logrus.FieldLogger
	//backupTracker     BackupTracker
	resticMgr restic.RepositoryManager
	//metrics           *metrics.ServerMetrics
	Scheme *runtime.Scheme
	clock  clock.Clock
	//discoveryHelper   discovery.Helper
	//newPluginManager  func(logrus.FieldLogger) clientmgmt.Manager
	//backupStoreGetter persistence.ObjectBackupStoreGetter
}

// NewBackupDeletionReconciler creates a new backup deletion reconciler.
func NewBackupDeletionReconciler(
	client client.Client, schema *runtime.Scheme, logger logrus.FieldLogger,
	//backupTracker BackupTracker,
	resticMgr restic.RepositoryManager,
	//metrics *metrics.ServerMetrics,
	//helper discovery.Helper,
	//newPluginManager func(logrus.FieldLogger) clientmgmt.Manager,
	//backupStoreGetter persistence.ObjectBackupStoreGetter,
) *backupDeletionReconciler {
	return &backupDeletionReconciler{
		Client: client,
		logger: logger,
		//backupTracker:     backupTracker,
		resticMgr: resticMgr,
		//metrics:           metrics,
		Scheme: schema,
		clock:  clock.RealClock{},
		//discoveryHelper:   helper,
		//newPluginManager:  newPluginManager,
		//backupStoreGetter: backupStoreGetter,
	}
}

func (r *backupDeletionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Make sure the expired requests can be deleted eventually
	r.logger.Debug("setup manager for backupDeletionReconciler")
	s := kube.NewPeriodicalEnqueueSource(r.logger, r.Client, &quchengv1beta1.DeleteBackupRequestList{}, time.Minute*15)
	return ctrl.NewControllerManagedBy(mgr).
		For(&quchengv1beta1.DeleteBackupRequest{}).
		Watches(s, nil).
		Complete(r)
}

//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=deletebackuprequests,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=deletebackuprequests/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=deletebackuprequests/finalizers,verbs=update
func (r *backupDeletionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.logger.WithFields(logrus.Fields{
		"controller": "BackupDeletion",
		"dbr":        req.String(),
	})

	log.Debug("Getting deletebackuprequest")
	dbr := &quchengv1beta1.DeleteBackupRequest{}
	if err := r.Get(ctx, req.NamespacedName, dbr); err != nil {
		if apierrors.IsNotFound(err) {
			log.Debug("Unable to find the deletebackuprequest")
			return ctrl.Result{}, nil
		}
		log.WithError(err).Error("Error getting deletebackuprequest")
		return ctrl.Result{}, err
	}

	// Since we use the reconciler along with the PeriodicalEnqueueSource, there may be reconciliation triggered by
	// stale requests.
	if dbr.Status.Phase == quchengv1beta1.DeleteBackupPhaseCompleted {
		age := r.clock.Now().Sub(dbr.CreationTimestamp.Time)
		if age >= deleteBackupRequestMaxAge { // delete the expired request
			log.Debug("The request is expired, deleting it.")
			if err := r.Delete(ctx, dbr); err != nil {
				log.WithError(err).Error("Error deleting DeleteBackupRequest")
			}
		} else {
			log.Info("The request has been processed, skip.")
		}
		return ctrl.Result{}, nil
	}

	log = log.WithField("backup", dbr.Spec.BackupName)
	// Remove any existing deletion requests for this backup so we only have
	// one at a time
	if errs := r.deleteExistingDeletionRequests(ctx, dbr, log); errs != nil {
		return ctrl.Result{}, kubeerrs.NewAggregate(errs)
	}

	// Get the backup we're trying to delete
	backup := &quchengv1beta1.Backup{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: dbr.Namespace,
		Name:      dbr.Spec.BackupName,
	}, backup); apierrors.IsNotFound(err) {
		// Couldn't find backup - update status to Processed and record the not-found error
		origin := dbr.DeepCopy()
		dbr.Status.Phase = quchengv1beta1.DeleteBackupPhaseCompleted
		dbr.Status.Message = "backup not found"
		if err = r.Status().Patch(ctx, dbr, client.MergeFrom(origin)); err != nil {
			return r.updateStatusToFailed(ctx, dbr, err, "update status failed", log)
		}
		log.WithField("phase", dbr.Status.Phase).Infoln("updated status")
		return ctrl.Result{}, nil
	} else if err != nil {
		return r.updateStatusToFailed(ctx, dbr, err, "error getting backup", log)
	}

	// if the request object has no labels defined, initialise an empty map since
	// we will be updating labels
	if dbr.Labels == nil {
		dbr.Labels = map[string]string{}
	}
	origin := dbr.DeepCopy()
	dbr.Status.Phase = quchengv1beta1.DeleteBackupPhaseProcess
	if dbr.Labels[quchengv1beta1.BackupNameLabel] == "" {
		dbr.Labels[quchengv1beta1.BackupNameLabel] = label.GetValidName(dbr.Spec.BackupName)
	}
	// Update status to InProgress and set backup-name and backup-uid label if needed
	if err := r.Status().Patch(ctx, dbr, client.MergeFrom(origin)); err != nil {
		return r.updateStatusToFailed(ctx, dbr, err, "update status failed", log)
	}

	// Set backup status to Deleting
	if err := r.updateBackupStatusToDeleting(ctx, backup, log); err != nil {
		return r.updateStatusToFailed(ctx, dbr, err, "Error setting backup phase to deleting", log)
	}

	var errs = make([]error, 0)
	// find related podvolumebackups, and exec restic forget <snapshot>
	log.Info("Removing restic snapshots")
	if deleteErrs := r.deleteResticSnapshots(ctx, backup); len(deleteErrs) > 0 {
		for _, err := range deleteErrs {
			errs = append(errs, err)
		}
	}
	if errs != nil {
		return r.updateStatusToFailed(ctx, dbr, kubeerrs.NewAggregate(errs), "remove restic snapshots failed", log)
	}
	// todo prune

	// find related dbbackups, and remove the stored file or object.
	if deleteErrs := r.deleteDbDumpFiles(ctx, backup); len(deleteErrs) > 0 {
		for _, err := range deleteErrs {
			errs = append(errs, err)
		}
	}

	// Remove related restores
	log.Info("Removing restores")
	restoreList := &quchengv1beta1.RestoreList{}
	selector := labels.Set{quchengv1beta1.BackupNameLabel: backup.Name}.AsSelector()
	if err := r.List(ctx, restoreList, &client.ListOptions{
		Namespace:     backup.Namespace,
		LabelSelector: selector,
	}); err != nil {
		log.WithError(errors.WithStack(err)).Error("Error listing restore API objects")
	} else {
		for _, restore := range restoreList.Items {

			log.Infof("Deleting restore %s", restore.Name)
			if err = r.Delete(ctx, &restore); err != nil {
				errs = append(errs, errors.Wrapf(err, "error deleting restore %s", kube.NamespaceAndName(&restore)))
			}
		}
	}

	// If no errors,
	//  update status phase to complete,
	//  remove the backup, the related podvolumebackup and dbbackup will be deleted automatic.
	// Else to failed.
	origin = dbr.DeepCopy()
	if len(errs) == 0 {
		// Only try to delete the backup object from kube if everything preceding went smoothly
		if err := r.Delete(ctx, backup); err != nil {
			return r.updateStatusToFailed(ctx, dbr, err, "error deleting backup", log)
		}
		dbr.Status.Phase = quchengv1beta1.DeleteBackupPhaseCompleted
		dbr.Status.Message = "Completed"
	} else {
		dbr.Status.Phase = quchengv1beta1.DeleteBackupPhaseFailed
		dbr.Status.Message = errs[0].Error()
	}

	if err := r.Status().Patch(ctx, dbr, client.MergeFrom(origin)); err != nil {
		log.WithError(err).Error("update status failed")
		return ctrl.Result{}, err
	}
	log.WithField("phase", dbr.Status.Phase).Infoln("updated status")
	return ctrl.Result{}, nil
}

func (r *backupDeletionReconciler) deleteExistingDeletionRequests(ctx context.Context, req *quchengv1beta1.DeleteBackupRequest, log logrus.FieldLogger) []error {
	log.Info("Removing existing deletion requests for backup")
	dbrList := &quchengv1beta1.DeleteBackupRequestList{}
	selector := label.NewSelectorForBackup(req.Spec.BackupName)
	if err := r.List(ctx, dbrList, &client.ListOptions{
		Namespace:     req.Namespace,
		LabelSelector: selector,
	}); err != nil {
		return []error{errors.Wrap(err, "error listing existing DeleteBackupRequests for backup")}
	}
	var errs []error
	for _, dbr := range dbrList.Items {
		if dbr.Name == req.Name {
			continue
		}
		if err := r.Delete(ctx, &dbr); err != nil {
			errs = append(errs, errors.WithStack(err))
		} else {
			log.Infof("deletion request '%s' removed.", dbr.Name)
		}
	}
	return errs
}

func (r *backupDeletionReconciler) patchDeleteBackupRequest(ctx context.Context, req *quchengv1beta1.DeleteBackupRequest, mutate func(*quchengv1beta1.DeleteBackupRequest)) (*quchengv1beta1.DeleteBackupRequest, error) {
	original := req.DeepCopy()
	mutate(req)
	if err := r.Patch(ctx, req, client.MergeFrom(original)); err != nil {
		return nil, errors.Wrap(err, "error patching the deletebackuprquest")
	}
	return req, nil
}

func (r *backupDeletionReconciler) deleteResticSnapshots(ctx context.Context, backup *quchengv1beta1.Backup) []error {
	if r.resticMgr == nil {
		return nil
	}

	var errs []error

	//snapshots, err := restic.GetSnapshotsInBackup(ctx, backup, r.Client)

	podVolumeBackups := &velerov1.PodVolumeBackupList{}
	options := &client.ListOptions{
		LabelSelector: labels.Set(map[string]string{
			quchengv1beta1.BackupNameLabel: label.GetValidName(backup.Name),
		}).AsSelector(),
	}

	err := r.Client.List(ctx, podVolumeBackups, options)
	if err != nil {
		return []error{errors.WithStack(err)}
	}

	ctx2, cancelFunc := context.WithTimeout(ctx, resticTimeout)
	defer cancelFunc()
	for _, item := range podVolumeBackups.Items {
		if item.Status.SnapshotID == "" {
			continue
		}
		bsl := backup.Spec.StorageName
		if bsl == "" {
			bsl = "minio"
		}
		snapshot := restic.SnapshotIdentifier{
			VolumeNamespace:       strings.ReplaceAll(parseRepoNamespace(item.Spec.RepoIdentifier), "/", "-"), // we change the volumeNamespace to subPath of pvc
			BackupStorageLocation: bsl,
			SnapshotID:            item.Status.SnapshotID,
		}
		r.logger.Infof("forget snapshot %+v", snapshot)
		// The resticMgr doesn't check stderr,
		// so if repo or snapshot is incorrect, err still is nil
		if err = r.resticMgr.Forget(ctx2, snapshot); err != nil {
			r.logger.Error(err)
			errs = append(errs, err)
		}
	}

	r.logger.Debugf("found %d errors", len(errs))
	return errs
}

func parseRepoNamespace(s string) string {
	key := "restic/"
	index := strings.Index(s, key)
	return s[index+len(key):]
}

func (r *backupDeletionReconciler) deleteDbDumpFiles(ctx context.Context, backup *quchengv1beta1.Backup) []error {
	var errs []error
	dbBackups := &quchengv1beta1.DbBackupList{}
	options := &client.ListOptions{
		LabelSelector: labels.Set(map[string]string{
			quchengv1beta1.BackupNameLabel: label.GetValidName(backup.Name),
		}).AsSelector(),
	}

	err := r.Client.List(ctx, dbBackups, options)
	if err != nil {
		return []error{errors.WithStack(err)}
	}

	var store storage.Storage
	for _, dbb := range dbBackups.Items {
		if dbb.Status.Path == "" {
			r.logger.Infof("dbbackup path is null, ignore")
			continue
		}

		if dbb.Spec.BackupStorageLocation != "" {
			store, err = GetDbObjectStorage(ctx, r.Client, dbb.Spec.BackupStorageLocation, viper.GetString("pod-namespace"))
			if err != nil {
				errs = append(errs, err)
				continue
			}
		} else {
			store = storage.NewFileStorage()
		}

		r.logger.Infof("remove dbbackup dumpfile %s", dbb.Status.Path)
		if err = store.RemoveBackup(dbb.Status.Path); err != nil {
			errs = append(errs, err)
		}

		r.logger.Infof("remove dbbackup %s", dbb.Name)
		if err = r.Client.Delete(ctx, &dbb); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

func (r *backupDeletionReconciler) updateBackupStatusToDeleting(ctx context.Context, backup *quchengv1beta1.Backup, log logrus.FieldLogger) error {
	original := backup.DeepCopy()
	backup.Status.Phase = quchengv1beta1.BackupPhaseDeleting

	if err := r.Client.Status().Patch(ctx, backup, client.MergeFrom(original)); err != nil {
		log.WithError(err).Error("error updating Backup status")
		return err
	}

	return nil
}

func (r *backupDeletionReconciler) updateStatusToFailed(ctx context.Context, dbr *quchengv1beta1.DeleteBackupRequest, err error, msg string, log logrus.FieldLogger) (ctrl.Result, error) {
	original := dbr.DeepCopy()
	dbr.Status.Phase = quchengv1beta1.DeleteBackupPhaseFailed
	dbr.Status.Message = errors.WithMessage(err, msg).Error()
	dbr.Status.CompletionTimestamp = &metav1.Time{Time: r.clock.Now()}

	log.WithError(err).Error(msg)

	if err = r.Status().Patch(ctx, dbr, client.MergeFrom(original)); err != nil {
		log.WithError(err).Error("error updating DeleteBackupRequest status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}
