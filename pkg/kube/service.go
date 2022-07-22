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
