package controllers

import (
	"github.com/easysoft/qucheng-operator/controllers/globaldb"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var controllerAddFuncs []func(manager.Manager) error

func init() {
	controllerAddFuncs = append(controllerAddFuncs, globaldb.Add)
}

func SetupWithManager(m manager.Manager) error {
	for _, f := range controllerAddFuncs {
		if err := f(m); err != nil {
			if kindMatchErr, ok := err.(*meta.NoKindMatchError); ok {
				logrus.Warnf("CRD %v is not installed, its controller will perform noops!", kindMatchErr.GroupKind)
				continue
			}
			return err
		}
	}
	return nil
}
