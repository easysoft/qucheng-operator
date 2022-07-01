package mysql

import (
	"bytes"
	"fmt"
	"gitlab.zcorp.cc/pangu/cne-operator/pkg/db"
	"io/ioutil"
	"os"
	"os/exec"
)

type RestoreAction struct {
	*db.AccessInfo
	dbName string

	restoreFile string
	errLogFile  string

	errMessage string
}

func NewRestoreRequest(access *db.AccessInfo, dbName string, restoreFile string) *RestoreAction {

	_ = os.MkdirAll(backupRoot, 0755)

	return &RestoreAction{
		AccessInfo:  access,
		dbName:      dbName,
		restoreFile: restoreFile,
	}
}

func (r *RestoreAction) Run() error {
	var err error
	extraFile := r.buildExtraFile()
	commandArgs := []string{"--defaults-extra-file=" + extraFile, "--database", r.dbName}

	f, err := os.Open(r.restoreFile)
	if err != nil {
		return err
	}

	stderr, _ := ioutil.TempFile("/tmp", "")
	defer func() {
		os.Remove(extraFile)
		if err == nil {
			os.Remove(stderr.Name())
		}
	}()

	cmd := exec.Command("mysql", commandArgs...)
	cmd.Stdin = f
	cmd.Stderr = stderr
	err = cmd.Run()

	if err != nil {
		stderr.Seek(0, 0)
		fileStat, _ := stderr.Stat()
		errMessage := make([]byte, fileStat.Size())
		stderr.Read(errMessage)
		r.errMessage = string(errMessage)
	}
	return err
}

func (r *RestoreAction) Errors() string {
	return r.errMessage
}

func (r *RestoreAction) buildExtraFile() string {
	buf := bytes.NewBufferString("[client]\n")
	buf.WriteString(fmt.Sprintf("host = %s\n", r.Host))
	buf.WriteString(fmt.Sprintf("port = %d\n", r.Port))
	buf.WriteString(fmt.Sprintf("user = %s\n", r.User))
	buf.WriteString(fmt.Sprintf("password = %s\n", r.Password))

	tmpExtraFile, err := ioutil.TempFile("/tmp", "")
	if err != nil {
		fmt.Println(err)
	}

	buf.WriteTo(tmpExtraFile)

	return tmpExtraFile.Name()
}
