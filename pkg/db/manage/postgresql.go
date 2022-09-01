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
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
)

const (
	_postgresqlDriver = "postgres"
)

type postgresqlManage struct {
	meta   DbServiceMeta
	logger logrus.FieldLogger
}

func newPostgresqlManager(meta DbServiceMeta, logger logrus.FieldLogger) (DbManager, error) {
	return &postgresqlManage{
		meta:   meta,
		logger: logger,
	}, nil
}

func (m *postgresqlManage) DbType() quchengv1beta1.DbType {
	return m.meta.Type
}

func (m *postgresqlManage) ServerInfo() DbServerInfo {
	return &serverInfo{
		host: m.meta.Host, port: m.meta.Port,
		user: m.meta.AdminUser, passwd: m.meta.AdminPassword,
	}
}

func (m *postgresqlManage) IsValid(meta *DbMeta) error {
	dsn := m.genBusinessDsn(meta)
	dbClient, err := sql.Open(_postgresqlDriver, dsn)
	if err != nil {
		return err
	}
	defer dbClient.Close()

	return dbClient.Ping()
}
func (m *postgresqlManage) CreateDB(meta *DbMeta) error {
	dsn := m.genAdminDsn()
	dbClient, err := sql.Open(_postgresqlDriver, dsn)
	if err != nil {
		return err
	}
	defer dbClient.Close()

	createDbCmd := fmt.Sprintf("CREATE DATABASE %s", meta.Name)
	m.logger.Debugf("execute sql: %s", createDbCmd)
	_, err = dbClient.Exec(createDbCmd)
	if err != nil {
		return fmt.Errorf("create db err: %v", err)
	}
	m.logger.Infof("created database %s", meta.Name)

	createUserCmd := fmt.Sprintf(createPgUser, meta.User, meta.User, meta.Password)
	m.logger.Debugf("execute sql %s", createUserCmd)
	_, err = dbClient.Exec(createUserCmd)
	if err != nil {
		return fmt.Errorf("crea user err: %v", err)
	}
	m.logger.Infof("created user %s", meta.Name)

	grantCmd := fmt.Sprintf(`GRANT ALL PRIVILEGES ON DATABASE "%s" TO "%s"`, meta.Name, meta.User)
	m.logger.Debugf("execute sql %s", grantCmd)
	_, err = dbClient.Exec(grantCmd)
	if err != nil {
		return fmt.Errorf("grant user err: %v", err)
	}
	m.logger.Infof("granted privileges to user %s", meta.User)

	alterOwnerCmd := fmt.Sprintf(`ALTER DATABASE "%s" OWNER TO "%s"`, meta.Name, meta.User)
	m.logger.Debugf("execute sql %s", alterOwnerCmd)
	_, err = dbClient.Exec(alterOwnerCmd)
	if err != nil {
		return fmt.Errorf("change db owner err: %v", err)
	}
	m.logger.Infof("altered owner to %s", meta.User)

	return nil
}
func (m *postgresqlManage) RecycleDB(meta *DbMeta, real bool) error {
	dsn := m.genAdminDsn()
	dbClient, err := sql.Open(_postgresqlDriver, dsn)
	if err != nil {
		return err
	}
	defer dbClient.Close()

	// revoke privileges
	revokeCmd := fmt.Sprintf(`REVOKE ALL PRIVILEGES ON DATABASE "%s" FROM %s`, meta.Name, meta.User)
	_, err = dbClient.Exec(revokeCmd)
	if err != nil {
		return fmt.Errorf("revoke user %v err: %v, sql: %v", meta.User, err, revokeCmd)
	}
	logrus.Debugf("revoke user %v", meta.User)

	// drop database
	dropDbCmd := fmt.Sprintf(`DROP DATABASE IF EXISTS "%s"`, meta.Name)
	_, err = dbClient.Exec(dropDbCmd)
	if err != nil {
		return fmt.Errorf("delete db %v err: %v, sql: %v", meta.Name, err, dropDbCmd)
	}
	logrus.Debugf("delete database %v", meta.Name)

	// drop role
	dropUserCmd := fmt.Sprintf(`DROP ROLE IF EXISTS "%s";`, meta.User)
	_, err = dbClient.Exec(dropUserCmd)
	if err != nil {
		return fmt.Errorf("delete user %v err: %v, sql: %v", meta.User, err, dropUserCmd)
	}
	logrus.Debugf("delete user %v", meta.Name)
	return nil
}

func (m *postgresqlManage) genAdminDsn() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/?sslmode=disable",
		m.meta.AdminUser, m.meta.AdminPassword, m.meta.Host, m.meta.Port)
}

func (m *postgresqlManage) genBusinessDsn(dbMeta *DbMeta) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		dbMeta.User, dbMeta.Password, m.meta.Host, m.meta.Port, dbMeta.Name)
}

const (
	createPgUser = `
DO
$body$
BEGIN
   IF NOT EXISTS (
      SELECT FROM pg_catalog.pg_roles WHERE rolname = '%s'
   ) THEN
      CREATE ROLE "%s" LOGIN PASSWORD '%s';
   END IF;
END
$body$
`
)
