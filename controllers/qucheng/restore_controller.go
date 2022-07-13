// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package qucheng

import (
	"context"

	"github.com/easysoft/qucheng-operator/pkg/volume"
	veleroclientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	veleroinformers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions"
	"time"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	"github.com/easysoft/qucheng-operator/controllers/base"
	clientset "github.com/easysoft/qucheng-operator/pkg/client/clientset/versioned"
	quchenginformers "github.com/easysoft/qucheng-operator/pkg/client/informers/externalversions/qucheng/v1beta1"
	quchenglister "github.com/easysoft/qucheng-operator/pkg/client/listers/qucheng/v1beta1"
	"github.com/easysoft/qucheng-operator/pkg/db/mysql"
	"github.com/easysoft/qucheng-operator/pkg/storage"
	"github.com/sirupsen/logrus"
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
	ctx           context.Context
	namespace     string
	informer      quchenginformers.RestoreInformer
	lister        quchenglister.RestoreLister
	clients       clientset.Interface
	veleroClients veleroclientset.Interface
	veleroInfs    veleroinformers.SharedInformerFactory
	kbClient      client.Client
	schema        *runtime.Scheme
	logger        logrus.FieldLogger
}

func NewRestoreController(ctx context.Context, namespace string, schema *runtime.Scheme,
	informer quchenginformers.RestoreInformer, kbClient client.Client, clientset clientset.Interface,
	veleroClientset veleroclientset.Interface,
	veleroInfs veleroinformers.SharedInformerFactory, logger logrus.FieldLogger,
) base.Controller {
	c := &RestoreController{
		ctx:               ctx,
		namespace:         namespace,
		schema:            schema,
		GenericController: base.NewGenericController(restoreControllerName, logger),
		lister:            informer.Lister(),
		kbClient:          kbClient,
		clients:           clientset,
		veleroClients:     veleroClientset,
		veleroInfs:        veleroInfs,
		logger:            logger,
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

	ctx, cannelFunc := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cannelFunc()

	for _, archive := range backup.Status.Archives {
		// continue
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

	volumeRestorer, err := volume.NewRestorer(ctx, request, c.schema, c.veleroClients, c.kbClient, c.veleroInfs, c.logger)
	if err != nil {
		return err
	}
	pvbList, err := volumeRestorer.FindPodVolumeBackups(c.namespace)
	if err != nil {
		return err
	}

	for _, pvb := range pvbList.Items {
		volumeRestorer.AddTask(c.namespace, &pvb)
	}

	if err = volumeRestorer.WaitSync(c.ctx); err != nil {
		return err
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
