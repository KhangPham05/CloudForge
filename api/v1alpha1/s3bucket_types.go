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

// S3BucketSpec defines the desired state of S3Bucket
type S3BucketSpec struct {
	// bucketName is the name of the S3 bucket to create. Must be
	// globally unique within AWS (or, for local dev, within LocalStack).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`
	BucketName string `json:"bucketName"`

	// acl is the canned access control list to apply to the bucket.
	// +kubebuilder:validation:Enum=private;public-read
	// +kubebuilder:default=private
	Acl string `json:"acl,omitempty"`

	// versioningEnabled turns on S3 object versioning for this bucket.
	// +kubebuilder:default=false
	// +optional
	VersioningEnabled *bool `json:"versioningEnabled,omitempty"`
}

// S3BucketStatus defines the observed state of S3Bucket.
type S3BucketStatus struct {
	// arn is the AWS ARN of the created bucket, set once it exists.
	// +optional
	Arn string `json:"arn,omitempty"`

	// phase is a short, human-readable summary of where this bucket
	// is in its lifecycle.
	// +kubebuilder:validation:Enum=Provisioning;Ready
	// +kubebuilder:default=Provisioning
	Phase string `json:"phase,omitempty"`

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the S3Bucket resource.
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
// +kubebuilder:printcolumn:name="Bucket",type=string,JSONPath=`.spec.bucketName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`

// S3Bucket is the Schema for the s3buckets API
type S3Bucket struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of S3Bucket
	// +required
	Spec S3BucketSpec `json:"spec"`

	// status defines the observed state of S3Bucket
	// +optional
	Status S3BucketStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// S3BucketList contains a list of S3Bucket
type S3BucketList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []S3Bucket `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &S3Bucket{}, &S3BucketList{})
		return nil
	})
}
