package main

import (
	"context"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
	"path/filepath"
	"reflect"
)

func loadAllCrds(ctx context.Context, config *rest.Config, directory string, logger logrus.FieldLogger) error {
	client, err := clientset.NewForConfig(config)
	if err != nil {
		return err
	}

	files, err := filepath.Glob(directory + "/*.yaml")
	if err != nil {
		return err
	}

	for _, file := range files {
		if err = installCrd(ctx, client, file, logger); err != nil {
			return err
		}
	}
	return nil
}

func installCrd(ctx context.Context, client clientset.Interface, crdPath string, logger logrus.FieldLogger) error {
	var crd v1.CustomResourceDefinition

	logger.Debugf("read crd manifest %s", crdPath)
	content, err := ioutil.ReadFile(crdPath)
	if err != nil {
		return err
	}

	if err = yaml.Unmarshal(content, &crd); err != nil {
		return err
	}

	logger = logger.WithField("crd", crd.Name)
	logger.Debug("check crd upgradeable")
	obj, err := client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
	if err != nil {
		logger.Info("crd not found, will do install")
		_, err = client.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, &crd, metav1.CreateOptions{})
		return err
	}

	metaChanged := removeHelmOwner(obj)

	if !reflect.DeepEqual(obj.Spec, crd.Spec) || metaChanged {
		obj.Spec = crd.Spec
		logger.Info("crd changed, will do upgrade")
		_, err = client.ApiextensionsV1().CustomResourceDefinitions().Update(ctx, obj, metav1.UpdateOptions{})
		return err
	}

	return nil
}

func removeHelmOwner(crd *v1.CustomResourceDefinition) bool {
	var requireUpdate bool
	if _, ok := crd.Labels["app.kubernetes.io/managed-by"]; ok {
		delete(crd.Labels, "app.kubernetes.io/managed-by")
		requireUpdate = true
	}

	if _, ok := crd.Annotations["meta.helm.sh/release-name"]; ok {
		delete(crd.Annotations, "meta.helm.sh/release-name")
		requireUpdate = true
	}

	if _, ok := crd.Annotations["meta.helm.sh/release-namespace"]; ok {
		delete(crd.Annotations, "meta.helm.sh/release-namespace")
		requireUpdate = true
	}

	if _, ok := crd.Annotations["helm.sh/resource-policy"]; !ok {
		crd.Annotations["helm.sh/resource-policy"] = "keep"
		requireUpdate = true
	}

	return requireUpdate
}
