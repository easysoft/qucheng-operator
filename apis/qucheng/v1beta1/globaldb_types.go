// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GlobalDBSpec defines the desired state of GlobalDB
type GlobalDBSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Name   string `json:"name"`
	Source string `json:"source"`
	Type   DbType `json:"type"`
}

// GlobalDBStatus defines the observed state of GlobalDB
type GlobalDBStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Ready   bool  `json:"ready"`
	ChildDB int64 `json:"childDB"`
}

//+genclient
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// GlobalDB is the Schema for the globaldbs API
type GlobalDB struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GlobalDBSpec   `json:"spec,omitempty"`
	Status GlobalDBStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GlobalDBList contains a list of GlobalDB
type GlobalDBList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GlobalDB `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GlobalDB{}, &GlobalDBList{})
}
