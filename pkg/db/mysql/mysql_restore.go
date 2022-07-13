// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package mysql

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/easysoft/qucheng-operator/pkg/db"
)

type RestoreAction struct {
	*db.AccessInfo
	dbName string

	restoreFile *os.File
	errLogFile  string

	errMessage string
}

func NewRestoreRequest(access *db.AccessInfo, dbName string, restoreFile *os.File) *RestoreAction {

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

	stderr, _ := ioutil.TempFile("/tmp", "")
	defer func() {
		os.Remove(extraFile)
		if err == nil {
			os.Remove(stderr.Name())
		}
	}()

	cmd := exec.Command("mysql", commandArgs...)
	cmd.Stdin = r.restoreFile
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
