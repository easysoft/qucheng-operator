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
	"github.com/sirupsen/logrus"
	veleroclientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	veleroinformers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions"
	"github.com/vmware-tanzu/velero/pkg/restic"
	"k8s.io/apimachinery/pkg/runtime"
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

	log := c.Logger.WithFields(logrus.Fields{"name": name, "namespace": ns})

	origin, err := c.lister.Backups(ns).Get(name)
	if err != nil {
		log.WithError(err).Error("get backup field")
		return err
	}

	if origin.Status.Phase != "" && origin.Status.Phase != quchengv1beta1.BackupPhaseNew {
		log.Infoln("backup is not new, skip")
		return nil
	}

	request := origin.DeepCopy()
	request.Status.Phase = quchengv1beta1.BackupPhaseProcess
	if err = c.kbClient.Status().Update(context.TODO(), request); err != nil {
		log.WithError(err).Error("update status failed")
		return err
	}
	log.WithField("phase", request.Status.Phase).Infoln("updated status")

	archives := make([]quchengv1beta1.Archive, 0)

	ctx, cannelFunc := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cannelFunc()

	dbBackupper, err := db.NewBackupper(ctx, origin, c.schema, c.kbClient, c.clients, log)
	if err != nil {
		return err
	}

	log.Infoln("find backup dbs")
	dbs, err := dbBackupper.FindBackupDbs(origin.Spec.Namespace, origin.Spec.Selector)
	if err != nil {
		log.WithError(err).Errorf("search backup dbs failed, with selector %v", origin.Spec.Selector)
		return err
	}
	log.Infof("find %d dbs to backup", len(dbs.Items))

	for _, d := range dbs.Items {
		dbBackupper.AddTask(c.namespace, &d)
	}

	err = dbBackupper.WaitSync(ctx)
	if err != nil {
		log.WithError(err).Error("backup dbs ")
		request.Status.Phase = quchengv1beta1.BackupPhaseExecuteFailed
		return c.kbClient.Status().Update(context.TODO(), request)
	}
	log.Infoln("all of dbs was backed up")
	//for _, db := range dbs.Items {
	//	logdb := log.WithFields(logrus.Fields{
	//		"dbname": db.Spec.DbName,
	//	})
	//	p := mysql.NewParser(c.kbClient, &db, logdb)
	//	access, err := p.ParseAccessInfo()
	//	if err != nil {
	//		logdb.WithError(err).Error("parse db access info failed")
	//		return err
	//	}
	//
	//	backupReq := mysql.NewBackupRequest(access, db.Spec.DbName)
	//	err = backupReq.Run()
	//	if err != nil {
	//		logdb.WithError(err).Error("execute mysql backup failed")
	//		request.Status.Phase = quchengv1beta1.BackupPhaseExecuteFailed
	//		request.Status.Reason = backupReq.Errors()
	//		_ = c.kbClient.Status().Update(context.TODO(), request)
	//		logdb.WithFields(logrus.Fields{
	//			"phase": request.Status.Phase, "reason": request.Status.Reason,
	//		}).Info("update status to failed")
	//		return err
	//	}
	//
	//	backupInfo := storage.BackupInfo{
	//		BackupTime: backupReq.BackupTime, Name: name, Namespace: ns,
	//		File: backupReq.BackupFile.FullPath, FileFd: backupReq.BackupFile.Fd,
	//	}
	//	logdb.Infof("temporary backup file path %s", backupReq.BackupFile.FullPath)
	//
	//	store := storage.NewFileStorage()
	//	err = store.PutBackup(backupInfo)
	//	if err != nil {
	//		logdb.WithError(err).Error("put backup file to persistent storage failed")
	//		request.Status.Phase = quchengv1beta1.BackupPhaseUploadFailure
	//		request.Status.Reason = backupReq.Errors()
	//		_ = c.kbClient.Status().Update(context.TODO(), request)
	//		logdb.WithFields(logrus.Fields{
	//			"phase": request.Status.Phase, "reason": request.Status.Reason,
	//		}).Info("update status to failed")
	//		return err
	//	}
	//
	//	archive := quchengv1beta1.Archive{
	//		Path:  store.GetAbsPath(),
	//		DbRef: &quchengv1beta1.DbRef{Name: db.Name},
	//	}
	//	logdb.Infof("archive successful, storage path %s", store.GetAbsPath())
	//	archives = append(archives, archive)
	//}

	bslName := "minio"
	backupper, err := volume.NewBackupper(ctx, origin, c.schema, c.veleroClients, c.kbClient, c.veleroInfs, log, bslName)
	if err != nil {
		return err
	}

	backupPvclist, err := backupper.FindBackupPvcs(origin.Spec.Namespace, origin.Spec.Selector)
	if err != nil {
		return err
	}
	for _, pvcInfo := range backupPvclist {
		resticRepo, err := backupper.EnsureRepo(c.ctx, pvcInfo, c.namespace)
		if err != nil {
			log.WithError(err).WithField("pvc", pvcInfo.PvcName).Error("ensure repo failed")
			return err
		}
		backupper.AddTask(c.namespace, resticRepo, pvcInfo)
	}

	err = backupper.WaitSync(c.ctx)
	if err != nil {
		request.Status.Phase = quchengv1beta1.BackupPhaseExecuteFailed
		return c.kbClient.Status().Update(context.TODO(), request)
	}
	log.Infoln("all of volumes was backed up")

	request.Status.Archives = archives
	request.Status.Phase = quchengv1beta1.BackupPhaseCompleted
	log.WithFields(logrus.Fields{
		"phase": request.Status.Phase,
	}).Info("update status to completed")
	return c.kbClient.Status().Update(context.TODO(), request)
}
