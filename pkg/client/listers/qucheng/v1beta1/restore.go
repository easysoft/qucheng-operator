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
// Code generated by lister-gen. DO NOT EDIT.

package v1beta1

import (
	v1beta1 "github.com/easysoft/qucheng-operator/apis/qucheng/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// RestoreLister helps list Restores.
// All objects returned here must be treated as read-only.
type RestoreLister interface {
	// List lists all Restores in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1beta1.Restore, err error)
	// Restores returns an object that can list and get Restores.
	Restores(namespace string) RestoreNamespaceLister
	RestoreListerExpansion
}

// restoreLister implements the RestoreLister interface.
type restoreLister struct {
	indexer cache.Indexer
}

// NewRestoreLister returns a new RestoreLister.
func NewRestoreLister(indexer cache.Indexer) RestoreLister {
	return &restoreLister{indexer: indexer}
}

// List lists all Restores in the indexer.
func (s *restoreLister) List(selector labels.Selector) (ret []*v1beta1.Restore, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1beta1.Restore))
	})
	return ret, err
}

// Restores returns an object that can list and get Restores.
func (s *restoreLister) Restores(namespace string) RestoreNamespaceLister {
	return restoreNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// RestoreNamespaceLister helps list and get Restores.
// All objects returned here must be treated as read-only.
type RestoreNamespaceLister interface {
	// List lists all Restores in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1beta1.Restore, err error)
	// Get retrieves the Restore from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1beta1.Restore, error)
	RestoreNamespaceListerExpansion
}

// restoreNamespaceLister implements the RestoreNamespaceLister
// interface.
type restoreNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all Restores in the indexer for a given namespace.
func (s restoreNamespaceLister) List(selector labels.Selector) (ret []*v1beta1.Restore, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1beta1.Restore))
	})
	return ret, err
}

// Get retrieves the Restore from the indexer for a given namespace and name.
func (s restoreNamespaceLister) Get(name string) (*v1beta1.Restore, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1beta1.Resource("restore"), name)
	}
	return obj.(*v1beta1.Restore), nil
}
