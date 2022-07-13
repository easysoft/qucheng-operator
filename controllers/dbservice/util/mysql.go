// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package util

import (
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type MysqlMeta struct {
	DBMeta
}

func (mysql MysqlMeta) GenHost() string {
	ns := mysql.Spec.Service.Namespace
	if ns == "" {
		ns = mysql.Namespace
	}
	return fmt.Sprintf("%s.%s.svc", mysql.Spec.Service.Name, ns)
}

func (mysql MysqlMeta) GenPort() string {
	port := mysql.Spec.Service.Port.IntValue()
	if port == 0 {
		port = 3306
	}
	return strconv.Itoa(port)
}

func (mysql MysqlMeta) GenDsn() string {
	host := fmt.Sprintf("%s:%s", mysql.GenHost(), mysql.GenPort())
	return mysql.Spec.Account.User.Value + ":" + mysql.Spec.Account.Password.Value + "@tcp(" + host + ")/"
}

func (mysql MysqlMeta) CheckNetWork() error {
	address := net.JoinHostPort(mysql.GenHost(), mysql.GenPort())
	_, err := net.DialTimeout("tcp", address, 5*time.Second)
	return err
}

func (mysql MysqlMeta) CheckAuth() error {
	db, err := sql.Open("mysql", mysql.GenDsn())
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Ping()
}

// func (mysql MysqlMeta) CreateDB(dbname, dbuser, dbpass string) error {
// 	dbclient, err := sql.Open("mysql", mysql.genDsn())
// 	if err != nil {
// 		return err
// 	}
// 	defer dbclient.Close()
// 	_, err = dbclient.Exec("CREATE DATABASE IF NOT EXISTS " + dbname + ";")
// 	if err != nil {
// 		return fmt.Errorf("创建数据库失败, err: %v", err)
// 	}
// 	_, err = dbclient.Exec("use " + dbname)
// 	if err != nil {
// 		return fmt.Errorf("创建数据库失败, err: %v", err)
// 	}
// 	_, err = dbclient.Exec("CREATE USER '" + dbuser + "'@'%' IDENTIFIED BY '" + dbpass + "';")
// 	if err != nil {
// 		return fmt.Errorf("创建用户失败, err: %v", err)
// 	}
// 	grantCmd := fmt.Sprintf("GRANT ALL ON %s.* TO '%s'@'%%'", dbname, dbuser)
// 	_, err = dbclient.Exec(grantCmd)
// 	if err != nil {
// 		return fmt.Errorf("授权失败, err: %v", err)
// 	}
// 	_, err = dbclient.Exec("flush privileges;")
// 	if err != nil {
// 		return fmt.Errorf("刷新权限失败, err: %v", err)
// 	}
// 	return nil
// }

// func (mysql MysqlMeta) Drop(dbname, dbuser string) error {
// 	dbclient, err := sql.Open("mysql", mysql.genDsn())
// 	if err != nil {
// 		return err
// 	}
// 	defer dbclient.Close()
// 	// 移除权限
// 	revokeCmd := fmt.Sprintf("REVOKE ALL ON %s.* FROM '%s'@'%%';", dbname, dbuser)
// 	_, err = dbclient.Exec(revokeCmd)
// 	if err != nil {
// 		logrus.Warnf("revoke user %v err: %v, sql: %v", dbuser, err, revokeCmd)
// 	}
// 	logrus.Debugf("revoke user %v", dbuser)
// 	// 删除用户
// 	dropUserCmd := fmt.Sprintf("DROP USER IF EXISTS \"%v\";", dbuser)
// 	_, err = dbclient.Exec(dropUserCmd)
// 	if err != nil {
// 		logrus.Errorf("delete user %v err: %v, sql: %v", dbuser, err, dropUserCmd)
// 		return err
// 	}
// 	logrus.Debugf("delete user %v", dbuser)
// 	// 删除数据库
// 	dropDBCmd := fmt.Sprintf("DROP DATABASE IF EXISTS %v;", dbname)
// 	_, err = dbclient.Exec(dropDBCmd)
// 	if err != nil {
// 		logrus.Errorf("delete db %v err: %v, sql: %v", dbname, err, dropDBCmd)
// 		return err
// 	}
// 	logrus.Debugf("delete db %v", dbname)
// 	_, err = dbclient.Exec("flush privileges;")
// 	if err != nil {
// 		logrus.Errorf("flush privileges err: %v", err)
// 		return err
// 	}
// 	logrus.Debug("刷新权限")
// 	return nil
// }
