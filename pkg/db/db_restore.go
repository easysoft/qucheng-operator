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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Restorer interface {
	FindDbBackupList(namespace string) (*quchengv1beta1.DbBackupList, error)
	AddTask(namespace string, dbb *quchengv1beta1.DbBackup)
	WaitSync(ctx context.Context) error
}

type restorer struct {
	schema        *runtime.Scheme
	restore       *quchengv1beta1.Restore
	kbClient      client.Client
	quickonClient clientset.Interface
	log           logrus.FieldLogger
	tasks         map[string]quchengv1beta1.DbRestore
	dbrChan       chan *quchengv1beta1.DbRestore
	appName       string
}

func NewRestorer(ctx context.Context, restore *quchengv1beta1.Restore, schema *runtime.Scheme,
	kbClient client.Client, quickonClient clientset.Interface,
	log logrus.FieldLogger,
) (Restorer, error) {
	r := &restorer{
		schema:        schema,
		restore:       restore,
		kbClient:      kbClient,
		quickonClient: quickonClient,
		log:           log,
		tasks:         make(map[string]quchengv1beta1.DbRestore),
		dbrChan:       make(chan *quchengv1beta1.DbRestore),
	}

	inf := quickonv1binfs.NewFilteredDbRestoreInformer(quickonClient,
		restore.Namespace, 0, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		func(opts *metav1.ListOptions) {
			opts.LabelSelector = fmt.Sprintf("%s=%s", quchengv1beta1.RestoreNameLabel, restore.Name)
		})

	inf.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(old, new interface{}) {
				oldObj := old.(*quchengv1beta1.DbRestore)
				newObj := new.(*quchengv1beta1.DbRestore)

				log.Debugln("receive dbr update event", newObj.Name, newObj.Status)
				if oldObj.Status.Phase == newObj.Status.Phase {
					return
				}

				if newObj.Status.Phase != quchengv1beta1.DbRestorePhaseCompleted && newObj.Status.Phase != quchengv1beta1.DbRestorePhaseFailed {
					return
				}

				log.Debugln("send event to chan", newObj.Name, newObj.Status.Phase)
				r.dbrChan <- newObj
			},
		})

	go inf.Run(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
		return nil, errors.New("timed out waiting for caches to sync")
	}

	return r, nil
}

func (r *restorer) FindDbBackupList(namespace string) (*quchengv1beta1.DbBackupList, error) {
	list := quchengv1beta1.DbBackupList{}

	err := r.kbClient.List(context.TODO(), &list, client.InNamespace(namespace),
		client.MatchingLabelsSelector{
			Selector: labels.Set{quchengv1beta1.BackupNameLabel: r.restore.Spec.BackupName}.AsSelector(),
		})
	return &list, err
}

func (r *restorer) AddTask(namespace string, dbb *quchengv1beta1.DbBackup) {
	log := r.log
	timeStamp := time.Now().Unix()
	dbbName := fmt.Sprintf("%s-%s-%d", r.restore.Name, dbb.Spec.Db.Name, timeStamp)

	dbr := quchengv1beta1.DbRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbbName,
			Namespace: namespace,
			Labels: map[string]string{
				quchengv1beta1.RestoreNameLabel: r.restore.Name,
			},
		},
		Spec: quchengv1beta1.DbRestoreSpec{
			Db:   dbb.Spec.Db,
			Path: dbb.Status.Path,
		},
	}
	err := controllerutil.SetControllerReference(r.restore, &dbr, r.schema)
	if err != nil {
		log.WithError(err).Error("setup reference failed")
	}

	_, err = r.quickonClient.QuchengV1beta1().DbRestores(namespace).Create(context.TODO(), &dbr, metav1.CreateOptions{})
	if err != nil {
		log.WithError(err).Error("create dbBackup failed")
	}

	r.tasks[dbr.Name] = dbr
	r.log.Infoln("creat dbRestore success")
}

func (r *restorer) WaitSync(ctx context.Context) error {
	var err error
	log := r.log
	log.Info("start wait sync")
	for {
		select {
		case <-time.After(time.Minute):
			err = errors.New("timed out waiting for db restore to become completed")
			goto END
		case <-ctx.Done():
			err = errors.New("timed out waiting for db restore to become completed")
			goto END
		case res := <-r.dbrChan:
			if res.Status.Phase == quchengv1beta1.DbRestorePhaseFailed {
				err = errors.Errorf("dbBackup failed: %s", res.Status.Message)
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
