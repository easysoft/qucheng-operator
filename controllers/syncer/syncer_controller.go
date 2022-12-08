// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package syncer

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/easysoft/qucheng-operator/pkg/logging"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName     = "syncer-controller"
	minRequeueDuration = time.Second * 10

	labelResourceSyncKey   = "easycorp.io/resource_sync"
	labelResourceSyncValue = "true"
)

func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	recorder := mgr.GetEventRecorderFor(controllerName)
	return &SyncerReconciler{
		Logger:        logging.DefaultLogger().WithField("controller", controllerName),
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: recorder,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	// Watch for changes to DbService
	err = c.Watch(&source.Kind{Type: &corev1.Namespace{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &SyncerReconciler{}

// DbServiceReconciler reconciles a DbService object
type SyncerReconciler struct {
	client.Client
	Logger        *logrus.Entry
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GlobalDB object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *SyncerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Logger.Infof("start reconcile for namespace: %s", req.Name)
	logger := r.Logger.WithField("key", req.Name)
	// fetch ns
	ns := &corev1.Namespace{}
	err := r.Get(ctx, req.NamespacedName, ns)
	if err != nil {
		logger.WithError(err).Errorf("reconcile get req failed")
		if !errors.IsNotFound(err) {
			return reconcile.Result{}, fmt.Errorf("failed to get ns %s: %v", req.NamespacedName.Name, err)
		}
		ns = nil
	}
	if ns == nil || ns.DeletionTimestamp != nil {
		logger.Info("namespace is deleted")
		return reconcile.Result{}, nil
	}

	logger.Debugf("start sync resources")
	sourceNamespace := viper.GetString("pod-namespace")
	if ns.Name == sourceNamespace {
		return ctrl.Result{}, nil
	}

	logger.Debugf("start sync secrets from %s to %s", sourceNamespace, ns.Name)
	err = r.syncLabeledSecrets(sourceNamespace, ns.Name)
	if err != nil {
		logger.WithError(err).Error("sync secrets failed")
		return ctrl.Result{RequeueAfter: minRequeueDuration}, err
	}

	logger.Debugf("start sync configmaps from %s to %s", sourceNamespace, ns.Name)
	err = r.syncLabeledConfigMaps(sourceNamespace, ns.Name)
	if err != nil {
		logger.WithError(err).Error("sync configmaps failed")
		return ctrl.Result{RequeueAfter: minRequeueDuration}, err
	}

	logger.Debugf("start update sa imagePullSecrets")
	err = r.updateSaSecrets(ns.Name)
	if err != nil {
		logger.WithError(err).Error("update sa imagePullSecrets failed")
		return ctrl.Result{RequeueAfter: minRequeueDuration}, err
	}
	return ctrl.Result{RequeueAfter: 9 * minRequeueDuration}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *SyncerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Complete(r)
}

func (r *SyncerReconciler) syncLabeledConfigMaps(source, dest string) error {
	var cmList corev1.ConfigMapList
	err := r.Client.List(context.TODO(), &cmList, client.InNamespace(source), client.MatchingLabels{labelResourceSyncKey: labelResourceSyncValue})
	if err != nil {
		return err
	}

	for _, cm := range cmList.Items {
		var targetCm corev1.ConfigMap
		err = r.Client.Get(context.TODO(), client.ObjectKey{Namespace: dest, Name: cm.Name}, &targetCm)
		if err != nil {
			targetCm = corev1.ConfigMap{
				ObjectMeta: v1.ObjectMeta{
					Name: cm.Name, Namespace: dest,
				},
				Immutable:  cm.Immutable,
				Data:       cm.Data,
				BinaryData: cm.BinaryData,
			}
			targetCm.Namespace = dest
			err = r.Client.Create(context.TODO(), &targetCm)
			if err != nil {
				return err
			}
			continue
		}

		if !reflect.DeepEqual(cm.Data, &targetCm.Data) {
			targetCm.Data = cm.Data
			err = r.Client.Update(context.TODO(), &targetCm)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *SyncerReconciler) syncLabeledSecrets(source, dest string) error {
	var secretList corev1.SecretList
	err := r.Client.List(context.TODO(), &secretList, client.InNamespace(source), client.MatchingLabels{labelResourceSyncKey: labelResourceSyncValue})
	if err != nil {
		return err
	}

	for _, secret := range secretList.Items {
		var targetSecret corev1.Secret
		err = r.Client.Get(context.TODO(), client.ObjectKey{Namespace: dest, Name: secret.Name}, &targetSecret)
		if err != nil {
			targetSecret = corev1.Secret{
				ObjectMeta: v1.ObjectMeta{
					Name: secret.Name, Namespace: dest,
				},
				Immutable:  secret.Immutable,
				Data:       secret.Data,
				StringData: secret.StringData,
				Type:       secret.Type,
			}
			targetSecret.Namespace = dest
			err = r.Client.Create(context.TODO(), &targetSecret)
			if err != nil {
				return err
			}
			continue
		}

		if !reflect.DeepEqual(secret.Data, &targetSecret.Data) {
			targetSecret.Data = secret.Data
			err = r.Client.Update(context.TODO(), &targetSecret)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *SyncerReconciler) updateSaSecrets(namespace string) error {
	var secretList corev1.SecretList
	err := r.Client.List(context.TODO(), &secretList, client.InNamespace(namespace))
	if err != nil {
		return err
	}

	var sa corev1.ServiceAccount
	err = r.Client.Get(context.TODO(), client.ObjectKey{Name: "default", Namespace: namespace}, &sa)
	if err != nil {
		return err
	}

	appendSecretNames := make([]string, 0)

	for _, secret := range secretList.Items {
		var found bool
		if secret.Type != corev1.SecretTypeDockerConfigJson {
			continue
		}
		for _, s := range sa.ImagePullSecrets {
			if s.Name == secret.Name {
				found = true
				break
			}
		}
		if !found {
			appendSecretNames = append(appendSecretNames, secret.Name)
		}
	}

	if len(appendSecretNames) > 0 {
		for _, name := range appendSecretNames {
			sa.ImagePullSecrets = append(sa.ImagePullSecrets, corev1.LocalObjectReference{Name: name})
		}
		return r.Client.Update(context.TODO(), &sa)
	}
	return nil
}
