package volume

import v1 "k8s.io/api/core/v1"

type PvcBackup struct {
	Pod        v1.Pod
	VolumeName string
	PvcName    string
}
