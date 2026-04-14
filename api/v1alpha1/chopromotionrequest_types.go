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

// ChoPromotionRequestSpec defines the desired state of ChoPromotionRequest.
type ChoPromotionRequestSpec struct {
	// application is the parent application name.
	Application string `json:"application"`

	// domain is the domain to promote.
	Domain string `json:"domain"`

	// sandbox is the source sandbox name.
	Sandbox string `json:"sandbox"`

	// requestedBy is the identity of the requester.
	RequestedBy string `json:"requestedBy"`

	// diff is a human-readable summary of the changes.
	// +optional
	Diff string `json:"diff,omitempty"`

	// externalRef is an optional external ticket reference.
	// +optional
	ExternalRef string `json:"externalRef,omitempty"`

	// compiledWithRevision is the controller revision that compiled the sandbox contents.
	// +optional
	CompiledWithRevision string `json:"compiledWithRevision,omitempty"`
}

// PromotionApproval represents a single approval for a promotion request.
type PromotionApproval struct {
	// approver is the identity of the approver.
	Approver string `json:"approver"`

	// role is the role of the approver.
	Role string `json:"role"`

	// approvedAt is when the approval was granted.
	ApprovedAt metav1.Time `json:"approvedAt"`
}

// ChoPromotionRequestStatus defines the observed state of ChoPromotionRequest.
type ChoPromotionRequestStatus struct {
	// conditions represent the current state of the promotion request.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// phase is the promotion lifecycle state.
	// +kubebuilder:validation:Enum=Pending;Approved;Executing;Completed;Rejected;Failed
	// +optional
	Phase string `json:"phase,omitempty"`

	// approvals lists the approvals received.
	// +optional
	Approvals []PromotionApproval `json:"approvals,omitempty"`

	// compiledWithRevision is the controller revision used during execution.
	// +optional
	CompiledWithRevision string `json:"compiledWithRevision,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Domain",type=string,JSONPath=`.spec.domain`
// +kubebuilder:printcolumn:name="Sandbox",type=string,JSONPath=`.spec.sandbox`
// +kubebuilder:printcolumn:name="RequestedBy",type=string,JSONPath=`.spec.requestedBy`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChoPromotionRequest is the Schema for the chopromotionrequests API.
type ChoPromotionRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChoPromotionRequestSpec   `json:"spec,omitempty"`
	Status ChoPromotionRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChoPromotionRequestList contains a list of ChoPromotionRequest.
type ChoPromotionRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChoPromotionRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChoPromotionRequest{}, &ChoPromotionRequestList{})
}
