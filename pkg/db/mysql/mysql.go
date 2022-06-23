package mysql

import (
	"context"
	"errors"
	"fmt"
	quchengv1beta1 "gitlab.zcorp.cc/pangu/cne-operator/apis/qucheng/v1beta1"
	"gitlab.zcorp.cc/pangu/cne-operator/pkg/db"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Parser struct {
	c   client.Client
	obj *quchengv1beta1.Db
}

func NewParser(c client.Client, obj *quchengv1beta1.Db) db.InterFace {
	return &Parser{
		c: c, obj: obj,
	}
}

func (p *Parser) ParseAccessInfo() (*db.AccessInfo, error) {
	data := db.AccessInfo{}
	dbServiceKey := client.ObjectKey{Name: p.obj.Spec.TargetService.Name, Namespace: p.obj.Spec.TargetService.Namespace}
	if dbServiceKey.Namespace == "" {
		dbServiceKey.Namespace = p.obj.Namespace
	}

	dbService := &quchengv1beta1.DbService{}
	if err := p.c.Get(context.TODO(), dbServiceKey, dbService); err != nil {
		return nil, err
	}

	svcSpec := dbService.Spec.Service
	svcKey := client.ObjectKey{Name: svcSpec.Name, Namespace: svcSpec.Namespace}
	if svcKey.Namespace == "" {
		svcKey.Namespace = p.obj.Namespace
	}

	svc := &v1.Service{}
	if err := p.c.Get(context.TODO(), svcKey, svc); err != nil {
		return nil, err
	}

	var port int32
	if svcSpec.Port.Type == intstr.Int {
		port = svcSpec.Port.IntVal
	} else {
		for _, p := range svc.Spec.Ports {
			if p.Name == svcSpec.Port.StrVal {
				port = p.Port
				break
			}
		}
	}
	if port == 0 {
		return nil, errors.New("parse port failed")
	}

	data.Host = fmt.Sprintf("%s.%s.svc", svc.Name, svc.Namespace)
	data.Port = port

	user, err := getSourceValue(p.c, dbServiceKey.Namespace, dbService.Spec.Account.User.ValueFrom)
	if err != nil {
		return nil, err
	}
	data.User = user

	passwd, err := getSourceValue(p.c, dbServiceKey.Namespace, dbService.Spec.Account.Password.ValueFrom)
	if err != nil {
		return nil, err
	}
	data.Password = passwd
	return &data, nil
}

func getSourceValue(c client.Client, namespace string, v *quchengv1beta1.ValueSource) (string, error) {
	if v.ConfigMapKeyRef != nil {
		return getConfigMapRefValue(c, namespace, v.ConfigMapKeyRef)
	} else if v.SecretKeyRef != nil {
		return getSecretRefValue(c, namespace, v.SecretKeyRef)
	} else {
		return "", errors.New("no resource ref defined")
	}
}

// getSecretRefValue returns the value of a secret in the supplied namespace
func getSecretRefValue(c client.Client, namespace string, secretSelector *v1.SecretKeySelector) (string, error) {
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

// getConfigMapRefValue returns the value of a configmap in the supplied namespace
func getConfigMapRefValue(c client.Client, namespace string, configMapSelector *v1.ConfigMapKeySelector) (string, error) {
	var err error
	configMap := &v1.Secret{}
	configMapKey := client.ObjectKey{Name: configMapSelector.Name, Namespace: namespace}
	err = c.Get(context.TODO(), configMapKey, configMap)
	if err != nil {
		return "", err
	}
	if data, ok := configMap.Data[configMapSelector.Key]; ok {
		return string(data), nil
	}
	return "", fmt.Errorf("key %s not found in config map %s", configMapSelector.Key, configMapSelector.Name)
}
