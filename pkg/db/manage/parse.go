// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package manage

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	"github.com/easysoft/qucheng-operator/pkg/kube"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ParseDB(ctx context.Context, c client.Client, db *quchengv1beta1.Db, logger logrus.FieldLogger) (DbManager, *DbMeta, error) {
	var err error
	var dbSvc quchengv1beta1.DbService
	var dbMgr DbManager
	var dbMeta *DbMeta

	target := db.Spec.TargetService
	ns := target.Namespace
	if ns == "" {
		ns = db.Namespace
	}

	if err = c.Get(ctx, client.ObjectKey{Name: target.Name, Namespace: ns}, &dbSvc); err != nil {
		return nil, nil, err
	}
	dbMgr, err = ParseDbService(ctx, c, &dbSvc, logger)
	if err != nil {
		return nil, nil, err
	}

	dbUser, dbPass, err := readAccount(ctx, c, db.Namespace, &db.Spec.Account)
	if err != nil {
		dbMeta = getDbMetaFromCache(client.ObjectKeyFromObject(db).String())
		if dbMeta == nil {
			return nil, nil, err
		}
	} else {
		dbMeta = &DbMeta{
			Name:     db.Spec.DbName,
			User:     dbUser,
			Password: dbPass,
			Config:   db.Spec.Config,
		}
		addDbMetaToCache(client.ObjectKeyFromObject(db).String(), dbMeta)
	}

	return dbMgr, dbMeta, nil
}

func ParseDbService(ctx context.Context, c client.Client, dbSvc *quchengv1beta1.DbService, logger logrus.FieldLogger) (DbManager, error) {
	svc := dbSvc.Spec.Service
	host, port, err := readService(ctx, c, dbSvc.Namespace, &svc)
	if err != nil {
		return nil, err
	}

	dbUser, dbPass, err := readAccount(ctx, c, dbSvc.Namespace, &dbSvc.Spec.Account)
	if err != nil {
		return nil, err
	}

	meta := DbServiceMeta{
		Type:          dbSvc.Spec.Type,
		Host:          host,
		Port:          port,
		AdminUser:     dbUser,
		AdminPassword: dbPass,
	}

	switch meta.Type {
	case quchengv1beta1.DbTypeMysql:
		return newMysqlManager(meta)
	case quchengv1beta1.DbTypePostgresql:
		return newPostgresqlManager(meta, logger)
	default:
		return nil, errors.New("dbType is not supported")
	}
}

func readService(ctx context.Context, c client.Client, namespace string, svc *quchengv1beta1.Service) (string, int32, error) {
	targetNs := svc.Namespace
	if targetNs == "" {
		targetNs = namespace
	}
	host := fmt.Sprintf("%s.%s.svc", svc.Name, targetNs)

	var (
		port int32
		err  error
		s    v1.Service
	)

	if svc.Port.Type == intstr.Int {
		port = svc.Port.IntVal
	} else {
		err = c.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: targetNs}, &s)
		if err != nil {
			return "", 0, err
		}
		for _, p := range s.Spec.Ports {
			if p.Name == svc.Port.StrVal {
				port = p.Port
				break
			}
		}
	}

	if port == 0 {
		return "", 0, fmt.Errorf("parse service port '%s' failed", svc.Port.StrVal)
	}

	return host, port, nil
}

func readAccount(ctx context.Context, c client.Client, namespace string, info *quchengv1beta1.Account) (string, string, error) {
	user, err := kube.ReadValueSource(c, namespace, kube.NewValueRef(info.User.Value, info.User.ValueFrom))
	if err != nil {
		return "", "", err
	}
	passwd, err := kube.ReadValueSource(c, namespace, kube.NewValueRef(info.Password.Value, info.Password.ValueFrom))
	if err != nil {
		return "", "", err
	}
	return user, passwd, nil
}
