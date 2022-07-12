package util

import (
	"database/sql"
	"strconv"

	_ "github.com/go-sql-driver/mysql"
)

type MysqlMeta struct {
	DBMeta
}

func (mysql MysqlMeta) CheckAuth() error {
	db, err := sql.Open("mysql", mysql.Source.User+":"+mysql.Source.Pass+"@tcp("+mysql.Source.Host+":"+strconv.Itoa(mysql.Source.Port)+")/")
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Ping()
}
