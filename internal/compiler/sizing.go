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

package compiler

import (
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

// ResolveSizingTemplate looks up a named sizing template for a given resource type.
// Returns the resolved template or an error if the template is not found.
func ResolveSizingTemplate(sizeName, resourceType string, templates map[string]choristerv1alpha1.SizingTemplateSet) (*choristerv1alpha1.SizingTemplate, error) {
	templateSet, ok := templates[resourceType]
	if !ok {
		return nil, fmt.Errorf("no sizing templates defined for resource type %q", resourceType)
	}

	tmpl, ok := templateSet.Templates[sizeName]
	if !ok {
		available := availableTemplateNames(templateSet)
		return nil, fmt.Errorf(
			"undefined size %q for resource type %q; available sizes: %v",
			sizeName, resourceType, available,
		)
	}

	return &tmpl, nil
}

// SizingTemplateToResourceRequirements converts a SizingTemplate to Kubernetes ResourceRequirements.
func SizingTemplateToResourceRequirements(tmpl *choristerv1alpha1.SizingTemplate) *corev1.ResourceRequirements {
	requests := corev1.ResourceList{}
	if !tmpl.CPU.IsZero() {
		requests[corev1.ResourceCPU] = tmpl.CPU
	}
	if !tmpl.Memory.IsZero() {
		requests[corev1.ResourceMemory] = tmpl.Memory
	}
	if !tmpl.Storage.IsZero() {
		requests[corev1.ResourceStorage] = tmpl.Storage
	}

	return &corev1.ResourceRequirements{
		Requests: requests,
	}
}

// ResolveResourceRequirements resolves the effective resource requirements for a resource,
// preferring explicit resources over sizing template.
// Returns nil if neither is specified.
func ResolveResourceRequirements(
	explicitResources *corev1.ResourceRequirements,
	sizeName, resourceType string,
	templates map[string]choristerv1alpha1.SizingTemplateSet,
) (*corev1.ResourceRequirements, error) {
	// Explicit resources take priority
	if explicitResources != nil {
		return explicitResources, nil
	}

	// Fall back to sizing template
	if sizeName == "" {
		return nil, nil
	}

	tmpl, err := ResolveSizingTemplate(sizeName, resourceType, templates)
	if err != nil {
		return nil, err
	}
	return SizingTemplateToResourceRequirements(tmpl), nil
}

// DefaultSizingTemplates returns the sensible default sizing templates for all resource types.
func DefaultSizingTemplates() map[string]choristerv1alpha1.SizingTemplateSet {
	return map[string]choristerv1alpha1.SizingTemplateSet{
		"compute": {
			Templates: map[string]choristerv1alpha1.SizingTemplate{
				"small":  {CPU: resource.MustParse("100m"), Memory: resource.MustParse("128Mi")},
				"medium": {CPU: resource.MustParse("500m"), Memory: resource.MustParse("512Mi")},
				"large":  {CPU: resource.MustParse("2"), Memory: resource.MustParse("2Gi")},
			},
		},
		"database": {
			Templates: map[string]choristerv1alpha1.SizingTemplate{
				"small":  {CPU: resource.MustParse("250m"), Memory: resource.MustParse("512Mi"), Storage: resource.MustParse("10Gi")},
				"medium": {CPU: resource.MustParse("1"), Memory: resource.MustParse("2Gi"), Storage: resource.MustParse("50Gi")},
				"large":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("8Gi"), Storage: resource.MustParse("200Gi")},
			},
		},
		"cache": {
			Templates: map[string]choristerv1alpha1.SizingTemplate{
				"small":  {CPU: resource.MustParse("100m"), Memory: resource.MustParse("128Mi")},
				"medium": {CPU: resource.MustParse("250m"), Memory: resource.MustParse("512Mi")},
				"large":  {CPU: resource.MustParse("1"), Memory: resource.MustParse("2Gi")},
			},
		},
		"queue": {
			Templates: map[string]choristerv1alpha1.SizingTemplate{
				"small":  {CPU: resource.MustParse("100m"), Memory: resource.MustParse("256Mi"), Storage: resource.MustParse("5Gi")},
				"medium": {CPU: resource.MustParse("500m"), Memory: resource.MustParse("1Gi"), Storage: resource.MustParse("20Gi")},
				"large":  {CPU: resource.MustParse("2"), Memory: resource.MustParse("4Gi"), Storage: resource.MustParse("100Gi")},
			},
		},
	}
}

func availableTemplateNames(set choristerv1alpha1.SizingTemplateSet) []string {
	names := make([]string, 0, len(set.Templates))
	for name := range set.Templates {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
