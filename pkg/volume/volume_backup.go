// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package volume

import (
	"context"
	"fmt"
	"sync"
	"time"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	veleroclientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	velerov1informers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions/velero/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Backupper interface {
	FindBackupPvcs(namespace string, selector client.MatchingLabels) ([]PvcBackup, error)
	EnsureRepo(ctx context.Context, p PvcBackup, namespace string) (*velerov1.ResticRepository, error)
	AddTask(namespace string, repo *velerov1.ResticRepository, p PvcBackup)
	WaitSync(ctx context.Context) error
}

type backupper struct {
	schema        *runtime.Scheme
	backup        *quchengv1beta1.Backup
	kbClient      client.Client
	veleroClients veleroclientset.Interface
	log           logrus.FieldLogger
	bslName       string
	tasks         map[string]velerov1.PodVolumeBackup
	repoChans     map[string]chan *velerov1.ResticRepository
	repoLocks     map[string]*sync.Mutex
	pvbChan       chan *velerov1.PodVolumeBackup
	appName       string
}

func NewBackupper(ctx context.Context, backup *quchengv1beta1.Backup, schema *runtime.Scheme,
	veleroClient veleroclientset.Interface, kbClient client.Client,
	log logrus.FieldLogger, bslName string,
) (Backupper, error) {
	b := &backupper{
		backup:        backup,
		schema:        schema,
		veleroClients: veleroClient,
		kbClient:      kbClient,
		log:           log,
		bslName:       bslName,
		tasks:         make(map[string]velerov1.PodVolumeBackup),
		repoChans:     make(map[string]chan *velerov1.ResticRepository),
		repoLocks:     make(map[string]*sync.Mutex),
		pvbChan:       make(chan *velerov1.PodVolumeBackup),
	}

	if appName := backup.Spec.Selector[quchengv1beta1.SelectorReleaseKey]; appName != "" {
		b.appName = appName
	}

	pvbInf := velerov1informers.NewFilteredPodVolumeBackupInformer(
		veleroClient, backup.Namespace, 0, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		func(opts *metav1.ListOptions) {
			opts.LabelSelector = fmt.Sprintf("%s=%s", quchengv1beta1.BackupNameLabel, backup.Name)
		})

	repoInf := velerov1informers.NewResticRepositoryInformer(
		veleroClient, backup.Namespace, 0, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	)

	repoInf.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(old, new interface{}) {
				oldObj := old.(*velerov1.ResticRepository)
				newObj := new.(*velerov1.ResticRepository)

				if oldObj.Status.Phase == newObj.Status.Phase {
					return
				}

				if newObj.Status.Phase != velerov1.ResticRepositoryPhaseReady && newObj.Status.Phase != velerov1.ResticRepositoryPhaseNotReady {
					return
				}

				repoChan, ok := b.repoChans[newObj.Name]
				if !ok {
					log.Debugf("No ready channel found for repository %s/%s", newObj.Namespace, newObj.Name)
					return
				}

				repoChan <- newObj
			},
		})

	pvbInf.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(old, new interface{}) {
				oldObj := old.(*velerov1.PodVolumeBackup)
				newObj := new.(*velerov1.PodVolumeBackup)

				if oldObj.Status.Phase == newObj.Status.Phase {
					return
				}

				if newObj.Status.Phase != velerov1.PodVolumeBackupPhaseCompleted && newObj.Status.Phase != velerov1.PodVolumeBackupPhaseFailed {
					return
				}

				log.Debugf("updated pvb %s -> channel", newObj.Name)
				b.pvbChan <- newObj
			},
		})

	go pvbInf.Run(ctx.Done())
	go repoInf.Run(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), pvbInf.HasSynced, repoInf.HasSynced) {
		return nil, errors.New("timed out waiting for caches to sync")
	}
	return b, nil
}

type pvcInfo struct {
	pvb       velerov1.PodVolumeBackup
	pvcName   string
	podInfo   v1.ObjectReference
	confirmed bool
}

func (b *backupper) FindBackupPvcs(namespace string, selector client.MatchingLabels) ([]PvcBackup, error) {
	var result = make([]PvcBackup, 0)
	podList := v1.PodList{}
	if err := b.kbClient.List(context.TODO(), &podList, client.InNamespace(namespace), selector); err != nil {
		return result, err
	}

	var backupPvcMap = make(map[string]*v1.PersistentVolumeClaim)
	pvcList := v1.PersistentVolumeClaimList{}
	if err := b.kbClient.List(context.TODO(), &pvcList, client.InNamespace(namespace), selector); err != nil {
		return result, err
	}
	for _, pvc := range pvcList.Items {
		if pvc.Annotations[quchengv1beta1.PvcBackupExcludeAnnotation] == "true" {
			continue
		}
		backupPvcMap[pvc.Name] = &pvc
	}

	var pvcMap = make(map[string]PvcBackup)

	for _, pod := range podList.Items {
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil {
				pvcName := volume.PersistentVolumeClaim.ClaimName
				if _, ok := backupPvcMap[pvcName]; !ok {
					continue
				}
				if _, ok := pvcMap[pvcName]; !ok {
					pvcMap[pvcName] = PvcBackup{
						Pod:        pod,
						VolumeName: volume.Name,
						PvcName:    pvcName,
					}
				}
			}
		}
	}

	for _, p := range pvcMap {
		result = append(result, p)
	}

	return result, nil
}

