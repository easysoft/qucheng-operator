// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package manage

import (
	"io"
	"os"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
)

type DbManager interface {
	DbType() quchengv1beta1.DbType
	ServerInfo() DbServerInfo
	IsValid(meta *DbMeta) error
	CreateDB(meta *DbMeta) error
	RecycleDB(meta *DbMeta, real bool) error
	Dump(meta *DbMeta) (*os.File, error)
	Restore(meta *DbMeta, input io.Reader) error
}

type DbServerInfo interface {
	Host() string
	Port() int32
	Address() string
}

type DbServiceMeta struct {
	Type          quchengv1beta1.DbType
	Host          string
	Port          int32
	AdminUser     string
	AdminPassword string
}

type DbMeta struct {
	Name     string
	User     string
	Password string
	Config   map[string]string
}
