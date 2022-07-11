// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

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
