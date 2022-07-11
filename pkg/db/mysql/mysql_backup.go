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
	"time"

	"github.com/easysoft/qucheng-operator/pkg/db"
)

const (
	backupRoot = "/tmp/backup"

	resourceType     = "mysql"
	backupNameFormat = resourceType + "." + "%s.%s"
)

type BackupAction struct {
	*db.AccessInfo
	dbName     string
	BackupName string
	BackupTime time.Time

	outputFile string
	errLogFile string

	errMessage string
	BackupFile BackupFile
}

type BackupFile struct {
	AbsPath  string
	FullPath string
	Fd       *os.File
}

func NewBackupRequest(access *db.AccessInfo, dbName string) *BackupAction {

	_ = os.MkdirAll(backupRoot, 0755)

	currTime := time.Now()
	backupName := fmt.Sprintf(backupNameFormat, dbName, currTime.Format("20060102150405"))
	return &BackupAction{
		AccessInfo: access,
		dbName:     dbName,
		BackupTime: currTime,
		BackupName: backupName,

		outputFile: fmt.Sprintf("%s.sql", backupName),
		errLogFile: fmt.Sprintf("%s.err", backupName),
	}
}

func (b *BackupAction) Run() error {
	var err error
	extraFile := b.buildExtraFile()
	commandArgs := []string{"--defaults-extra-file=" + extraFile, "--databases", b.dbName}

	f, err := os.Create(b.GenerateFullPath(b.outputFile))
	if err != nil {
		return err
	}

	b.BackupFile = BackupFile{AbsPath: b.outputFile, FullPath: f.Name(), Fd: f}

	stderr, _ := os.Create(b.GenerateFullPath(b.errLogFile))
	defer func() {
		os.Remove(extraFile)
		if err == nil {
			os.Remove(stderr.Name())
		}
	}()

	cmd := exec.Command("mysqldump", commandArgs...)
	cmd.Stdout = f
	cmd.Stderr = stderr
	err = cmd.Run()

	if err != nil {
		stderr.Seek(0, 0)
		fileStat, _ := stderr.Stat()
		errMessage := make([]byte, fileStat.Size())
		stderr.Read(errMessage)
		b.errMessage = string(errMessage)
	}
	return err
}

func (b *BackupAction) GenerateFullPath(path string) string {
	return backupRoot + "/" + path
}

func (b *BackupAction) Errors() string {
	return b.errMessage
}

func (b *BackupAction) buildExtraFile() string {
	buf := bytes.NewBufferString("[client]\n")
	buf.WriteString(fmt.Sprintf("host = %s\n", b.Host))
	buf.WriteString(fmt.Sprintf("port = %d\n", b.Port))
	buf.WriteString(fmt.Sprintf("user = %s\n", b.User))
	buf.WriteString(fmt.Sprintf("password = %s\n", b.Password))

	tmpExtraFile, err := ioutil.TempFile("/tmp", "")
	if err != nil {
		fmt.Println(err)
	}

	buf.WriteTo(tmpExtraFile)

	return tmpExtraFile.Name()
}
