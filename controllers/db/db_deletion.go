// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"github.com/sirupsen/logrus"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	dbmanage "github.com/easysoft/qucheng-operator/pkg/db/manage"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func recycleDB(c client.Client, obj *quchengv1beta1.Db, logger logrus.FieldLogger) error {
	m, dbMeta, err := dbmanage.ParseDB(context.TODO(), c, obj, logger)
	if err != nil {
		return err
	}
	if err = m.RecycleDB(dbMeta, true); err != nil {
		return err
	}
	return nil
}
