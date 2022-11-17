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
