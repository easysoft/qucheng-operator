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
