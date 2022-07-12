// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package qucheng

import (
	"context"
	"fmt"
	"github.com/vmware-tanzu/velero/pkg/restic"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	"github.com/easysoft/qucheng-operator/controllers/base"
	clientset "github.com/easysoft/qucheng-operator/pkg/client/clientset/versioned"
	quchenginformers "github.com/easysoft/qucheng-operator/pkg/client/informers/externalversions/qucheng/v1beta1"
	quchenglister "github.com/easysoft/qucheng-operator/pkg/client/listers/qucheng/v1beta1"
	"github.com/easysoft/qucheng-operator/pkg/db/mysql"
	"github.com/easysoft/qucheng-operator/pkg/storage"
	"github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	veleroclientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BackupController struct {
	*base.GenericController
	namespace     string
	informer      quchenginformers.BackupInformer
	lister        quchenglister.BackupLister
	clients       clientset.Interface
	veleroClients veleroclientset.Interface
	kbClient      client.Client
	schema        *runtime.Scheme
	repoManager   restic.RepositoryManager
}

func NewBackupController(
	namespace string, schema *runtime.Scheme, informer quchenginformers.BackupInformer,
	kbClient client.Client, clientset clientset.Interface, veleroClientset veleroclientset.Interface,
	logger logrus.FieldLogger,
	repoManager restic.RepositoryManager) base.Controller {
	c := &BackupController{
		GenericController: base.NewGenericController(backupControllerName, logger),
		namespace:         namespace,
		lister:            informer.Lister(),
		kbClient:          kbClient,
		clients:           clientset,
		veleroClients:     veleroClientset,
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

	log.Infoln("find backup resources")
	dbs, err := c.filterDbList(origin.Spec.Namespace, origin.Spec.Selector)
	if err != nil {
		log.WithError(err).Errorf("search backup dbs failed, with selector %v", origin.Spec.Selector)
		return err
	}

	log.Infof("find %d dbs to backup", len(dbs.Items))

	archives := make([]quchengv1beta1.Archive, 0)

	for _, db := range dbs.Items {
		continue
		logdb := log.WithFields(logrus.Fields{
			"dbname": db.Spec.DbName,
		})
		p := mysql.NewParser(c.kbClient, &db, logdb)
		access, err := p.ParseAccessInfo()
		if err != nil {
			logdb.WithError(err).Error("parse db access info failed")
			return err
		}

		backupReq := mysql.NewBackupRequest(access, db.Spec.DbName)
		err = backupReq.Run()
		if err != nil {
			logdb.WithError(err).Error("execute mysql backup failed")
			request.Status.Phase = quchengv1beta1.BackupPhaseExecuteFailed
			request.Status.Reason = backupReq.Errors()
			_ = c.kbClient.Status().Update(context.TODO(), request)
			logdb.WithFields(logrus.Fields{
				"phase": request.Status.Phase, "reason": request.Status.Reason,
			}).Info("update status to failed")
			return err
		}

		backupInfo := storage.BackupInfo{
			BackupTime: backupReq.BackupTime, Name: name, Namespace: ns,
			File: backupReq.BackupFile.FullPath, FileFd: backupReq.BackupFile.Fd,
		}
		logdb.Infof("temporary backup file path %s", backupReq.BackupFile.FullPath)

		store := storage.NewFileStorage()
		err = store.PutBackup(backupInfo)
		if err != nil {
			logdb.WithError(err).Error("put backup file to persistent storage failed")
			request.Status.Phase = quchengv1beta1.BackupPhaseUploadFailure
			request.Status.Reason = backupReq.Errors()
			_ = c.kbClient.Status().Update(context.TODO(), request)
			logdb.WithFields(logrus.Fields{
				"phase": request.Status.Phase, "reason": request.Status.Reason,
			}).Info("update status to failed")
			return err
		}

		archive := quchengv1beta1.Archive{
			Path:  store.GetAbsPath(),
			DbRef: &quchengv1beta1.DbRef{Name: db.Name},
		}
		logdb.Infof("archive successful, storage path %s", store.GetAbsPath())
		archives = append(archives, archive)
	}

	backupPvclist, err := c.filterPvcBackups(origin.Spec.Namespace, origin.Spec.Selector)
	bslName := "minio"
	for _, pvcInfo := range backupPvclist {
		log.WithFields(logrus.Fields{
			"pod":    pvcInfo.Pod.Name,
			"volume": pvcInfo.VolumeName,
			"pvc":    pvcInfo.PvcName,
		}).Info("create podVolumeBackup")

		repoName := fmt.Sprintf("%s-%s-%s", bslName, c.namespace, pvcInfo.PvcName)
		repoPath := fmt.Sprintf("%s/%s", pvcInfo.Pod.Namespace, pvcInfo.PvcName)
		currRepo, err := c.veleroClients.VeleroV1().ResticRepositories(c.namespace).Get(context.TODO(), repoName, metav1.GetOptions{})
		if err != nil {
			repo := velerov1.ResticRepository{
				ObjectMeta: metav1.ObjectMeta{
					Name: repoName, Namespace: c.namespace,
					Labels: map[string]string{
						quchengv1beta1.BackupNameLabel: origin.Name,
					},
				},
				Spec: velerov1.ResticRepositorySpec{
					BackupStorageLocation: bslName,
					VolumeNamespace:       repoPath,
				},
			}
			currRepo, err = c.veleroClients.VeleroV1().ResticRepositories(c.namespace).Create(context.TODO(), &repo, metav1.CreateOptions{})
			if err != nil {
				log.WithError(err).Error("restic repository create failed")
			}
		}

		time.Sleep(5 * time.Second)

		timeStamp := time.Now().Unix()
		pvbName := fmt.Sprintf("%s-%d", repoName, timeStamp)

		pvb := velerov1.PodVolumeBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:         pvbName,
				Namespace:    c.namespace,
				GenerateName: origin.Name + "-",
				Labels: map[string]string{
					velerov1.PVCUIDLabel:           pvcInfo.PvcName,
					quchengv1beta1.BackupNameLabel: origin.Name,
				},
			},
			Spec: velerov1.PodVolumeBackupSpec{
				Pod: v1.ObjectReference{
					Kind:      "Pod",
					Name:      pvcInfo.Pod.Name,
					Namespace: pvcInfo.Pod.Namespace,
					UID:       pvcInfo.Pod.UID,
				},
				Node:                  pvcInfo.Pod.Spec.NodeName,
				Volume:                pvcInfo.VolumeName,
				BackupStorageLocation: bslName,
				RepoIdentifier:        currRepo.Spec.ResticIdentifier,
			},
		}

		err = controllerutil.SetControllerReference(origin, &pvb, c.schema)
		if err != nil {
			log.WithError(err).WithField("pvb", pvbName).Error("setup reference failed")
		}

		_, err = c.veleroClients.VeleroV1().PodVolumeBackups(c.namespace).Create(context.TODO(), &pvb, metav1.CreateOptions{})
		if err != nil {
			log.WithError(err).WithField("pvb", pvbName).Error("create pvb failed")
		}
	}

	request.Status.Archives = archives
	request.Status.Phase = quchengv1beta1.BackupPhaseCompleted
	log.WithFields(logrus.Fields{
		"phase": request.Status.Phase,
	}).Info("update status to completed")
	return c.kbClient.Status().Update(context.TODO(), request)
}

