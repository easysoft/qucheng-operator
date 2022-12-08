// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package qucheng

import (
	"context"

	"github.com/easysoft/qucheng-operator/pkg/storage"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetDbObjectStorage(ctx context.Context, c client.Client, storageName string, namespace string) (storage.Storage, error) {
	var bsl velerov1.BackupStorageLocation
	err := c.Get(ctx, client.ObjectKey{Name: storageName, Namespace: namespace}, &bsl)
	if err != nil {
		return nil, err
	}
	config := bsl.Spec.Config
	store, err := storage.NewObjectStorage(ctx,
		config["s3Url"],
		config["accessKey"],
		config["secretKey"],
		config["region"],
		config["bucketDb"],
	)
	return store, err
}
