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

// ChoDatabaseSpec defines the desired state of ChoDatabase.
type ChoDatabaseSpec struct {
	// application is the parent application name.
	Application string `json:"application"`

	// domain is the domain this database belongs to.
	Domain string `json:"domain"`

	// engine is the database engine.
	// +kubebuilder:validation:Enum=postgres
	// +kubebuilder:default=postgres
	Engine string `json:"engine"`

	// size references a sizing template from ChoCluster.
	// +optional
	Size string `json:"size,omitempty"`

	// ha enables high availability (Patroni auto-failover).
	// +optional
	HA bool `json:"ha,omitempty"`

	// resources defines explicit CPU/memory/storage (overrides size template).
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ChoDatabaseStatus defines the observed state of ChoDatabase.
type ChoDatabaseStatus struct {
	// conditions represent the current state of the ChoDatabase resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ready indicates whether the database is fully available.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// credentialsSecretRef is the name of the Secret containing connection credentials.
	// +optional
	CredentialsSecretRef string `json:"credentialsSecretRef,omitempty"`

	// instances is the number of running database instances.
	// +optional
	Instances int32 `json:"instances,omitempty"`

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

	// finalSnapshotRef is the reference to the final backup snapshot.
	// +optional
	FinalSnapshotRef string `json:"finalSnapshotRef,omitempty"`

	// compiledWithRevision is the controller revision that compiled this resource.
	// +optional
	CompiledWithRevision string `json:"compiledWithRevision,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Engine",type=string,JSONPath=`.spec.engine`
// +kubebuilder:printcolumn:name="Size",type=string,JSONPath=`.spec.size`
// +kubebuilder:printcolumn:name="HA",type=boolean,JSONPath=`.spec.ha`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Lifecycle",type=string,JSONPath=`.status.lifecycle`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChoDatabase is the Schema for the chodatabases API.
type ChoDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChoDatabaseSpec   `json:"spec,omitempty"`
	Status ChoDatabaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChoDatabaseList contains a list of ChoDatabase.
type ChoDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChoDatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChoDatabase{}, &ChoDatabaseList{})
}
