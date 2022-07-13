// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package util

type DB interface {
	CheckRootAuth() error
	CheckChildAuth() error
	CreateDB() error
	DropDB() error
}

type DBMeta struct {
	Type      string `json:"type"`
	Host      string `json:"host"`
	Port      string `json:"port"`
	Address   string `json:"address"`
	RootUser  string `json:"rootUser"`
	RootPass  string `json:"rootPass"`
	ChildName string `json:"childName"`
	ChildUser string `json:"childUser"`
	ChildPass string `json:"childPass"`
}

func NewDB(m DBMeta) DB {
	if m.Type == "mysql" {
		return &MysqlMeta{DBMeta: m}
	}
	return nil
}
