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

// ChoNetworkSpec defines the desired state of ChoNetwork.
type ChoNetworkSpec struct {
	// application is the parent application name.
	Application string `json:"application"`

	// domain is the domain this network resource belongs to.
	Domain string `json:"domain"`

	// ingress defines inbound traffic rules.
	// +optional
	Ingress *NetworkIngressSpec `json:"ingress,omitempty"`

	// egress defines outbound traffic rules.
	// +optional
	Egress *NetworkEgressSpec `json:"egress,omitempty"`
}

// NetworkIngressSpec defines inbound traffic configuration.
type NetworkIngressSpec struct {
	// from is the traffic source ("internet" or a service reference).
	From string `json:"from"`

	// port is the listening port.
	Port int `json:"port"`

	// auth defines authentication requirements.
	// +optional
	Auth *NetworkAuthSpec `json:"auth,omitempty"`

	// routes defines per-path routing rules.
	// +optional
	Routes []NetworkRouteSpec `json:"routes,omitempty"`
}

// NetworkAuthSpec defines authentication for ingress.
type NetworkAuthSpec struct {
	// jwt defines JWT-based authentication.
	// +optional
	JWT *JWTAuthSpec `json:"jwt,omitempty"`
}

// JWTAuthSpec defines JWT verification parameters.
type JWTAuthSpec struct {
	JWKSUri  string   `json:"jwksUri"`
	Issuer   string   `json:"issuer"`
	Audience []string `json:"audience,omitempty"`
}

// NetworkRouteSpec defines a per-path route rule.
type NetworkRouteSpec struct {
	// path is the URL path pattern.
	Path string `json:"path"`

	// auth overrides per-route auth ("none" for anonymous).
	// +optional
	Auth string `json:"auth,omitempty"`

	// claims defines required JWT claims for this route.
	// +optional
	Claims map[string]string `json:"claims,omitempty"`

	// hmac enables HMAC signature verification (e.g. for webhooks).
	// +optional
	HMAC bool `json:"hmac,omitempty"`
}

// NetworkEgressSpec defines outbound traffic rules.
type NetworkEgressSpec struct {
	// allowlist selects from the application's egress allowlist.
	// +optional
	Allowlist []string `json:"allowlist,omitempty"`
}

// ChoNetworkStatus defines the observed state of ChoNetwork.
type ChoNetworkStatus struct {
	// conditions represent the current state of the ChoNetwork resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ready indicates whether the network configuration is applied.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// compiledWithRevision is the controller revision that compiled this resource.
	// +optional
	CompiledWithRevision string `json:"compiledWithRevision,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChoNetwork is the Schema for the chonetworks API.
type ChoNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChoNetworkSpec   `json:"spec,omitempty"`
	Status ChoNetworkStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChoNetworkList contains a list of ChoNetwork.
type ChoNetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChoNetwork `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChoNetwork{}, &ChoNetworkList{})
}
