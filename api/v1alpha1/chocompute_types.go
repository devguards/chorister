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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ChoComputeSpec defines the desired state of ChoCompute.
type ChoComputeSpec struct {
	// application is the parent application name.
	Application string `json:"application"`

	// domain is the domain this compute belongs to.
	Domain string `json:"domain"`

	// image is the container image reference.
	Image string `json:"image"`

	// variant is the workload type.
	// +kubebuilder:validation:Enum=long-running;job;cronjob;gpu
	// +kubebuilder:default=long-running
	// +optional
	Variant string `json:"variant,omitempty"`

	// replicas is the desired replica count (for long-running variant).
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// port is the primary container port.
	// +optional
	Port *int32 `json:"port,omitempty"`

	// schedule is the cron schedule (required when variant=cronjob).
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// resources defines explicit CPU/memory requests and limits.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// size references a sizing template from ChoCluster.
	// +optional
	Size string `json:"size,omitempty"`

	// autoscaling defines HPA configuration.
	// +optional
	Autoscaling *AutoscalingSpec `json:"autoscaling,omitempty"`

	// gpu defines GPU resource requirements (for gpu variant).
	// +optional
	GPU *GPUSpec `json:"gpu,omitempty"`

	// env defines environment variables for the container.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// command overrides the container entrypoint.
	// +optional
	Command []string `json:"command,omitempty"`

	// args overrides the container arguments.
	// +optional
	Args []string `json:"args,omitempty"`
}

// AutoscalingSpec defines horizontal pod autoscaling configuration.
type AutoscalingSpec struct {
	// minReplicas is the lower bound for autoscaling.
	// +kubebuilder:validation:Minimum=1
	MinReplicas int32 `json:"minReplicas"`

	// maxReplicas is the upper bound for autoscaling.
	MaxReplicas int32 `json:"maxReplicas"`

	// targetCPUPercent is the target CPU utilization percentage.
	// +optional
	TargetCPUPercent *int32 `json:"targetCPUPercent,omitempty"`

	// targetMemoryPercent is the target memory utilization percentage.
	// +optional
	TargetMemoryPercent *int32 `json:"targetMemoryPercent,omitempty"`
}

// GPUSpec defines GPU resource requirements.
type GPUSpec struct {
	// count is the number of GPUs required.
	// +kubebuilder:validation:Minimum=1
	Count int `json:"count"`

	// type is the GPU resource name (e.g. "nvidia.com/gpu").
	// +kubebuilder:default="nvidia.com/gpu"
	// +optional
	Type string `json:"type,omitempty"`

	// memory is the minimum GPU memory required.
	// +optional
	Memory *resource.Quantity `json:"memory,omitempty"`
}

// ChoComputeStatus defines the observed state of ChoCompute.
type ChoComputeStatus struct {
	// conditions represent the current state of the ChoCompute resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ready indicates whether the compute resource is fully available.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// readyReplicas is the number of ready replicas.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// compiledWithRevision is the controller revision that compiled this resource.
	// +optional
	CompiledWithRevision string `json:"compiledWithRevision,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Variant",type=string,JSONPath=`.spec.variant`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChoCompute is the Schema for the chocomputes API.
type ChoCompute struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChoComputeSpec   `json:"spec,omitempty"`
	Status ChoComputeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChoComputeList contains a list of ChoCompute.
type ChoComputeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChoCompute `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChoCompute{}, &ChoComputeList{})
}
