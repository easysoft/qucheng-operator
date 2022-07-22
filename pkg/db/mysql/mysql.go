// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package mysql

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/easysoft/qucheng-operator/pkg/kube"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	"github.com/easysoft/qucheng-operator/pkg/db"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Parser struct {
	c         client.Client
	obj       *quchengv1beta1.Db
	dbService *quchengv1beta1.DbService

	logger logrus.FieldLogger
}

func NewParser(c client.Client, obj *quchengv1beta1.Db, logger logrus.FieldLogger) db.InterFace {
	return &Parser{
		c: c, obj: obj,
		logger: logger,
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
	data.DbType = string(dbService.Spec.Type)

	svcSpec := dbService.Spec.Service
	svcKey := client.ObjectKey{Name: svcSpec.Name, Namespace: svcSpec.Namespace}
	if svcKey.Namespace == "" {
		svcKey.Namespace = p.obj.Namespace
	}

	svc := &v1.Service{}
	if err := p.c.Get(context.TODO(), svcKey, svc); err != nil {
		return nil, err
	}

	port := kube.ReadServicePort(svc, svcSpec.Port)
	if port == 0 {
		return nil, errors.New("parse port failed")
	}

	data.Host = fmt.Sprintf("%s.%s.svc", svc.Name, svc.Namespace)
	data.Port = port

	userRef := dbService.Spec.Account.User
	passwdRef := dbService.Spec.Account.Password
	user, err := kube.ReadValueSource(p.c, dbServiceKey.Namespace, kube.NewValueRef(userRef.Value, userRef.ValueFrom))
	if err != nil {
		return nil, err
	}
	passwd, err := kube.ReadValueSource(p.c, dbServiceKey.Namespace, kube.NewValueRef(passwdRef.Value, passwdRef.ValueFrom))
	if err != nil {
		return nil, err
	}

	data.User = user
	data.Password = passwd

	p.logger.Infof("parse accessinfo host %s, port %d, user %s, password %s",
		data.Host, data.Port, data.User,
		hiddenPassword(data.Password))
	return &data, nil
}

func hiddenPassword(s string) string {
	frames := strings.Split(s, "")
	length := len(frames)
	return fmt.Sprintf("%s%s%s", frames[0], strings.Repeat("*", length-2), frames[length-1])
}
