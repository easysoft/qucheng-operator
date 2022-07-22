package db

import (
	"context"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	dbmanage "github.com/easysoft/qucheng-operator/pkg/db/manage"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func recycleDB(c client.Client, obj *quchengv1beta1.Db) error {
	m, dbMeta, err := dbmanage.ParseDB(context.TODO(), c, obj)
	if err != nil {
		return err
	}
	if err = m.RecycleDB(dbMeta, true); err != nil {
		return err
	}
	return nil
}
