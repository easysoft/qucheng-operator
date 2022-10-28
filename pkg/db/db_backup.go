// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"fmt"
	"time"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	clientset "github.com/easysoft/qucheng-operator/pkg/client/clientset/versioned"
	quickonv1binfs "github.com/easysoft/qucheng-operator/pkg/client/informers/externalversions/qucheng/v1beta1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Backupper interface {
	FindBackupDbs(namespace string, selector client.MatchingLabels) (*quchengv1beta1.DbList, error)
	AddTask(namespace string, db *quchengv1beta1.Db)
	WaitSync(ctx context.Context) error
}

type backupper struct {
	schema        *runtime.Scheme
	backup        *quchengv1beta1.Backup
	kbClient      client.Client
	quickonClient clientset.Interface
	log           logrus.FieldLogger
	tasks         map[string]quchengv1beta1.DbBackup
	dbbChan       chan *quchengv1beta1.DbBackup
	appName       string
}

func NewBackupper(ctx context.Context, backup *quchengv1beta1.Backup, schema *runtime.Scheme,
	kbClient client.Client, quickonClient clientset.Interface,
	log logrus.FieldLogger,
) (Backupper, error) {
	b := &backupper{
		schema:        schema,
		backup:        backup,
		kbClient:      kbClient,
		quickonClient: quickonClient,
		log:           log,
		tasks:         make(map[string]quchengv1beta1.DbBackup),
		dbbChan:       make(chan *quchengv1beta1.DbBackup),
	}

	if appName := backup.Spec.Selector[quchengv1beta1.SelectorReleaseKey]; appName != "" {
		b.appName = appName
	}

	inf := quickonv1binfs.NewFilteredDbBackupInformer(quickonClient,
		backup.Namespace, 0, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		func(opts *metav1.ListOptions) {
			opts.LabelSelector = fmt.Sprintf("%s=%s", quchengv1beta1.BackupNameLabel, backup.Name)
		})

	inf.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(old, new interface{}) {
				oldObj := old.(*quchengv1beta1.DbBackup)
				newObj := new.(*quchengv1beta1.DbBackup)

				if oldObj.Status.Phase == newObj.Status.Phase {
					return
				}

				if newObj.Status.Phase != quchengv1beta1.DbBackupPhasePhaseCompleted && newObj.Status.Phase != quchengv1beta1.DbBackupPhasePhaseFailed {
					return
				}

				b.dbbChan <- newObj
			},
		})

	go inf.Run(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
		return nil, errors.New("timed out waiting for caches to sync")
	}

	return b, nil
}

func (b *backupper) FindBackupDbs(namespace string, selector client.MatchingLabels) (*quchengv1beta1.DbList, error) {
	list := quchengv1beta1.DbList{}

	ns := client.InNamespace(namespace)
	err := b.kbClient.List(context.TODO(), &list, ns, selector)
	return &list, err
}

func (b *backupper) AddTask(namespace string, db *quchengv1beta1.Db) {
	log := b.log
	timeStamp := time.Now().Unix()
	dbbName := fmt.Sprintf("%s-%d", db.Name, timeStamp)

	dbb := quchengv1beta1.DbBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbbName,
			Namespace: namespace,
			Labels: map[string]string{
				quchengv1beta1.BackupNameLabel:      b.backup.Name,
				quchengv1beta1.ApplicationNameLabel: b.appName,
			},
		},
		Spec: quchengv1beta1.DbBackupSpec{
			Db: v1.ObjectReference{
				Kind: "Db",
				Name: db.Name, Namespace: db.Namespace,
				UID: db.UID,
			},
		},
	}
	err := controllerutil.SetControllerReference(b.backup, &dbb, b.schema)
	if err != nil {
		log.WithError(err).Error("setup reference failed")
	}

	_, err = b.quickonClient.QuchengV1beta1().DbBackups(namespace).Create(context.TODO(), &dbb, metav1.CreateOptions{})
	if err != nil {
		log.WithError(err).Error("create dbBackup failed")
	}

	b.tasks[dbb.Name] = dbb
	log.Infoln("creat dbBackup success")
}

func (b *backupper) WaitSync(ctx context.Context) error {
	var err error
	log := b.log
	log.Info("start wait sync")
	for {
		select {
		case <-time.After(5 * time.Minute):
			err = errors.New("timed out waiting for restic repository to become ready")
			goto END
		case <-ctx.Done():
			err = errors.New("timed out waiting for restic repository to become ready")
			goto END
		case res := <-b.dbbChan:
			if res.Status.Phase == quchengv1beta1.DbBackupPhasePhaseFailed {
				err = errors.Errorf("dbBackup failed: %s", res.Status.Message)
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
