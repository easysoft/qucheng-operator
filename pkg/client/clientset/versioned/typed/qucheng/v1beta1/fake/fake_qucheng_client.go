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
// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1beta1 "gitlab.zcorp.cc/pangu/cne-operator/pkg/client/clientset/versioned/typed/qucheng/v1beta1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeQuchengV1beta1 struct {
	*testing.Fake
}

func (c *FakeQuchengV1beta1) Backups(namespace string) v1beta1.BackupInterface {
	return &FakeBackups{c, namespace}
}

func (c *FakeQuchengV1beta1) Dbs(namespace string) v1beta1.DbInterface {
	return &FakeDbs{c, namespace}
}

func (c *FakeQuchengV1beta1) DbServices(namespace string) v1beta1.DbServiceInterface {
	return &FakeDbServices{c, namespace}
}

func (c *FakeQuchengV1beta1) Restores(namespace string) v1beta1.RestoreInterface {
	return &FakeRestores{c, namespace}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeQuchengV1beta1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}
