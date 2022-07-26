// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package kube

import (
	"context"
	"errors"
	"fmt"

	"github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type valueRefer interface {
	Default() string
	Source() *v1beta1.ValueSource
}

type valueRef struct {
	value  string
	source *v1beta1.ValueSource
}

func NewValueRef(value string, source *v1beta1.ValueSource) valueRefer {
	return &valueRef{
		value:  value,
		source: source,
	}
}

func (v *valueRef) Default() string {
	return v.value
}

func (v *valueRef) Source() *v1beta1.ValueSource {
	return v.source
}

func ReadValueSource(c client.Client, namespace string, ref valueRefer) (string, error) {
	ctx := context.TODO()
	source := ref.Source()
	if source != nil {
		if source.ConfigMapKeyRef != nil {
			return getConfigMapRefValue(ctx, c, namespace, source.ConfigMapKeyRef)
		} else if source.SecretKeyRef != nil {
			return getSecretRefValue(ctx, c, namespace, source.SecretKeyRef)
		}
	}
	if ref.Default() != "" {
		return ref.Default(), nil
	} else {
		return "", errors.New("no resource ref defined")
	}
}

// getSecretRefValue returns the value of a secret in the supplied namespace
func getSecretRefValue(ctx context.Context, c client.Client, namespace string, secretSelector *v1.SecretKeySelector) (string, error) {
	var err error
	secret := &v1.Secret{}
	secretKey := client.ObjectKey{Name: secretSelector.Name, Namespace: namespace}
	err = c.Get(ctx, secretKey, secret)
	if err != nil {
		return "", err
	}
	if data, ok := secret.Data[secretSelector.Key]; ok {
		return string(data), nil
	}
	return "", fmt.Errorf("key %s not found in secret %s", secretSelector.Key, secretSelector.Name)

}

// getConfigMapRefValue returns the value of a configmap in the supplied namespace
func getConfigMapRefValue(ctx context.Context, c client.Client, namespace string, configMapSelector *v1.ConfigMapKeySelector) (string, error) {
	var err error
	configMap := &v1.Secret{}
	configMapKey := client.ObjectKey{Name: configMapSelector.Name, Namespace: namespace}
	err = c.Get(ctx, configMapKey, configMap)
	if err != nil {
		return "", err
	}
	if data, ok := configMap.Data[configMapSelector.Key]; ok {
		return string(data), nil
	}
	return "", fmt.Errorf("key %s not found in config map %s", configMapSelector.Key, configMapSelector.Name)
}
