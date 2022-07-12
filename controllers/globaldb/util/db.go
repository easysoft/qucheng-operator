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
