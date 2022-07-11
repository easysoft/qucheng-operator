// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
)

func main() {
	v := velerov1api.PodVolumeBackupStatus{}
	fmt.Println(v)
}