// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package kube

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func ReadServicePort(svc *v1.Service, portDefine intstr.IntOrString) int32 {
	var port int32
	if portDefine.Type == intstr.Int {
		port = portDefine.IntVal
	} else {
		for _, p := range svc.Spec.Ports {
			if p.Name == portDefine.StrVal {
				port = p.Port
				break
			}
		}
	}
	return port
}
