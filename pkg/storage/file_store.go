package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	_defaultBackupRoot = "/data/backup"
)

type fileStorage struct {
	AbsPath string
}

func NewFileStorage() Storage {
	return &fileStorage{}
}

func (f *fileStorage) PutBackup(info BackupInfo) error {
	absDir := f.genBackupPath(info.Name, info.Namespace, info.BackupTime)
	err := os.MkdirAll(filepath.Join(_defaultBackupRoot, absDir), 0755)
	if err != nil {
		return err
	}

	path := info.FileFd.Name()
	_, file := filepath.Split(path)
	absPath := filepath.Join(absDir, file)
	f.AbsPath = absPath

	destFile := filepath.Join(_defaultBackupRoot, absPath)
	destF, err := os.Create(destFile)
	if err != nil {
		return err
	}

	input := info.FileFd
	input.Seek(0, 0)
	_, err = io.Copy(destF, input)
	return err
}

func (f *fileStorage) PullBackup(path string) (string, error) {
	fullPath := filepath.Join(_defaultBackupRoot, path)
	return fullPath, nil
}

func (f *fileStorage) GetAbsPath() string {
	return f.AbsPath
}

func (f *fileStorage) genBackupPath(name, namespace string, backupTime time.Time) string {
	return fmt.Sprintf("%s/%s/%s/", backupTime.Format("200601"), namespace, name)
}
