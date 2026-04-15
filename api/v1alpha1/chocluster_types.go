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

// ChoClusterSpec defines the desired state of ChoCluster.
type ChoClusterSpec struct {
	// operators defines the operators to install and their versions.
	// +optional
	Operators *OperatorVersions `json:"operators,omitempty"`

	// sizingTemplates defines named resource sizing templates.
	// +optional
	SizingTemplates map[string]SizingTemplateSet `json:"sizingTemplates,omitempty"`

	// finops defines cost estimation configuration.
	// +optional
	FinOps *FinOpsSpec `json:"finops,omitempty"`

	// controllerRevision is the current stable controller revision name.
	// +optional
	ControllerRevision string `json:"controllerRevision,omitempty"`

	// revisions defines the blue-green controller revisions for upgrades.
	// +optional
	Revisions []ControllerRevisionEntry `json:"revisions,omitempty"`

	// observability defines the observability stack configuration.
	// +optional
	Observability *ObservabilitySpec `json:"observability,omitempty"`

	// externalSecretBackend configures the external secret backend for production environments.
	// +optional
	ExternalSecretBackend *ExternalSecretBackendSpec `json:"externalSecretBackend,omitempty"`
}

// ControllerRevisionEntry defines a named controller revision with a tag.
type ControllerRevisionEntry struct {
	// name is the revision identifier (e.g. "1-4", "1-5").
	Name string `json:"name"`

	// tag is the revision tag (e.g. "stable", "canary").
	Tag string `json:"tag"`
}

// ExternalSecretBackendSpec configures external secret management.
type ExternalSecretBackendSpec struct {
	// provider is the external secret backend provider.
	// +kubebuilder:validation:Enum=gcp;aws;azure
	Provider string `json:"provider"`

	// secretStoreRef is the name of the ClusterSecretStore or SecretStore to use.
	SecretStoreRef string `json:"secretStoreRef"`
}

// OperatorVersions defines desired versions for managed operators.
type OperatorVersions struct {
	// +optional
	Kro string `json:"kro,omitempty"`
	// +optional
	StackGres string `json:"stackgres,omitempty"`
	// +optional
	NATS string `json:"nats,omitempty"`
	// +optional
	Dragonfly string `json:"dragonfly,omitempty"`
	// +optional
	CertManager string `json:"certManager,omitempty"`
	// +optional
	Gatekeeper string `json:"gatekeeper,omitempty"`
	// +optional
	Tetragon string `json:"tetragon,omitempty"`
}

// SizingTemplateSet defines named sizing templates for a resource type.
type SizingTemplateSet struct {
	// templates maps size names (e.g. "small", "medium", "large") to resource definitions.
	Templates map[string]SizingTemplate `json:"templates"`
}

// SizingTemplate defines resource allocations for a named size.
type SizingTemplate struct {
	// +optional
	CPU resource.Quantity `json:"cpu,omitempty"`
	// +optional
	Memory resource.Quantity `json:"memory,omitempty"`
	// +optional
	Storage resource.Quantity `json:"storage,omitempty"`
	// instances is the number of database instances (e.g. 1 for single, 2+ for HA).
	// +optional
	Instances *int32 `json:"instances,omitempty"`
	// replicas is the number of queue/cache replicas.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
}

// FinOpsSpec defines cost estimation rates.
type FinOpsSpec struct {
	// rates defines per-unit cost rates.
	// +optional
	Rates *CostRates `json:"rates,omitempty"`
}

// CostRates defines cost per resource unit.
type CostRates struct {
	// cpuPerHour is the cost per CPU core per hour.
	// +optional
	CPUPerHour *resource.Quantity `json:"cpuPerHour,omitempty"`
	// memoryPerGBHour is the cost per GB memory per hour.
	// +optional
	MemoryPerGBHour *resource.Quantity `json:"memoryPerGBHour,omitempty"`
	// storagePerGBMonth is the cost per GB storage per month.
	// +optional
	StoragePerGBMonth *resource.Quantity `json:"storagePerGBMonth,omitempty"`
	// postgresSmall is the flat monthly rate for a small PostgreSQL instance.
	// +optional
	PostgresSmall *resource.Quantity `json:"postgresSmall,omitempty"`
	// postgresMedium is the flat monthly rate for a medium PostgreSQL instance.
	// +optional
	PostgresMedium *resource.Quantity `json:"postgresMedium,omitempty"`
	// postgresLarge is the flat monthly rate for a large PostgreSQL instance.
	// +optional
	PostgresLarge *resource.Quantity `json:"postgresLarge,omitempty"`
}

// ObservabilitySpec configures the cluster-wide observability stack (Grafana LGTM).
type ObservabilitySpec struct {
	// enabled controls whether the observability stack is deployed. Defaults to true.
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// monitoringNamespace is the namespace where observability components are deployed.
	// +kubebuilder:default="cho-monitoring"
	// +optional
	MonitoringNamespace string `json:"monitoringNamespace,omitempty"`

	// versions specifies component versions for the observability stack.
	// +optional
	Versions *ObservabilityVersions `json:"versions,omitempty"`

	// retention defines data retention policies.
	// +optional
	Retention *RetentionSpec `json:"retention,omitempty"`
}

// ObservabilityVersions specifies the version of each observability component.
type ObservabilityVersions struct {
	// +optional
	Alloy string `json:"alloy,omitempty"`
	// +optional
	Mimir string `json:"mimir,omitempty"`
	// +optional
	Loki string `json:"loki,omitempty"`
	// +optional
	Tempo string `json:"tempo,omitempty"`
	// +optional
	Grafana string `json:"grafana,omitempty"`
}

// RetentionSpec defines retention periods for observability data.
type RetentionSpec struct {
	// metrics retention period (e.g. "30d"). Defaults to 30d.
	// +optional
	Metrics string `json:"metrics,omitempty"`
	// logs retention period (e.g. "14d"). Defaults to 14d.
	// +optional
	Logs string `json:"logs,omitempty"`
	// traces retention period (e.g. "7d"). Defaults to 7d.
	// +optional
	Traces string `json:"traces,omitempty"`
}

// ChoClusterStatus defines the observed state of ChoCluster.
type ChoClusterStatus struct {
	// conditions represent the current state of the ChoCluster.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// phase is the cluster lifecycle state.
	// +optional
	Phase string `json:"phase,omitempty"`

	// operatorStatus tracks the status of each managed operator.
	// +optional
	OperatorStatus map[string]string `json:"operatorStatus,omitempty"`

	// cisBenchmark stores the latest kube-bench results.
	// +optional
	CISBenchmark string `json:"cisBenchmark,omitempty"`

	// observabilityReady indicates whether the monitoring stack is operational.
	// +optional
	ObservabilityReady bool `json:"observabilityReady,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChoCluster is the Schema for the choclusters API.
// It bootstraps the full operator stack and defines cluster-wide configuration.
type ChoCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChoClusterSpec   `json:"spec,omitempty"`
	Status ChoClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChoClusterList contains a list of ChoCluster.
type ChoClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChoCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChoCluster{}, &ChoClusterList{})
}
