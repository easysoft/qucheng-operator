/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
// Code generated by informer-gen. DO NOT EDIT.

package v1beta1

import (
	internalinterfaces "github.com/easysoft/qucheng-operator/pkg/client/informers/externalversions/internalinterfaces"
)

// Interface provides access to all the informers in this group version.
type Interface interface {
	// Backups returns a BackupInformer.
	Backups() BackupInformer
	// Dbs returns a DbInformer.
	Dbs() DbInformer
	// DbBackups returns a DbBackupInformer.
	DbBackups() DbBackupInformer
	// DbServices returns a DbServiceInformer.
	DbServices() DbServiceInformer
	// GlobalDBs returns a GlobalDBInformer.
	GlobalDBs() GlobalDBInformer
	// Restores returns a RestoreInformer.
	Restores() RestoreInformer
}

type version struct {
	factory          internalinterfaces.SharedInformerFactory
	namespace        string
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// New returns a new Interface.
func New(f internalinterfaces.SharedInformerFactory, namespace string, tweakListOptions internalinterfaces.TweakListOptionsFunc) Interface {
	return &version{factory: f, namespace: namespace, tweakListOptions: tweakListOptions}
}

// Backups returns a BackupInformer.
func (v *version) Backups() BackupInformer {
	return &backupInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// Dbs returns a DbInformer.
func (v *version) Dbs() DbInformer {
	return &dbInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// DbBackups returns a DbBackupInformer.
func (v *version) DbBackups() DbBackupInformer {
	return &dbBackupInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// DbServices returns a DbServiceInformer.
func (v *version) DbServices() DbServiceInformer {
	return &dbServiceInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// GlobalDBs returns a GlobalDBInformer.
func (v *version) GlobalDBs() GlobalDBInformer {
	return &globalDBInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}

// Restores returns a RestoreInformer.
func (v *version) Restores() RestoreInformer {
	return &restoreInformer{factory: v.factory, namespace: v.namespace, tweakListOptions: v.tweakListOptions}
}
