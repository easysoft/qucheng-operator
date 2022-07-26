// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package manage

import (
	"database/sql"
	"fmt"

	quchengv1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/json"

	_ "github.com/go-sql-driver/mysql"
)

const (
	_mysqlDriver = "mysql"

	_defaultCharacterSet = "utf8mb4"
)

type mysqlManage struct {
	meta DbServiceMeta
}

func newMysqlManager(meta DbServiceMeta) (DbManager, error) {
	return &mysqlManage{
		meta: meta,
	}, nil
}

func (m *mysqlManage) DbType() quchengv1beta1.DbType {
	return m.meta.Type
}

func (m *mysqlManage) ServerInfo() DbServerInfo {
	return &serverInfo{
		host: m.meta.Host, port: m.meta.Port,
	}
}

func (m *mysqlManage) IsValid(meta *DbMeta) error {
	dsn := m.genBusinessDsn(meta)
	dbClient, err := sql.Open(_mysqlDriver, dsn)
	if err != nil {
		return err
	}
	defer dbClient.Close()

	return dbClient.Ping()
}
func (m *mysqlManage) CreateDB(meta *DbMeta) error {
	dsn := m.genAdminDsn()
	dbClient, err := sql.Open(_mysqlDriver, dsn)
	if err != nil {
		return err
	}
	defer dbClient.Close()

	var config MysqlConfig
	bs, _ := json.Marshal(meta.Config)
	_ = json.Unmarshal(bs, &config)

	character := _defaultCharacterSet
	if config.CharacterSet != "" {
		character = config.CharacterSet
	}
	_, err = dbClient.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET %s;", meta.Name, character))
	if err != nil {
		return fmt.Errorf("create db err: %v", err)
	}
	_, err = dbClient.Exec(fmt.Sprintf("use `%s`", meta.Name))
	if err != nil {
		return fmt.Errorf("use db err: %v", err)
	}
	_, err = dbClient.Exec(fmt.Sprintf("CREATE USER `%s`@'%%' IDENTIFIED BY '%s';", meta.User, meta.Password))
	if err != nil {
		return fmt.Errorf("crea db user err: %v", err)
	}
	grantCmd := fmt.Sprintf("GRANT ALL ON `%s`.* TO `%s`@'%%'", meta.Name, meta.User)
	_, err = dbClient.Exec(grantCmd)
	if err != nil {
		return fmt.Errorf("grant user err: %v", err)
	}
	if config.GrantSuperPrivilege == "true" {
		superCmd := fmt.Sprintf("UPDATE mysql.user SET Super_Priv='Y' WHERE user='%s' AND host='%%';", meta.User)
		_, err = dbClient.Exec(superCmd)
		if err != nil {
			return fmt.Errorf("grant super privileges to user '%s' err: %v", meta.User, err)
		}
	}

	_, err = dbClient.Exec("flush privileges;")
	if err != nil {
		return fmt.Errorf("flush privileges err: %v", err)
	}

	return nil
}
func (m *mysqlManage) RecycleDB(meta *DbMeta, real bool) error {
	dsn := m.genAdminDsn()
	dbClient, err := sql.Open(_mysqlDriver, dsn)
	if err != nil {
		return err
	}
	defer dbClient.Close()

	// revoke privileges
	revokeCmd := fmt.Sprintf("REVOKE ALL ON `%s`.* FROM '%s'@'%%';", meta.Name, meta.User)
	_, err = dbClient.Exec(revokeCmd)
	if err != nil {
		return fmt.Errorf("revoke user %v err: %v, sql: %v", meta.User, err, revokeCmd)
	}
	logrus.Debugf("revoke user %v", meta.User)

	// remove user
	dropUserCmd := fmt.Sprintf("DROP USER IF EXISTS \"%s\";", meta.User)
	_, err = dbClient.Exec(dropUserCmd)
	if err != nil {
		return fmt.Errorf("delete user %v err: %v, sql: %v", meta.User, err, dropUserCmd)
	}
	logrus.Debugf("delete user %v", meta.User)

	// drop database
	dropDBCmd := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`;", meta.Name)
	_, err = dbClient.Exec(dropDBCmd)
	if err != nil {
		return fmt.Errorf("delete db %v err: %v, sql: %v", meta.Name, err, dropDBCmd)
	}
	logrus.Debugf("delete db %v", meta.Name)
	_, err = dbClient.Exec("flush privileges;")
	if err != nil {
		return fmt.Errorf("flush privileges err: %v", err)
	}
	return nil
}

func (m *mysqlManage) genAdminDsn() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/?charset=utf8mb4&parseTime=True&loc=Local",
		m.meta.AdminUser, m.meta.AdminPassword, m.meta.Host, m.meta.Port)
}

func (m *mysqlManage) genBusinessDsn(dbMeta *DbMeta) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		dbMeta.User, dbMeta.Password, m.meta.Host, m.meta.Port, dbMeta.Name)
}
