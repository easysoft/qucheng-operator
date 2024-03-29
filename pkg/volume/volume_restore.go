// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package volume

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	veleroclientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	veleroinformers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions"
	velerov1informers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Restorer interface {
	FindPodVolumeBackups(namespace string) (*velerov1.PodVolumeBackupList, error)
	AddTask(namespace string, pvb *velerov1.PodVolumeBackup, pod v1.ObjectReference)
	WaitSync(ctx context.Context) error
}

type restorer struct {
	schema        *runtime.Scheme
	restore       *quchengv1beta1.Restore
	kbClient      client.Client
	veleroClients veleroclientset.Interface
	log           logrus.FieldLogger
	tasks         map[string]velerov1.PodVolumeRestore
	pvrChan       chan *velerov1.PodVolumeRestore
}

func NewRestorer(ctx context.Context, restore *quchengv1beta1.Restore, schema *runtime.Scheme,
	veleroClient veleroclientset.Interface, kbClient client.Client,
	veleroinfs veleroinformers.SharedInformerFactory,
	log logrus.FieldLogger,
) (Restorer, error) {
	r := &restorer{
		restore:       restore,
		schema:        schema,
		veleroClients: veleroClient,
		kbClient:      kbClient,
		log:           log,
		tasks:         make(map[string]velerov1.PodVolumeRestore),
		pvrChan:       make(chan *velerov1.PodVolumeRestore),
	}

	pvrInf := velerov1informers.NewFilteredPodVolumeRestoreInformer(
		veleroClient, restore.Namespace, 0, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		func(opts *metav1.ListOptions) {
			opts.LabelSelector = fmt.Sprintf("%s=%s", quchengv1beta1.RestoreNameLabel, restore.Name)
		})

	pvrInf.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(old, new interface{}) {
				oldObj := old.(*velerov1.PodVolumeRestore)
				newObj := new.(*velerov1.PodVolumeRestore)

				if oldObj.Status.Phase == newObj.Status.Phase {
					return
				}

				if newObj.Status.Phase != velerov1.PodVolumeRestorePhaseCompleted && newObj.Status.Phase != velerov1.PodVolumeRestorePhaseFailed {
					return
				}

				r.pvrChan <- newObj
			},
		})

	go pvrInf.Run(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), pvrInf.HasSynced) {
		return nil, errors.New("timed out waiting for caches to sync")
	}
	return r, nil
}

func (r *restorer) FindPodVolumeBackups(namespace string) (*velerov1.PodVolumeBackupList, error) {
	var pvbList velerov1.PodVolumeBackupList
	if err := r.kbClient.List(context.TODO(), &pvbList, client.InNamespace(namespace),
		client.MatchingLabelsSelector{
			Selector: labels.Set{quchengv1beta1.BackupNameLabel: r.restore.Spec.BackupName}.AsSelector(),
		}); err != nil {
		return nil, err
	}
	return &pvbList, nil
}

func (r *restorer) AddTask(namespace string, pvb *velerov1.PodVolumeBackup, pod v1.ObjectReference) {
	timeStamp := time.Now().Unix()
	pvbName := fmt.Sprintf("%s-%s-%d", r.restore.Name, pvb.Status.SnapshotID, timeStamp)
	log := r.log

	pvr := velerov1.PodVolumeRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:         pvbName,
			Namespace:    namespace,
			GenerateName: r.restore.Name + "-",
			Labels: map[string]string{
				quchengv1beta1.RestoreNameLabel: r.restore.Name,
			},
		},
		Spec: velerov1.PodVolumeRestoreSpec{
			Pod:                   pod,
			Volume:                pvb.Spec.Volume,
			BackupStorageLocation: pvb.Spec.BackupStorageLocation,
			RepoIdentifier:        pvb.Spec.RepoIdentifier,
			SnapshotID:            pvb.Status.SnapshotID,
		},
	}

	err := controllerutil.SetControllerReference(r.restore, &pvr, r.schema)
	if err != nil {
		log.WithError(err).Error("setup reference failed")
	}

	_, err = r.veleroClients.VeleroV1().PodVolumeRestores(namespace).Create(context.TODO(), &pvr, metav1.CreateOptions{})
	if err != nil {
		log.WithError(err).Error("create podVolumeRestore failed")
	}
	r.tasks[pvr.Name] = pvr
	log.Infoln("creat podVolumeRestore success")
}

func (r *restorer) WaitSync(ctx context.Context) error {
	var err error
	log := r.log
	log.Info("start wait sync")
	for {
		select {
		case <-time.After(5 * time.Minute):
			err = errors.New("timed out waiting for restic repository to become ready")
			goto END
		case <-ctx.Done():
			err = errors.New("timed out waiting for restic repository to become ready")
			goto END
		case res := <-r.pvrChan:
			if res.Status.Phase == velerov1.PodVolumeRestorePhaseFailed {
				err = errors.Errorf("podVolumeRestore failed: %s", res.Status.Message)
				goto END
			}

			if _, ok := r.tasks[res.Name]; ok {
				log.Infof("remove completed task: %s", res.Name)
				delete(r.tasks, res.Name)
			}

			if len(r.tasks) == 0 {
				log.Infoln("all tasks completed")
				goto END
			}
		}
	}

END:
	return err
}
