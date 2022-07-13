// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package util

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	"github.com/sirupsen/logrus"
)

type MysqlMeta struct {
	DBMeta
}

func (mysql MysqlMeta) CheckRootAuth() error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/?charset=utf8mb4&parseTime=True&loc=Local",
		mysql.RootUser, mysql.RootPass, mysql.Address)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Ping()
}

func (mysql MysqlMeta) CheckChildAuth() error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		mysql.ChildUser, mysql.ChildPass, mysql.Address, mysql.ChildName)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Ping()
}

func (mysql MysqlMeta) CreateDB() error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/?charset=utf8mb4&parseTime=True&loc=Local",
		mysql.RootUser, mysql.RootPass, mysql.Address)
	dbclient, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer dbclient.Close()
	_, err = dbclient.Exec("CREATE DATABASE IF NOT EXISTS " + mysql.ChildName + ";")
	if err != nil {
		return fmt.Errorf("create db err: %v", err)
	}
	_, err = dbclient.Exec("use " + mysql.ChildName)
	if err != nil {
		return fmt.Errorf("use db err: %v", err)
	}
	_, err = dbclient.Exec("CREATE USER '" + mysql.ChildUser + "'@'%' IDENTIFIED BY '" + mysql.ChildPass + "';")
	if err != nil {
		return fmt.Errorf("crea db user err: %v", err)
	}
	grantCmd := fmt.Sprintf("GRANT ALL ON %s.* TO '%s'@'%%'", mysql.ChildName, mysql.ChildUser)
	_, err = dbclient.Exec(grantCmd)
	if err != nil {
		return fmt.Errorf("grant user err: %v", err)
	}
	_, err = dbclient.Exec("flush privileges;")
	if err != nil {
		return fmt.Errorf("flush privileges err: %v", err)
	}
	return nil
}
func (mysql MysqlMeta) DropDB() error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/?charset=utf8mb4&parseTime=True&loc=Local",
		mysql.RootUser, mysql.RootPass, mysql.Address)
	dbclient, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer dbclient.Close()
	// 移除权限
	revokeCmd := fmt.Sprintf("REVOKE ALL ON %s.* FROM '%s'@'%%';", mysql.ChildName, mysql.ChildUser)
	_, err = dbclient.Exec(revokeCmd)
	if err != nil {
		return fmt.Errorf("revoke user %v err: %v, sql: %v", mysql.ChildUser, err, revokeCmd)
	}
	logrus.Debugf("revoke user %v", mysql.ChildUser)

	// 删除用户
	dropUserCmd := fmt.Sprintf("DROP USER IF EXISTS \"%v\";", mysql.ChildUser)
	_, err = dbclient.Exec(dropUserCmd)
	if err != nil {
		return fmt.Errorf("delete user %v err: %v, sql: %v", mysql.ChildUser, err, dropUserCmd)
	}
	logrus.Debugf("delete user %v", mysql.ChildUser)
	// 删除数据库
	dropDBCmd := fmt.Sprintf("DROP DATABASE IF EXISTS %v;", mysql.ChildName)
	_, err = dbclient.Exec(dropDBCmd)
	if err != nil {
		return fmt.Errorf("delete db %v err: %v, sql: %v", mysql.ChildName, err, dropDBCmd)
	}
	logrus.Debugf("delete db %v", mysql.ChildName)
	_, err = dbclient.Exec("flush privileges;")
	if err != nil {
		return fmt.Errorf("flush privileges err: %v", err)
	}
	return nil
}
