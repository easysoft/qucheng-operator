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
