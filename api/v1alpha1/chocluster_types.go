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
