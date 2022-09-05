// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package manage

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

func (m *postgresqlManage) Dump(meta *DbMeta) (*os.File, error) {
	var err error
	commandArgs := m.buildConnectArgs()
	commandArgs = append(commandArgs, "-d", meta.Name, "-Fc")

	output, err := ioutil.TempFile(backupRoot, "pg_dump.*.sql")
	if err != nil {
		return nil, err
	}

	stderr, err := ioutil.TempFile(backupRoot, "pg_dump.*.err")
	if err != nil {
		return nil, err
	}

	defer func() {
		stderr.Close()
		os.Remove(stderr.Name())
	}()

	cmd := exec.Command("pg_dump", commandArgs...)
	cmd.Env = []string{fmt.Sprintf("PGPASSWORD=%s", m.ServerInfo().AdminPassword())}
	cmd.Stdout = output
	cmd.Stderr = stderr
	err = cmd.Run()

	if err != nil {
		stderr.Seek(0, 0)
		fileStat, _ := stderr.Stat()
		errMessage := make([]byte, fileStat.Size())
		stderr.Read(errMessage)
		return nil, errors.Wrap(err, string(errMessage))
	}
	return output, nil
}

func (m *postgresqlManage) Restore(meta *DbMeta, input io.Reader) error {
	var err error
	commandArgs := m.buildConnectArgs()
	commandArgs = append(commandArgs, "-c", "-d", meta.Name)

	stderr, _ := ioutil.TempFile(backupRoot, "")
	defer func() {
		stderr.Close()
		os.Remove(stderr.Name())
	}()

	cmd := exec.Command("pg_restore", commandArgs...)
	cmd.Env = []string{fmt.Sprintf("PGPASSWORD=%s", m.ServerInfo().AdminPassword())}
	cmd.Stdin = input
	cmd.Stderr = stderr
	err = cmd.Run()

	if err != nil {
		stderr.Seek(0, 0)
		fileStat, _ := stderr.Stat()
		errMessage := make([]byte, fileStat.Size())
		stderr.Read(errMessage)
		return errors.Wrap(err, string(errMessage))
	}
	return nil
}

func (m *postgresqlManage) buildConnectArgs() []string {
	var args = make([]string, 0)
	args = append(args, "-h", m.ServerInfo().Host())
	args = append(args, "-p", fmt.Sprintf("%d", m.ServerInfo().Port()))
	args = append(args, "-U", m.ServerInfo().AdminUser())
	return args
}
