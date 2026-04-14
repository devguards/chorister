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
)

// ChoSandboxSpec defines the desired state of ChoSandbox.
type ChoSandboxSpec struct {
	// application is the parent application name.
	Application string `json:"application"`

	// domain is the domain this sandbox belongs to.
	Domain string `json:"domain"`

	// name is the sandbox name (e.g. developer name or feature name).
	// +kubebuilder:validation:Pattern=`^[a-z][a-z0-9-]*$`
	Name string `json:"name"`

	// owner is the identity of the sandbox owner.
	Owner string `json:"owner"`
}

// ChoSandboxStatus defines the observed state of ChoSandbox.
type ChoSandboxStatus struct {
	// conditions represent the current state of the sandbox.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// phase is the sandbox lifecycle state.
	// +optional
	Phase string `json:"phase,omitempty"`

	// namespace is the created sandbox namespace name.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// lastApplyTime is when the last `chorister apply` was run.
	// +optional
	LastApplyTime *metav1.Time `json:"lastApplyTime,omitempty"`

	// estimatedMonthlyCost is the estimated monthly cost of sandbox resources.
	// +optional
	EstimatedMonthlyCost string `json:"estimatedMonthlyCost,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Application",type=string,JSONPath=`.spec.application`
// +kubebuilder:printcolumn:name="Domain",type=string,JSONPath=`.spec.domain`
// +kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Owner",type=string,JSONPath=`.spec.owner`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChoSandbox is the Schema for the chosandboxes API.
type ChoSandbox struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChoSandboxSpec   `json:"spec,omitempty"`
	Status ChoSandboxStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChoSandboxList contains a list of ChoSandbox.
type ChoSandboxList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChoSandbox `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChoSandbox{}, &ChoSandboxList{})
}
