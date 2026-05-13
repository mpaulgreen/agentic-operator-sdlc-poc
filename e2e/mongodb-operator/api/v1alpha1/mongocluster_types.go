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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MongoClusterSpec defines the desired state of MongoCluster.
type MongoClusterSpec struct {
	// Replicas is the number of MongoDB replica set members (should be odd for elections).
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=7
	// +kubebuilder:default=3
	Replicas int32 `json:"replicas,omitempty"`

	// Version is the MongoDB major version to deploy.
	// +kubebuilder:validation:Enum="7.0";"8.0"
	// +kubebuilder:default="7.0"
	Version string `json:"version,omitempty"`

	// Storage defines the persistent storage configuration.
	Storage StorageSpec `json:"storage"`

	// Resources defines the CPU and memory resource requirements.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Auth defines the optional authentication configuration.
	// +optional
	Auth *AuthSpec `json:"auth,omitempty"`

	// Backup defines the optional backup configuration.
	// +optional
	Backup *BackupSpec `json:"backup,omitempty"`

	// Arbiter defines the optional arbiter node configuration.
	// +optional
	Arbiter *ArbiterSpec `json:"arbiter,omitempty"`
}

// StorageSpec defines storage configuration for MongoDB data.
type StorageSpec struct {
	// Size is the storage volume size (e.g., "10Gi").
	// +kubebuilder:validation:Pattern=`^[0-9]+[KMGT]i$`
	Size string `json:"size"`

	// StorageClassName is the name of the StorageClass to use.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

// AuthSpec defines authentication configuration for MongoDB.
type AuthSpec struct {
	// AdminPassword is the MongoDB admin user password. Mutually exclusive with ExistingSecret.
	// +optional
	AdminPassword string `json:"adminPassword,omitempty"`

	// ExistingSecret is the name of an existing Secret containing the admin password.
	// Mutually exclusive with AdminPassword.
	// +optional
	ExistingSecret string `json:"existingSecret,omitempty"`

	// KeyFile is the name of an existing Secret containing the replica set keyFile.
	// If empty, the operator generates a random keyFile.
	// +optional
	KeyFile string `json:"keyFile,omitempty"`
}

// BackupSpec defines backup configuration for MongoDB.
type BackupSpec struct {
	// Enabled indicates whether backup is active.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// RetentionDays is the number of days to retain backups.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=30
	// +kubebuilder:default=7
	RetentionDays int32 `json:"retentionDays,omitempty"`
}

// ArbiterSpec defines arbiter node configuration for MongoDB replica set elections.
type ArbiterSpec struct {
	// Enabled indicates whether the arbiter node is active.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Resources defines the CPU and memory resource requirements for the arbiter.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// MongoClusterStatus defines the observed state of MongoCluster.
type MongoClusterStatus struct {
	// Phase represents the current lifecycle phase of the MongoCluster.
	// +kubebuilder:validation:Enum=Pending;Initializing;Running;Failed;Degraded
	Phase string `json:"phase,omitempty"`

	// ReadyReplicas is the number of MongoDB instances that are ready.
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// CurrentVersion is the currently running MongoDB version.
	CurrentVersion string `json:"currentVersion,omitempty"`

	// PrimaryEndpoint is the client service connection endpoint.
	PrimaryEndpoint string `json:"primaryEndpoint,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// LastBackupTime is when the last backup Job completed successfully.
	// +optional
	LastBackupTime *metav1.Time `json:"lastBackupTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.currentVersion`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// MongoCluster is the Schema for the mongoclusters API.
type MongoCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MongoClusterSpec   `json:"spec,omitempty"`
	Status MongoClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MongoClusterList contains a list of MongoCluster.
type MongoClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MongoCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MongoCluster{}, &MongoClusterList{})
}
