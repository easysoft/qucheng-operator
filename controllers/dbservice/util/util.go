// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package util

import (
	"github.com/easysoft/qucheng-operator/pkg/util"
)

func VaildGlobalDatabase(l map[string]string) bool {
	if util.CheckLabel(l, "easycorp.io/global_database") {
		return util.GetLabelValue(l, "easycorp.io/global_database") == "true"
	}
	return false
}
