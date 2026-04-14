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

	// size references a sizing template from ChoCluster (small/medium/large).
	// +kubebuilder:validation:Enum=small;medium;large
	// +kubebuilder:default=small
	Size string `json:"size"`

	// resources defines explicit CPU/memory (overrides size template).
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
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
