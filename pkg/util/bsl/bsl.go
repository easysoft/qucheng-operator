// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package bsl

import (
	"context"
	"errors"

	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	veleroclientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetBsl(ctx context.Context, name, namespace string, veleroClient veleroclientset.Interface) (*velerov1.BackupStorageLocation, error) {
	if name != "" {
		bsl, err := veleroClient.VeleroV1().BackupStorageLocations(namespace).Get(ctx, name, metav1.GetOptions{})
		return bsl, err
	}

	bslList, err := veleroClient.VeleroV1().BackupStorageLocations("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, bsl := range bslList.Items {
		if bsl.Spec.Default {
			return &bsl, nil
		}
	}

	return nil, errors.New("can't find a default backupStorageLocation")
}
