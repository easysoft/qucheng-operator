package manage

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

func (m *mysqlManage) Dump(meta *DbMeta) (*os.File, error) {
	var err error
	extraFile := m.buildExtraFile()
	commandArgs := []string{"--defaults-extra-file=" + extraFile, "--databases", meta.Name}

	output, err := ioutil.TempFile(backupRoot, "mysqldump.*****.sql")
	if err != nil {
		return nil, err
	}

	stderr, err := ioutil.TempFile(backupRoot, "mysqldump.*****.err")
	if err != nil {
		return nil, err
	}

	defer func() {
		os.Remove(extraFile)
		stderr.Close()
		os.Remove(stderr.Name())
	}()

	cmd := exec.Command("mysqldump", commandArgs...)
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

func (m *mysqlManage) Restore(meta *DbMeta, input io.Reader) error {
	var err error
	extraFile := m.buildExtraFile()
	commandArgs := []string{"--defaults-extra-file=" + extraFile, "--database", meta.Name}

	stderr, _ := ioutil.TempFile(backupRoot, "")
	defer func() {
		os.Remove(extraFile)
		stderr.Close()
		os.Remove(stderr.Name())
	}()

	cmd := exec.Command("mysql", commandArgs...)
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

func (m *mysqlManage) buildExtraFile() string {
	buf := bytes.NewBufferString("[client]\n")
	buf.WriteString(fmt.Sprintf("host = %s\n", m.meta.Host))
	buf.WriteString(fmt.Sprintf("port = %d\n", m.meta.Port))
	buf.WriteString(fmt.Sprintf("user = %s\n", m.meta.AdminUser))
	buf.WriteString(fmt.Sprintf("password = %s\n", m.meta.AdminPassword))

	tmpExtraFile, err := ioutil.TempFile("/tmp", "")
	if err != nil {
		fmt.Println(err)
	}

	buf.WriteTo(tmpExtraFile)

	return tmpExtraFile.Name()
}
