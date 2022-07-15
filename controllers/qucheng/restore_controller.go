// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package qucheng

import (
	"context"

	"github.com/easysoft/qucheng-operator/pkg/db"
	"github.com/pkg/errors"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/clock"

	"time"

	"github.com/easysoft/qucheng-operator/pkg/volume"
	veleroclientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	veleroinformers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	"github.com/easysoft/qucheng-operator/controllers/base"
	clientset "github.com/easysoft/qucheng-operator/pkg/client/clientset/versioned"
	quchenginformers "github.com/easysoft/qucheng-operator/pkg/client/informers/externalversions/qucheng/v1beta1"
	quchenglister "github.com/easysoft/qucheng-operator/pkg/client/listers/qucheng/v1beta1"
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
	clock         clock.Clock
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
		clock:             clock.RealClock{},
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
	restore, err := c.lister.Restores(ns).Get(name)
	if err != nil {
		log.WithError(err).Error("get backup field")
		return err
	}

	if restore.Status.Phase != "" && restore.Status.Phase != quchengv1beta1.RestorePhaseNew {
		log.Debug("restore is not new, skip")
		return nil
	}

	restore.Status.Phase = quchengv1beta1.RestorePhaseProcess
	if err = c.kbClient.Status().Update(context.TODO(), restore); err != nil {
		log.WithError(err).Error("update status failed")
		return err
	}
	log.WithField("phase", restore.Status.Phase).Infoln("updated status")

	var backup quchengv1beta1.Backup
	err = c.kbClient.Get(context.TODO(), client.ObjectKey{Namespace: restore.Namespace, Name: restore.Spec.BackupName}, &backup)
	if err != nil {
		return c.updateStatusToFailed(c.ctx, restore, err, "get target backup failed", log)
	}
	log.Infoln("got backup", backup.Spec, backup.Status)

	ctx, cannelFunc := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cannelFunc()

	dbrestorer, err := db.NewRestorer(ctx, restore, c.schema, c.kbClient, c.clients, log)
	if err != nil {
		return c.updateStatusToFailed(ctx, restore, err, "init db restorer failed", log)
	}

	dbbackups, err := dbrestorer.FindDbBackupList(c.namespace)
	if err != nil {
		return c.updateStatusToFailed(ctx, restore, err, "find dbbackups failed", log)
	}

	if len(dbbackups.Items) > 0 {
		for _, dbb := range dbbackups.Items {
			dbrestorer.AddTask(c.namespace, &dbb)
		}

		err = dbrestorer.WaitSync(ctx)
		if err != nil {
			log.WithError(err).Error("some tasks failed")
			return c.updateStatusToFailed(ctx, restore, err, "db restore task failed", log)
		}
	}

	volumeRestorer, err := volume.NewRestorer(ctx, restore, c.schema, c.veleroClients, c.kbClient, c.veleroInfs, c.logger)
	if err != nil {
		return c.updateStatusToFailed(ctx, restore, err, "init volume restorer failed", log)
	}
	pvbList, err := volumeRestorer.FindPodVolumeBackups(c.namespace)
	if err != nil {
		return c.updateStatusToFailed(ctx, restore, err, "find volumes for backup failed", log)
	}

	currPvbList, err := c.rebuildPodPvcRelation(pvbList, backup.Spec.Namespace, backup.Spec.Selector)
	if err != nil {
		return c.updateStatusToFailed(ctx, restore, err, "rebuild pvc and pod relation failed", log)
	}

	if len(currPvbList) > 0 {
		for _, pi := range currPvbList {
			volumeRestorer.AddTask(c.namespace, &pi.pvb, pi.podInfo)
		}

		if err = volumeRestorer.WaitSync(c.ctx); err != nil {
			return c.updateStatusToFailed(ctx, restore, err, "volume restore task failed", log)
		}
	}

	log.Infoln("restore completed")
	restore.Status.Phase = quchengv1beta1.RestorePhaseCompleted
	return c.kbClient.Status().Update(context.TODO(), restore)
}

func (c *RestoreController) updateStatusToFailed(ctx context.Context, resotre *quchengv1beta1.Restore, err error, msg string, log logrus.FieldLogger) error {
	original := resotre.DeepCopy()
	resotre.Status.Phase = quchengv1beta1.RestorePhaseFailed
	resotre.Status.Message = errors.WithMessage(err, msg).Error()
	resotre.Status.CompletionTimestamp = &metav1.Time{Time: c.clock.Now()}

	log.WithError(err).Error(msg)

	if err = c.kbClient.Status().Patch(ctx, resotre, client.MergeFrom(original)); err != nil {
		log.WithError(err).Error("error updating Restore status")
		return err
	}

	return err
}

func (c *RestoreController) rebuildPodPvcRelation(pvblist *velerov1.PodVolumeBackupList, namespace string, selector client.MatchingLabels) (map[string]*pvbInfo, error) {
	var podList v1.PodList
	if err := c.kbClient.List(context.TODO(), &podList, client.InNamespace(namespace), selector); err != nil {
		return nil, err
	}

	var pvcMap = make(map[string]*pvbInfo)
	for _, pvb := range pvblist.Items {
		pvcName := pvb.Labels[velerov1.PVCUIDLabel]
		pvcMap[pvcName] = &pvbInfo{
			pvb: pvb, pvcName: pvcName,
			podInfo: pvb.Spec.Pod, confirmed: false,
		}
	}

	for _, pod := range podList.Items {
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				pvcName := vol.PersistentVolumeClaim.ClaimName
				if pi, ok := pvcMap[pvcName]; ok {
					if pi.confirmed {
						continue
					}
					if pi.podInfo.Name != pod.Name {
						pi.podInfo = v1.ObjectReference{
							Kind:      "Pod",
							Namespace: namespace,
							Name:      pod.Name,
							UID:       pod.UID,
						}
					}
					pi.confirmed = true
				}
			}
		}
	}

	return pvcMap, nil
}

type pvbInfo struct {
	pvb       velerov1.PodVolumeBackup
	pvcName   string
	podInfo   v1.ObjectReference
	confirmed bool
}
