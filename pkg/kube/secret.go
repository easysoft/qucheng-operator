package kube

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetSecretKey(c client.Client, namespace string, secretSelector *v1.SecretKeySelector) (string, error) {
	var err error
	secret := &v1.Secret{}
	secretKey := client.ObjectKey{Name: secretSelector.Name, Namespace: namespace}
	err = c.Get(context.TODO(), secretKey, secret)
	if err != nil {
		return "", err
	}
	if data, ok := secret.Data[secretSelector.Key]; ok {
		return string(data), nil
	}
	return "", fmt.Errorf("key %s not found in secret %s", secretSelector.Key, secretSelector.Name)
}
