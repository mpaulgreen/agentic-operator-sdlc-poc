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

// PostgresClusterSpec defines the desired state of PostgresCluster.
type PostgresClusterSpec struct {
	// Replicas is the number of PostgreSQL instances to run.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=5
	// +kubebuilder:default=3
	Replicas int32 `json:"replicas,omitempty"`

	// Version is the PostgreSQL major version to deploy.
	// +kubebuilder:validation:Enum="14";"15";"16"
	// +kubebuilder:default="16"
	Version string `json:"version,omitempty"`

	// Storage defines the persistent storage configuration.
	Storage StorageSpec `json:"storage"`

	// Resources defines the CPU and memory resource requirements.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Backup defines the optional backup configuration.
	// +optional
	Backup *BackupSpec `json:"backup,omitempty"`
}

// StorageSpec defines storage configuration for PostgreSQL data.
type StorageSpec struct {
	// Size is the storage volume size (e.g., "10Gi").
	// +kubebuilder:validation:Pattern=`^[0-9]+[KMGT]i$`
	Size string `json:"size"`

	// StorageClassName is the name of the StorageClass to use.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
}

// BackupSpec defines backup configuration for PostgreSQL.
type BackupSpec struct {
	// Enabled indicates whether automated backups are active.
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Schedule is the cron expression for backup frequency.
	Schedule string `json:"schedule"`

	// RetentionDays is the number of days to retain backups.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=30
	// +kubebuilder:default=7
	RetentionDays int32 `json:"retentionDays,omitempty"`
}

// PostgresClusterStatus defines the observed state of PostgresCluster.
type PostgresClusterStatus struct {
	// Phase represents the current lifecycle phase of the PostgresCluster.
	// +kubebuilder:validation:Enum=Pending;Initializing;Running;Failed;Degraded
	Phase string `json:"phase,omitempty"`

	// ReadyReplicas is the number of PostgreSQL instances that are ready.
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// CurrentVersion is the currently running PostgreSQL version.
	CurrentVersion string `json:"currentVersion,omitempty"`

	// Endpoint is the connection endpoint for the PostgreSQL cluster.
	Endpoint string `json:"endpoint,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.status.currentVersion`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PostgresCluster is the Schema for the postgresclusters API.
type PostgresCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PostgresClusterSpec   `json:"spec,omitempty"`
	Status PostgresClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PostgresClusterList contains a list of PostgresCluster.
type PostgresClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PostgresCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PostgresCluster{}, &PostgresClusterList{})
}
