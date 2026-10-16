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

// VPCSpec defines the desired state of VPC
type VPCSpec struct {
	// vpcName is the Name tag to give the VPC in AWS - not the same
	// as the Kubernetes object's own metadata.name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	VPCName string `json:"vpcName"`

	// cidrBlock is the IPv4 CIDR block for this VPC (e.g. "10.0.0.0/16").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([0-9]{1,3}\.){3}[0-9]{1,3}/[0-9]{1,2}$`
	CidrBlock string `json:"cidrBlock"`

	// enableDnsSupport turns on DNS resolution within the VPC.
	// +kubebuilder:default=true
	// +optional
	EnableDnsSupport *bool `json:"enableDnsSupport,omitempty"`
}

// VPCStatus defines the observed state of VPC.
type VPCStatus struct {
	// vpcId is the AWS-generated ID of the VPC, set once it exists.
	// +optional
	VpcId string `json:"vpcId,omitempty"`

	// phase is a short, human-readable summary of where this VPC is
	// in its lifecycle.
	// +kubebuilder:validation:Enum=Pending;Ready
	// +kubebuilder:default=Pending
	Phase string `json:"phase,omitempty"`

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the VPC resource.
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
// +kubebuilder:printcolumn:name="VpcId",type=string,JSONPath=`.status.vpcId`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// VPC is the Schema for the vpcs API
type VPC struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of VPC
	// +required
	Spec VPCSpec `json:"spec"`

	// status defines the observed state of VPC
	// +optional
	Status VPCStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// VPCList contains a list of VPC
type VPCList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []VPC `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &VPC{}, &VPCList{})
		return nil
	})
}
