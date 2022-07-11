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

	// Foo is an example field of GlobalDB. Edit globaldb_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// GlobalDBStatus defines the observed state of GlobalDB
type GlobalDBStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
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
