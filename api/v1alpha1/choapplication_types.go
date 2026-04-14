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

// ChoApplicationSpec defines the desired state of ChoApplication.
type ChoApplicationSpec struct {
	// owners is a list of email addresses for application owners.
	// +kubebuilder:validation:MinItems=1
	Owners []string `json:"owners"`

	// policy defines the application-level policy boundary.
	Policy ApplicationPolicy `json:"policy"`

	// domains defines the bounded contexts (DDD) within this application.
	// +kubebuilder:validation:MinItems=1
	Domains []DomainSpec `json:"domains"`

	// links defines cross-application connections via Gateway API.
	// +optional
	Links []LinkSpec `json:"links,omitempty"`
}

// ApplicationPolicy defines compliance, HA, promotion, quota, and network policy.
type ApplicationPolicy struct {
	// compliance is the compliance profile for this application.
	// +kubebuilder:validation:Enum=essential;standard;regulated
	// +kubebuilder:default=essential
	Compliance string `json:"compliance"`

	// auditRetention is the duration to retain audit logs (e.g. "2y", "90d").
	// +optional
	AuditRetention string `json:"auditRetention,omitempty"`

	// ha defines the high-availability strategy.
	// +optional
	HA *HAPolicy `json:"ha,omitempty"`

	// promotion defines the promotion approval policy.
	Promotion PromotionPolicy `json:"promotion"`

	// quotas defines resource quota configuration.
	// +optional
	Quotas *QuotaPolicy `json:"quotas,omitempty"`

	// network defines application-level network policy (egress/ingress ceilings).
	// +optional
	Network *AppNetworkPolicy `json:"network,omitempty"`

	// archiveRetention is the minimum archive retention period for stateful resources.
	// Must be >= 30d. Defaults to 30d.
	// +optional
	ArchiveRetention string `json:"archiveRetention,omitempty"`

	// sandbox defines sandbox lifecycle policy.
	// +optional
	Sandbox *SandboxPolicy `json:"sandbox,omitempty"`
}

// HAPolicy defines high-availability settings.
type HAPolicy struct {
	// strategy is the HA strategy.
	// +kubebuilder:validation:Enum=single-cluster;hot-cold;active-active
	// +kubebuilder:default=single-cluster
	Strategy string `json:"strategy"`

	// clusters defines the primary and failover clusters (for multi-cluster).
	// +optional
	Clusters *HAClusters `json:"clusters,omitempty"`
}

// HAClusters names the primary and failover clusters.
type HAClusters struct {
	Primary  string `json:"primary"`
	Failover string `json:"failover,omitempty"`
}

// PromotionPolicy defines who can approve promotions.
type PromotionPolicy struct {
	// requiredApprovers is the number of approvals needed.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	RequiredApprovers int `json:"requiredApprovers"`

	// allowedRoles lists roles that can approve promotions.
	// +kubebuilder:validation:MinItems=1
	AllowedRoles []string `json:"allowedRoles"`

	// requireSecurityScan gates promotion on image scan results.
	// +optional
	RequireSecurityScan bool `json:"requireSecurityScan,omitempty"`

	// requireTicketRef requires an external ticket reference.
	// +optional
	RequireTicketRef bool `json:"requireTicketRef,omitempty"`
}

// QuotaPolicy defines resource quota defaults.
type QuotaPolicy struct {
	// defaultPerDomain sets the default resource quotas for each domain namespace.
	// +optional
	DefaultPerDomain *DomainQuota `json:"defaultPerDomain,omitempty"`
}

// DomainQuota specifies resource limits for a domain namespace.
type DomainQuota struct {
	// +optional
	CPU resource.Quantity `json:"cpu,omitempty"`
	// +optional
	Memory resource.Quantity `json:"memory,omitempty"`
	// +optional
	Storage resource.Quantity `json:"storage,omitempty"`
}

// AppNetworkPolicy defines application-level network boundaries.
type AppNetworkPolicy struct {
	// egress defines the application-level egress allowlist.
	// +optional
	Egress *EgressPolicy `json:"egress,omitempty"`

	// ingress defines the application-level ingress policy.
	// +optional
	Ingress *IngressPolicy `json:"ingress,omitempty"`
}

// EgressPolicy defines allowed external destinations.
type EgressPolicy struct {
	// allowlist is the set of approved external destinations.
	// +optional
	Allowlist []EgressTarget `json:"allowlist,omitempty"`
}

