// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package manage

import "fmt"

type serverInfo struct {
	host string
	port int32
}

func (s *serverInfo) Host() string {
	return s.host
}

func (s *serverInfo) Port() int32 {
	return s.port
}

func (s *serverInfo) Address() string {
	return fmt.Sprintf("%s:%d", s.host, s.port)
}
