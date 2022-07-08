package main

import (
	"fmt"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
)

func main() {
	v := velerov1api.PodVolumeBackupStatus{}
	fmt.Println(v)
}
