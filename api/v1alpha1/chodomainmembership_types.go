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

// ChoDomainMembershipSpec defines the desired state of ChoDomainMembership.
type ChoDomainMembershipSpec struct {
	// application is the parent application name.
	Application string `json:"application"`

	// domain is the domain to grant access to.
	Domain string `json:"domain"`

	// identity is the user identity (e.g. email address).
	Identity string `json:"identity"`

	// role is the access level.
	// +kubebuilder:validation:Enum=org-admin;domain-admin;developer;viewer
	Role string `json:"role"`

	// source indicates how the membership was created.
	// +kubebuilder:validation:Enum=manual;oidc-group
	// +kubebuilder:default=manual
	// +optional
	Source string `json:"source,omitempty"`

	// oidcGroup is the OIDC group name (when source=oidc-group).
	// +optional
	OIDCGroup string `json:"oidcGroup,omitempty"`

	// expiresAt is when this membership expires. Required for restricted domains
	// and regulated applications.
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`
}

// ChoDomainMembershipStatus defines the observed state of ChoDomainMembership.
type ChoDomainMembershipStatus struct {
	// conditions represent the current state of the membership.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// phase is the membership lifecycle state.
	// +kubebuilder:validation:Enum=Active;Expired;Deprovisioned
	// +optional
	Phase string `json:"phase,omitempty"`

	// roleBindingRef is the name of the created RoleBinding.
	// +optional
	RoleBindingRef string `json:"roleBindingRef,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Application",type=string,JSONPath=`.spec.application`
// +kubebuilder:printcolumn:name="Domain",type=string,JSONPath=`.spec.domain`
// +kubebuilder:printcolumn:name="Identity",type=string,JSONPath=`.spec.identity`
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.spec.role`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChoDomainMembership is the Schema for the chodomainmemberships API.
type ChoDomainMembership struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChoDomainMembershipSpec   `json:"spec,omitempty"`
	Status ChoDomainMembershipStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChoDomainMembershipList contains a list of ChoDomainMembership.
type ChoDomainMembershipList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChoDomainMembership `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChoDomainMembership{}, &ChoDomainMembershipList{})
}
