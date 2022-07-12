// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package util

import (
	"net"
	"strconv"
	"time"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
)

type DB interface {
	CheckNetWork() error
	CheckAuth() error
}

type DBMeta struct {
	quchengv1beta1.GlobalDBSpec
}

func (m DBMeta) CheckNetWork() error {
	address := net.JoinHostPort(m.Source.Host, strconv.Itoa(m.Source.Port))
	_, err := net.DialTimeout("tcp", address, 5*time.Second)
	return err
}

func NewDB(gdb quchengv1beta1.GlobalDBSpec) DB {
	if gdb.Type == "mysql" {
		return &MysqlMeta{DBMeta: DBMeta{GlobalDBSpec: gdb}}
	}
	return nil
}
