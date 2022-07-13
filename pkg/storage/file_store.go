// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package storage

import (
	"io"
	"os"
	"path/filepath"
)

const (
	_defaultBackupRoot = "/data/backup/databases"
)

type fileStorage struct {
	AbsPath string
}

func NewFileStorage() Storage {
	return &fileStorage{}
}

func (f *fileStorage) PutBackup(path string, fd *os.File) (int64, error) {
	destFile := filepath.Join(_defaultBackupRoot, path)
	if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
		return 0, err
	}
	destF, err := os.Create(destFile)
	if err != nil {
		return 0, err
	}
	defer destF.Close()

	input := fd
	input.Seek(0, 0)
	_, err = io.Copy(destF, input)
	if err != nil {
		return 0, err
	}

	stat, _ := destF.Stat()
	return stat.Size(), nil
}

func (f *fileStorage) PullBackup(path string) (*os.File, error) {
	fullPath := filepath.Join(_defaultBackupRoot, path)
	fd, err := os.Open(fullPath)
	return fd, err
}