func (c *BackupController) filterDbList(namespace string, selector client.MatchingLabels) (*quchengv1beta1.DbList, error) {
	list := quchengv1beta1.DbList{}

	ns := client.InNamespace(namespace)
	err := c.kbClient.List(context.TODO(), &list, ns, selector)
	return &list, err
}

func (c *BackupController) filterPvcBackupList(namespace string, selector client.MatchingLabels) (*v1.PersistentVolumeClaimList, error) {
	list := v1.PersistentVolumeClaimList{}
	ns := client.InNamespace(namespace)
	err := c.kbClient.List(context.TODO(), &list, ns, selector)
	return &list, err
}

func (c *BackupController) filterPvcBackups(namespace string, selector client.MatchingLabels) ([]pvcBackup, error) {
	var result = make([]pvcBackup, 0)
	pvcList := v1.PersistentVolumeClaimList{}
	ns := client.InNamespace(namespace)
	if err := c.kbClient.List(context.TODO(), &pvcList, ns, selector); err != nil {
		return result, err
	}

	podList := v1.PodList{}
	if err := c.kbClient.List(context.TODO(), &podList, ns, selector); err != nil {
		return result, err
	}

	var pvcMap = make(map[string]pvcBackup)

	for _, pod := range podList.Items {
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil {
				pvcName := volume.PersistentVolumeClaim.ClaimName
				if _, ok := pvcMap[pvcName]; !ok {
					// todo: filter exclude pvcs
					pvcMap[pvcName] = pvcBackup{
						Pod:        pod,
						VolumeName: volume.Name,
						PvcName:    pvcName,
					}
				}
			}
		}
	}

	for _, b := range pvcMap {
		result = append(result, b)
	}

	return result, nil
}

type pvcBackup struct {
	Pod        v1.Pod
	VolumeName string
	PvcName    string
}
