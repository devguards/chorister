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

// ChoQueueSpec defines the desired state of ChoQueue.
type ChoQueueSpec struct {
	// application is the parent application name.
	Application string `json:"application"`

	// domain is the domain this queue belongs to.
	Domain string `json:"domain"`

	// type is the queue type.
	// +kubebuilder:validation:Enum=nats;streaming
	// +kubebuilder:default=nats
	Type string `json:"type"`

	// size references a sizing template from ChoCluster.
	// +optional
	Size string `json:"size,omitempty"`

	// resources defines explicit CPU/memory/storage (overrides size template).
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ChoQueueStatus defines the observed state of ChoQueue.
type ChoQueueStatus struct {
	// conditions represent the current state of the ChoQueue resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ready indicates whether the queue is fully available.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// credentialsSecretRef is the name of the Secret containing connection credentials.
	// +optional
	CredentialsSecretRef string `json:"credentialsSecretRef,omitempty"`

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
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.spec.size`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChoQueue is the Schema for the choqueues API.
type ChoQueue struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChoQueueSpec   `json:"spec,omitempty"`
	Status ChoQueueStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChoQueueList contains a list of ChoQueue.
type ChoQueueList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChoQueue `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChoQueue{}, &ChoQueueList{})
}
