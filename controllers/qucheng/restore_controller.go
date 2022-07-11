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

	"github.com/sirupsen/logrus"
	quchengv1beta1 "gitlab.zcorp.cc/pangu/cne-operator/apis/qucheng/v1beta1"
	"gitlab.zcorp.cc/pangu/cne-operator/controllers/base"
	clientset "gitlab.zcorp.cc/pangu/cne-operator/pkg/client/clientset/versioned"
	quchenginformers "gitlab.zcorp.cc/pangu/cne-operator/pkg/client/informers/externalversions/qucheng/v1beta1"
	quchenglister "gitlab.zcorp.cc/pangu/cne-operator/pkg/client/listers/qucheng/v1beta1"
	"gitlab.zcorp.cc/pangu/cne-operator/pkg/db/mysql"
	"gitlab.zcorp.cc/pangu/cne-operator/pkg/storage"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RestoreReconciler reconciles a Restore object
type RestoreReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=restores,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=restores/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=qucheng.easycorp.io,resources=restores/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Restore object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *RestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&quchengv1beta1.Restore{}).
		Complete(r)
}

type RestoreController struct {
	*base.GenericController
	informer quchenginformers.RestoreInformer
	lister   quchenglister.RestoreLister
	clients  clientset.Interface
	kbClient client.Client

	logger logrus.FieldLogger
}

func NewRestoreController(informer quchenginformers.RestoreInformer, kbClient client.Client, clientset clientset.Interface, logger logrus.FieldLogger) base.Controller {
	c := &RestoreController{
		GenericController: base.NewGenericController(restoreControllerName, logger),
		lister:            informer.Lister(),
		kbClient:          kbClient,
		clients:           clientset,
	}

	informer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				restore := obj.(*quchengv1beta1.Restore)

				log := c.Logger.WithFields(logrus.Fields{
					"name": restore.Name, "namespace": restore.Namespace,
				})

				switch restore.Status.Phase {
				case "", quchengv1beta1.RestorePhaseNew:
					// only process new backups
				default:
					log.WithFields(logrus.Fields{
						"phase": restore.Status.Phase,
					}).Debug("request is not new, skip")
					return
				}

				key, err := cache.MetaNamespaceKeyFunc(restore)
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

func (c *RestoreController) process(key string) error {
	var err error

	ns, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		c.Logger.WithError(err).WithField("key", key).Error("split key failed")
		return err
	}

	log := c.Logger.WithFields(logrus.Fields{"name": name, "namespace": ns})
	origin, err := c.lister.Restores(ns).Get(name)
	if err != nil {
		log.WithError(err).Error("get backup field")
		return err
	}

	if origin.Status.Phase != "" && origin.Status.Phase != quchengv1beta1.RestorePhaseNew {
		log.Infoln("restore is not new, skip")
		return nil
	}

	request := origin.DeepCopy()
	request.Status.Phase = quchengv1beta1.RestorePhaseProcess
	if err = c.kbClient.Status().Update(context.TODO(), request); err != nil {
		log.WithError(err).Error("update status failed")
		return err
	}
	log.WithField("phase", request.Status.Phase).Infoln("updated status")

	var backup quchengv1beta1.Backup
	err = c.kbClient.Get(context.TODO(), client.ObjectKey{Namespace: origin.Namespace, Name: origin.Spec.BackupName}, &backup)
	if err != nil {
		log.WithError(err).Error("get backup resource failed")
		return err
	}
	log.Infoln("got backup", backup.Spec, backup.Status)
	log.Infof("find %d resources need for restore", len(backup.Status.Archives))

	for _, archive := range backup.Status.Archives {
		var db quchengv1beta1.Db
		store := storage.NewFileStorage()

		log.Infoln("find archive:", archive.Path)
		err = c.kbClient.Get(context.TODO(), client.ObjectKey{Namespace: backup.Namespace, Name: archive.DbRef.Name}, &db)
		if err != nil {
			log.WithError(err).Errorf("find db %s failed", archive.DbRef.Name)
			return err
		}

		targetFile, err := store.PullBackup(archive.Path)
		if err != nil {
			log.WithError(err).Error("pull backup file failed")
			request.Status.Phase = quchengv1beta1.RestorePhaseDownloadFailure
			request.Status.Reason = "pull backup file failed"
			_ = c.kbClient.Status().Update(context.TODO(), request)
			log.WithFields(logrus.Fields{
				"phase": request.Status.Phase, "reason": request.Status.Reason,
			}).Info("update status to failed")
			return err
		}
		log.Infof("download restore file %s", targetFile)

		p := mysql.NewParser(c.kbClient, &db, log)
		access, err := p.ParseAccessInfo()
		if err != nil {
			log.WithError(err).Error("parse db access info failed")
			return err
		}

		log.Infoln("start mysql restore")
		restoreReq := mysql.NewRestoreRequest(access, db.Spec.DbName, targetFile)
		err = restoreReq.Run()
		if err != nil {
			log.WithError(err).Error("execute mysql restore failed")
			request.Status.Phase = quchengv1beta1.RestorePhaseExecuteFailed
			request.Status.Reason = restoreReq.Errors()
			_ = c.kbClient.Status().Update(context.TODO(), request)
			return err
		}

		log.Infof("restore %s success", archive.Path)
	}

	log.Infoln("restore completed")
	request.Status.Phase = quchengv1beta1.RestorePhaseCompleted
	return c.kbClient.Status().Update(context.TODO(), request)
}

func (c *RestoreController) filterDbList(namespace string, selector client.MatchingLabels) (*quchengv1beta1.DbList, error) {
	list := quchengv1beta1.DbList{}

	ns := client.InNamespace(namespace)
	err := c.kbClient.List(context.TODO(), &list, ns, selector)
	return &list, err
}