// EgressTarget is an approved external destination.
type EgressTarget struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	// +optional
	Criticality string `json:"criticality,omitempty"`
	// +optional
	ExpectedLatency string `json:"expectedLatency,omitempty"`
	// +optional
	AlertOnErrorRate string `json:"alertOnErrorRate,omitempty"`
}

// IngressPolicy defines ingress identity requirements.
type IngressPolicy struct {
	// allowedIdPs lists approved identity providers.
	// +optional
	AllowedIdPs []IdPReference `json:"allowedIdPs,omitempty"`

	// allowAnonymousRoutes permits routes with auth=none.
	// +optional
	AllowAnonymousRoutes bool `json:"allowAnonymousRoutes,omitempty"`
}

// IdPReference is a reference to an OIDC identity provider.
type IdPReference struct {
	Issuer  string `json:"issuer"`
	JWKSUri string `json:"jwksUri"`
}

// SandboxPolicy defines sandbox lifecycle rules.
type SandboxPolicy struct {
	// maxIdleDays is the maximum number of days a sandbox can be idle before auto-destroy.
	// +optional
	MaxIdleDays *int `json:"maxIdleDays,omitempty"`

	// defaultBudgetPerDomain is the default monthly budget per domain for sandbox costs.
	// +optional
	DefaultBudgetPerDomain *resource.Quantity `json:"defaultBudgetPerDomain,omitempty"`
}

// DomainSpec defines a bounded context within an application.
type DomainSpec struct {
	// name is the domain name (e.g. "payments", "auth").
	// +kubebuilder:validation:Pattern=`^[a-z][a-z0-9-]*$`
	Name string `json:"name"`

	// owners is a list of email addresses for domain owners.
	// +optional
	Owners []string `json:"owners,omitempty"`

	// sensitivity is the data sensitivity level.
	// +kubebuilder:validation:Enum=public;internal;confidential;restricted
	// +kubebuilder:default=internal
	// +optional
	Sensitivity string `json:"sensitivity,omitempty"`

	// consumes declares dependencies on other domains.
	// +optional
	Consumes []ConsumeRef `json:"consumes,omitempty"`

	// supplies declares what this domain exposes to others.
	// +optional
	Supplies *SupplySpec `json:"supplies,omitempty"`
}

// ConsumeRef declares a dependency on another domain's service.
type ConsumeRef struct {
	Domain   string   `json:"domain"`
	Services []string `json:"services"`
	Port     int      `json:"port"`
}

// SupplySpec declares what a domain exposes.
type SupplySpec struct {
	Services []string `json:"services"`
	Port     int      `json:"port"`
}

// LinkSpec defines a cross-application link via Gateway API.
type LinkSpec struct {
	Name         string `json:"name"`
	Target       string `json:"target"`
	TargetDomain string `json:"targetDomain"`
	Port         int    `json:"port"`
	// +kubebuilder:validation:MinItems=1
	Consumers []string `json:"consumers"`
	// +optional
	Auth *LinkAuth `json:"auth,omitempty"`
	// +optional
	RateLimit *LinkRateLimit `json:"rateLimit,omitempty"`
	// +optional
	CircuitBreaker *LinkCircuitBreaker `json:"circuitBreaker,omitempty"`
}

// LinkAuth defines authentication for cross-application links.
type LinkAuth struct {
	Type string `json:"type"`
}

// LinkRateLimit defines rate limiting for a link.
type LinkRateLimit struct {
	RequestsPerMinute int `json:"requestsPerMinute"`
}

// LinkCircuitBreaker defines circuit breaker for a link.
type LinkCircuitBreaker struct {
	ConsecutiveErrors int `json:"consecutiveErrors"`
}

// ChoApplicationStatus defines the observed state of ChoApplication.
type ChoApplicationStatus struct {
	// conditions represent the current state of the ChoApplication.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// phase is the high-level phase of the application.
	// +optional
	Phase string `json:"phase,omitempty"`

	// domainNamespaces maps domain names to their namespace names.
	// +optional
	DomainNamespaces map[string]string `json:"domainNamespaces,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Compliance",type=string,JSONPath=`.spec.policy.compliance`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChoApplication is the Schema for the choapplications API.
// It represents a product and policy boundary containing one or more domains.
type ChoApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChoApplicationSpec   `json:"spec,omitempty"`
	Status ChoApplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChoApplicationList contains a list of ChoApplication.
type ChoApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChoApplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChoApplication{}, &ChoApplicationList{})
}
