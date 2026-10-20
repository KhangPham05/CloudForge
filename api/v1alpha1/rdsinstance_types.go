/*
Copyright 2026.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RDSInstanceSpec defines the desired state of RDSInstance
type RDSInstanceSpec struct {
	// instanceClass is the AWS RDS instance size (e.g. "db.t3.micro").
	// +kubebuilder:validation:Required
	InstanceClass string `json:"instanceClass"`

	// engine is the database engine to provision.
	// +kubebuilder:validation:Enum=postgres;mysql
	Engine string `json:"engine"`

	// allocatedStorage is the storage size in GiB.
	// +kubebuilder:validation:Minimum=20
	AllocatedStorage int32 `json:"allocatedStorage"`

	// vpcRef is the name of the VPC CR (in the same namespace) this
	// database should be provisioned inside. The RDSInstance
	// controller waits for that VPC to be Ready before provisioning.
	// +kubebuilder:validation:Required
	VpcRef string `json:"vpcRef"`
}

// RDSInstanceStatus defines the observed state of RDSInstance.
type RDSInstanceStatus struct {
	// endpoint is the database connection endpoint, set once available.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// port is the database connection port, set once available.
	// +optional
	Port int32 `json:"port,omitempty"`

	// phase is a short, human-readable summary of where this database
	// is in its lifecycle.
	// +kubebuilder:validation:Enum=Provisioning;Ready
	// +kubebuilder:default=Provisioning
	Phase string `json:"phase,omitempty"`

	// secretRef is the name of the generated Kubernetes Secret holding
	// this database's credentials, set once provisioning completes.
	// +optional
	SecretRef string `json:"secretRef,omitempty"`

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the RDSInstance resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Engine",type=string,JSONPath=`.spec.engine`
// +kubebuilder:printcolumn:name="VpcRef",type=string,JSONPath=`.spec.vpcRef`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// RDSInstance is the Schema for the rdsinstances API
type RDSInstance struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of RDSInstance
	// +required
	Spec RDSInstanceSpec `json:"spec"`

	// status defines the observed state of RDSInstance
	// +optional
	Status RDSInstanceStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// RDSInstanceList contains a list of RDSInstance
type RDSInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []RDSInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &RDSInstance{}, &RDSInstanceList{})
		return nil
	})
}
