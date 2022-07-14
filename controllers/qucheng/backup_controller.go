// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package qucheng

import (
	"context"
	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	"github.com/easysoft/qucheng-operator/controllers/base"
	clientset "github.com/easysoft/qucheng-operator/pkg/client/clientset/versioned"
	quchenginformers "github.com/easysoft/qucheng-operator/pkg/client/informers/externalversions/qucheng/v1beta1"
	quchenglister "github.com/easysoft/qucheng-operator/pkg/client/listers/qucheng/v1beta1"
	"github.com/easysoft/qucheng-operator/pkg/db"
	"github.com/easysoft/qucheng-operator/pkg/volume"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	veleroclientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	veleroinformers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions"
	"github.com/vmware-tanzu/velero/pkg/restic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

type BackupController struct {
	*base.GenericController
	ctx           context.Context
	namespace     string
	informer      quchenginformers.BackupInformer
	lister        quchenglister.BackupLister
	clients       clientset.Interface
	veleroClients veleroclientset.Interface
	veleroInfs    veleroinformers.SharedInformerFactory
	kbClient      client.Client
	schema        *runtime.Scheme
	clock         clock.Clock
	repoManager   restic.RepositoryManager
}

func NewBackupController(
	ctx context.Context, namespace string, schema *runtime.Scheme, informer quchenginformers.BackupInformer,
	kbClient client.Client, clientset clientset.Interface, veleroClientset veleroclientset.Interface,
	veleroInfs veleroinformers.SharedInformerFactory,
	logger logrus.FieldLogger,
	repoManager restic.RepositoryManager) base.Controller {
	c := &BackupController{
		ctx:               ctx,
		GenericController: base.NewGenericController(backupControllerName, logger),
		namespace:         namespace,
		lister:            informer.Lister(),
		kbClient:          kbClient,
		clients:           clientset,
		veleroClients:     veleroClientset,
		veleroInfs:        veleroInfs,
		schema:            schema,
		clock:             clock.RealClock{},
		repoManager:       repoManager,
	}

	informer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				backup := obj.(*quchengv1beta1.Backup)

				log := c.Logger.WithFields(logrus.Fields{
					"name": backup.Name, "namespace": backup.Namespace,
				})

				switch backup.Status.Phase {
				case "", quchengv1beta1.BackupPhaseNew:
					// only process new backups
				default:
					log.WithFields(logrus.Fields{
						"phase": backup.Status.Phase,
					}).Debug("request is not new, skip")
					return
				}

				key, err := cache.MetaNamespaceKeyFunc(backup)
				if err != nil {
					log.Error("Error creating queue key, item not added to queue")
					return
				}
				c.Queue.Add(key)
				log.Infof("task key %s added to queue", key)
			},
		},
	)

	c.SyncHandler = c.process
	return c
}

func (c *BackupController) process(key string) error {
	var err error

	ns, name, err := cache.SplitMetaNamespaceKey(key)

	if err != nil {
		c.Logger.WithError(err).WithField("key", key).Error("split key failed")
		return err
	}

	log := c.Logger.WithFields(logrus.Fields{"backup": name, "namespace": ns})

	backup, err := c.lister.Backups(ns).Get(name)
	if err != nil {
		log.WithError(err).Error("get backup field")
		return err
	}

	if backup.Status.Phase != "" && backup.Status.Phase != quchengv1beta1.BackupPhaseNew {
		log.Debugln("backup is not new, skip")
		return nil
	}

	original := backup.DeepCopy()
	backup.Status.Phase = quchengv1beta1.BackupPhaseProcess
	if err = c.kbClient.Patch(c.ctx, backup, client.MergeFrom(original)); err != nil {
		log.WithError(err).Error("update status failed")
		return err
	}
	log.WithField("phase", backup.Status.Phase).Infoln("updated status")

	ctx, cannelFunc := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cannelFunc()

	dbBackupper, err := db.NewBackupper(ctx, backup, c.schema, c.kbClient, c.clients, log)
	if err != nil {
		return c.updateStatusToFailed(ctx, backup, err, "init db backupper failed", log)
	}

	log.Infoln("find backup dbs")
	dbs, err := dbBackupper.FindBackupDbs(backup.Spec.Namespace, backup.Spec.Selector)
	if err != nil {
		return c.updateStatusToFailed(ctx, backup, err, "search backup dbs failed", log)
	}
	log.Infof("find %d dbs to backup", len(dbs.Items))

	for _, d := range dbs.Items {
		dbBackupper.AddTask(c.namespace, &d)
	}

	err = dbBackupper.WaitSync(ctx)
	if err != nil {
		return c.updateStatusToFailed(ctx, backup, err, "not all tasks complete with success", log)
	}
	log.Infoln("all of dbs was backed up")

	bslName := "minio"
	backupper, err := volume.NewBackupper(ctx, backup, c.schema, c.veleroClients, c.kbClient, log, bslName)
	if err != nil {
		return c.updateStatusToFailed(ctx, backup, err, "init volume backupper failed", log)
	}

	backupPvclist, err := backupper.FindBackupPvcs(backup.Spec.Namespace, backup.Spec.Selector)
	if err != nil {
		return c.updateStatusToFailed(ctx, backup, err, "lookup backup pvc list failed", log)
	}

	for _, pvcInfo := range backupPvclist {
		resticRepo, err := backupper.EnsureRepo(c.ctx, pvcInfo, c.namespace)
		if err != nil {
			return c.updateStatusToFailed(ctx, backup, err, "ensure repo failed", log.WithField("pvc", pvcInfo.PvcName))
		}
		backupper.AddTask(c.namespace, resticRepo, pvcInfo)
	}

	err = backupper.WaitSync(c.ctx)
	if err != nil {
		return c.updateStatusToFailed(ctx, backup, err, "not all tasks complete with success", log)
	}

	log.Infoln("all of volumes was backed up")

	backup.Status.Phase = quchengv1beta1.BackupPhaseCompleted
	backup.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}
	log.WithFields(logrus.Fields{"phase": backup.Status.Phase}).Info("update status to completed")
	return c.kbClient.Status().Update(context.TODO(), backup)
}

func (c *BackupController) updateStatusToFailed(ctx context.Context, backup *quchengv1beta1.Backup, err error, msg string, log logrus.FieldLogger) error {
	original := backup.DeepCopy()
	backup.Status.Phase = quchengv1beta1.BackupPhaseFailed
	backup.Status.Message = errors.WithMessage(err, msg).Error()
	backup.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}

	log.WithError(err).Error(msg)

	if err = c.kbClient.Status().Patch(ctx, backup, client.MergeFrom(original)); err != nil {
		log.WithError(err).Error("error updating Backup status")
		return err
	}

	return err
}
