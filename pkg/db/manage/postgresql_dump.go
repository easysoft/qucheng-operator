// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package manage

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

func (m *postgresqlManage) Dump(meta *DbMeta) (*os.File, error) {
	var err error

	var config PostgresqlConfig
	bs, _ := json.Marshal(meta.Config)
	_ = json.Unmarshal(bs, &config)

	commandArgs := m.buildConnectArgs()
	commandArgs = append(commandArgs, "-d", meta.Name)

	dumpFormat := _defaultDumpFormat
	if config.DumpFormat != "" {
		dumpFormat = config.DumpFormat
	}
	commandArgs = append(commandArgs, "-c", "--if-exists", "-F", dumpFormat)

	if config.DumpCompressLevel != "" {
		commandArgs = append(commandArgs, "-Z", config.DumpCompressLevel)
	}

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

func (m *postgresqlManage) Restore(meta *DbMeta, input io.Reader, path string) error {
	var err error

	var config PostgresqlConfig
	bs, _ := json.Marshal(meta.Config)
	_ = json.Unmarshal(bs, &config)

	commandArgs := m.buildConnectArgs()
	commandArgs = append(commandArgs, "-d", meta.Name)

	stderr, _ := ioutil.TempFile(backupRoot, meta.Name+".*.err")
	stdout, _ := ioutil.TempFile(backupRoot, meta.Name+".*.out")
	defer func() {
		stderr.Close()
		stdout.Close()
		_ = os.Remove(stderr.Name())
		_ = os.Remove(stdout.Name())
	}()

	dumpFormat := _defaultDumpFormat
	filename := filepath.Base(path)
	parts := strings.SplitN(filename, ".", 4)
	suffix := parts[len(parts)-1]
	if strings.HasPrefix(suffix, dumpFormatPlain+".") {
		dumpFormat = dumpFormatPlain
	}

	var cmd *exec.Cmd
	if dumpFormat == dumpFormatPlain {
		cmd = exec.Command("/bin/psql", commandArgs...)
	} else {
		cmd = exec.Command("pg_restore", commandArgs...)
	}
	cmd.Env = []string{fmt.Sprintf("PGPASSWORD=%s", m.ServerInfo().AdminPassword())}
	cmd.Stderr = stderr

	if filepath.Ext(suffix) == "gz" {
		gunzipCmd := exec.Command("gunzip", "-c", "-")
		gunzipCmd.Stdin = input

		pipeOut, e := gunzipCmd.StdoutPipe()
		if e != nil {
			return e
		}

		if err = gunzipCmd.Start(); err != nil {
			return err
		}
		cmd.Stdin = pipeOut
		if err = cmd.Run(); err != nil {
			return err
		}

		if err = gunzipCmd.Wait(); err != nil {
			return err
		}
	} else {
		cmd.Stdin = input
		err = cmd.Run()
	}

	if err != nil {
		stderr.Seek(0, 0)
		fileStat, _ := stderr.Stat()
		errMessage := make([]byte, fileStat.Size())
		stderr.Read(errMessage)
		if config.RestoreIgnoreErrorRegex != "" {
			re, err := regexp.Compile(config.RestoreIgnoreErrorRegex)
			if err != nil {
				return errors.Wrap(err, "restore ignore err regex is invalid")
			}
			errLines := make([]string, 0)
			for _, msg := range strings.Split(string(errMessage), "\n") {
				if !re.MatchString(msg) {
					errLines = append(errLines, msg)
				}
			}
			if len(errLines) > 0 {
				return errors.Wrap(err, strings.Join(errLines, "\n"))
			}
		} else {
			return errors.Wrap(err, string(errMessage))
		}
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
