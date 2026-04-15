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

// ChoCacheSpec defines the desired state of ChoCache.
type ChoCacheSpec struct {
	// application is the parent application name.
	Application string `json:"application"`

	// domain is the domain this cache belongs to.
	Domain string `json:"domain"`

	// size references a sizing template from ChoCluster.
	// +optional
	Size string `json:"size,omitempty"`

	// resources defines explicit CPU/memory (overrides size template).
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// ha enables high availability with multiple replicas.
	// When true, Dragonfly runs as a StatefulSet with replication.
	// +optional
	HA bool `json:"ha,omitempty"`

	// replicas is the number of cache instances. Defaults to 1, or 2 if ha=true.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// persistence configures data persistence for the cache.
	// +optional
	Persistence *CachePersistenceSpec `json:"persistence,omitempty"`
}

// CachePersistenceSpec configures persistence for the cache.
type CachePersistenceSpec struct {
	// enabled controls whether data is persisted to disk.
	Enabled bool `json:"enabled"`

	// size is the storage size for the persistent volume (e.g. "1Gi").
	// +kubebuilder:default="1Gi"
	// +optional
	Size string `json:"size,omitempty"`

	// storageClass is the StorageClass to use for the PVC.
	// +optional
	StorageClass string `json:"storageClass,omitempty"`
}

// ChoCacheStatus defines the observed state of ChoCache.
type ChoCacheStatus struct {
	// conditions represent the current state of the ChoCache resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ready indicates whether the cache is fully available.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// credentialsSecretRef is the name of the Secret containing connection credentials.
	// +optional
	CredentialsSecretRef string `json:"credentialsSecretRef,omitempty"`

	// compiledWithRevision is the controller revision that compiled this resource.
	// +optional
	CompiledWithRevision string `json:"compiledWithRevision,omitempty"`

	// lifecycle is the archive lifecycle state (Active, Archived, Deletable).
	// +optional
	Lifecycle string `json:"lifecycle,omitempty"`

	// archivedAt is the time the resource was archived.
	// +optional
	ArchivedAt *metav1.Time `json:"archivedAt,omitempty"`

	// deletableAfter is the earliest time the resource may be permanently deleted.
	// +optional
	DeletableAfter *metav1.Time `json:"deletableAfter,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.spec.size`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChoCache is the Schema for the chocaches API.
type ChoCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChoCacheSpec   `json:"spec,omitempty"`
	Status ChoCacheStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChoCacheList contains a list of ChoCache.
type ChoCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChoCache `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChoCache{}, &ChoCacheList{})
}
