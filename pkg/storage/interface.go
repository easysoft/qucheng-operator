package storage

import (
	"os"
	"time"
)

type BackupInfo struct {
	// Backup name, the app name is recommend
	Name string

	// Backup has a namespace
	Namespace string

	BackupTime time.Time
	File       string
	FileFd     *os.File
}

type Storage interface {
	PutBackup(info BackupInfo) error
	PullBackup(path string) (string, error)
	GetAbsPath() string
}
