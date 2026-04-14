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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ChoStorageSpec defines the desired state of ChoStorage.
type ChoStorageSpec struct {
	// application is the parent application name.
	Application string `json:"application"`

	// domain is the domain this storage belongs to.
	Domain string `json:"domain"`

	// variant is the storage type.
	// +kubebuilder:validation:Enum=object;block;file
	Variant string `json:"variant"`

	// size is the storage capacity (e.g. "10Gi").
	// +optional
	Size *resource.Quantity `json:"size,omitempty"`

	// accessMode is the PVC access mode (for block/file variants).
	// +kubebuilder:validation:Enum=ReadWriteOnce;ReadWriteMany;ReadOnlyMany
	// +optional
	AccessMode string `json:"accessMode,omitempty"`

	// storageClass overrides the default StorageClass.
	// +optional
	StorageClass string `json:"storageClass,omitempty"`

	// objectBackend specifies the object storage backend (for object variant).
	// +kubebuilder:validation:Enum=s3;gcs;azure
	// +optional
	ObjectBackend string `json:"objectBackend,omitempty"`
}

// ChoStorageStatus defines the observed state of ChoStorage.
type ChoStorageStatus struct {
	// conditions represent the current state of the ChoStorage resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ready indicates whether the storage is provisioned and available.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// lifecycle is the stateful resource lifecycle state.
	// +kubebuilder:validation:Enum=Active;Archived;Deletable
	// +optional
	Lifecycle string `json:"lifecycle,omitempty"`

	// archivedAt is when the resource was archived.
	// +optional
	ArchivedAt *metav1.Time `json:"archivedAt,omitempty"`

	// deletableAfter is when the resource becomes eligible for explicit deletion.
	// +optional
	DeletableAfter *metav1.Time `json:"deletableAfter,omitempty"`

	// compiledWithRevision is the controller revision that compiled this resource.
	// +optional
	CompiledWithRevision string `json:"compiledWithRevision,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Variant",type=string,JSONPath=`.spec.variant`
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.spec.size`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChoStorage is the Schema for the chostorages API.
type ChoStorage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChoStorageSpec   `json:"spec,omitempty"`
	Status ChoStorageStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChoStorageList contains a list of ChoStorage.
type ChoStorageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChoStorage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChoStorage{}, &ChoStorageList{})
}