func (b *backupper) EnsureRepo(ctx context.Context, p PvcBackup, namespace string) (*velerov1.ResticRepository, error) {
	repoName := fmt.Sprintf("%s-%s-%s", b.bslName, namespace, p.PvcName)
	repoPath := fmt.Sprintf("%s/%s/%s", p.Pod.Namespace, b.appName, p.PvcName)
	log := b.log

	currRepo, err := b.veleroClients.VeleroV1().ResticRepositories(namespace).Get(context.TODO(), repoName, metav1.GetOptions{})
	if err == nil {
		return currRepo, nil
	}

	repo := velerov1.ResticRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name: repoName, Namespace: namespace,
			Labels: map[string]string{
				quchengv1beta1.BackupNameLabel: b.backup.Name,
			},
		},
		Spec: velerov1.ResticRepositorySpec{
			BackupStorageLocation: b.bslName,
			VolumeNamespace:       repoPath,
		},
	}

	repoChan := b.getRepoChan(repoName)
	defer func() {
		delete(b.repoChans, repoName)
		close(repoChan)
	}()

	if _, err := b.veleroClients.VeleroV1().ResticRepositories(namespace).Create(context.TODO(), &repo, metav1.CreateOptions{}); err != nil {
		return nil, errors.Wrapf(err, "unable to create restic repository resource")
	}
	log.Debug("created restic repository")

	select {
	// repositories should become either ready or not ready quickly if they're
	// newly created.
	case <-time.After(time.Minute):
		return nil, errors.New("timed out waiting for restic repository to become ready")
	case <-ctx.Done():
		return nil, errors.New("timed out waiting for restic repository to become ready")
	case res := <-repoChan:
		if res.Status.Phase == velerov1.ResticRepositoryPhaseNotReady {
			return nil, errors.Errorf("restic repository is not ready: %s", res.Status.Message)
		}

		return res, nil
	}
}

func (b *backupper) AddTask(namespace string, repo *velerov1.ResticRepository, p PvcBackup) {
	timeStamp := time.Now().Unix()
	pvbName := fmt.Sprintf("%s-%d", repo.Name, timeStamp)
	log := b.log.WithField("pvb", pvbName)

	pvb := velerov1.PodVolumeBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:         pvbName,
			Namespace:    namespace,
			GenerateName: b.backup.Name + "-",
			Labels: map[string]string{
				velerov1.PVCUIDLabel:           p.PvcName,
				quchengv1beta1.BackupNameLabel: b.backup.Name,
			},
		},
		Spec: velerov1.PodVolumeBackupSpec{
			Pod: v1.ObjectReference{
				Kind:      "Pod",
				Name:      p.Pod.Name,
				Namespace: p.Pod.Namespace,
				UID:       p.Pod.UID,
			},
			Node:                  p.Pod.Spec.NodeName,
			Volume:                p.VolumeName,
			BackupStorageLocation: b.bslName,
			RepoIdentifier:        repo.Spec.ResticIdentifier,
		},
	}

	err := controllerutil.SetControllerReference(b.backup, &pvb, b.schema)
	if err != nil {
		log.WithError(err).Error("setup reference failed")
	}

	_, err = b.veleroClients.VeleroV1().PodVolumeBackups(namespace).Create(context.TODO(), &pvb, metav1.CreateOptions{})
	if err != nil {
		log.WithError(err).Error("create pvb failed")
	}
	b.tasks[pvb.Name] = pvb
	log.Infoln("creat podVolumeBackup success")
}

func (b *backupper) WaitSync(ctx context.Context) error {
	log := b.log
	log.Info("start wait sync")
	var err error

	for {
		select {
		case <-time.After(time.Minute):
			err = errors.New("timed out waiting for restic repository to become ready")
			goto END
		case <-ctx.Done():
			err = errors.New("timed out waiting for restic repository to become ready")
			goto END
		case res := <-b.pvbChan:
			log.Debugf("receive pvb %s with status %s", res.Name, res.Status.Phase)
			if res.Status.Phase == velerov1.PodVolumeBackupPhaseFailed {
				err = errors.Errorf("podVolumeBackup failed: %s", res.Status.Message)
				goto END
			}

			if _, ok := b.tasks[res.Name]; ok {
				log.Infof("remove completed task: %s", res.Name)
				delete(b.tasks, res.Name)
			}

			if len(b.tasks) == 0 {
				log.Infoln("all tasks completed")
				goto END
			}
		}
	}

END:
	return err
}

func (b *backupper) getRepoChan(name string) chan *velerov1.ResticRepository {
	b.repoChans[name] = make(chan *velerov1.ResticRepository)
	return b.repoChans[name]
}
