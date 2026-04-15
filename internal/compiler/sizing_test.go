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
	"testing"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestResolveSizingTemplate_Valid(t *testing.T) {
	templates := map[string]choristerv1alpha1.SizingTemplateSet{
		"database": {
			Templates: map[string]choristerv1alpha1.SizingTemplate{
				"small":  {CPU: resource.MustParse("250m"), Memory: resource.MustParse("512Mi"), Storage: resource.MustParse("10Gi")},
				"medium": {CPU: resource.MustParse("1"), Memory: resource.MustParse("2Gi"), Storage: resource.MustParse("50Gi")},
				"large":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("8Gi"), Storage: resource.MustParse("200Gi")},
			},
		},
	}

	tmpl, err := ResolveSizingTemplate("medium", "database", templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tmpl.CPU.Cmp(resource.MustParse("1")) != 0 {
		t.Errorf("expected CPU 1, got %s", tmpl.CPU.String())
	}
	if tmpl.Memory.Cmp(resource.MustParse("2Gi")) != 0 {
		t.Errorf("expected Memory 2Gi, got %s", tmpl.Memory.String())
	}
	if tmpl.Storage.Cmp(resource.MustParse("50Gi")) != 0 {
		t.Errorf("expected Storage 50Gi, got %s", tmpl.Storage.String())
	}
}

func TestResolveSizingTemplate_UndefinedSize(t *testing.T) {
	templates := map[string]choristerv1alpha1.SizingTemplateSet{
		"database": {
			Templates: map[string]choristerv1alpha1.SizingTemplate{
				"small":  {CPU: resource.MustParse("250m"), Memory: resource.MustParse("512Mi")},
				"medium": {CPU: resource.MustParse("1"), Memory: resource.MustParse("2Gi")},
			},
		},
	}

	_, err := ResolveSizingTemplate("jumbo", "database", templates)
	if err == nil {
		t.Fatal("expected error for undefined size, got nil")
	}
	if !contains(err.Error(), "undefined size") {
		t.Errorf("expected error about undefined size, got: %s", err.Error())
	}
	if !contains(err.Error(), "jumbo") {
		t.Errorf("expected error to include template name 'jumbo', got: %s", err.Error())
	}
	if !contains(err.Error(), "small") || !contains(err.Error(), "medium") {
		t.Errorf("expected error to include available options, got: %s", err.Error())
	}
}

func TestResolveSizingTemplate_UndefinedResourceType(t *testing.T) {
	templates := map[string]choristerv1alpha1.SizingTemplateSet{
		"database": {
			Templates: map[string]choristerv1alpha1.SizingTemplate{
				"small": {CPU: resource.MustParse("250m"), Memory: resource.MustParse("512Mi")},
			},
		},
	}

	_, err := ResolveSizingTemplate("small", "nosuchtype", templates)
	if err == nil {
		t.Fatal("expected error for undefined resource type, got nil")
	}
	if !contains(err.Error(), "nosuchtype") {
		t.Errorf("expected error to mention resource type, got: %s", err.Error())
	}
}

func TestSizingTemplateToResourceRequirements(t *testing.T) {
	tmpl := &choristerv1alpha1.SizingTemplate{
		CPU:     resource.MustParse("500m"),
		Memory:  resource.MustParse("1Gi"),
		Storage: resource.MustParse("20Gi"),
	}

	reqs := SizingTemplateToResourceRequirements(tmpl)
	if reqs == nil {
		t.Fatal("expected non-nil resource requirements")
	}

	if cpu, ok := reqs.Requests[corev1.ResourceCPU]; !ok || cpu.Cmp(resource.MustParse("500m")) != 0 {
		t.Errorf("expected CPU 500m, got %v", reqs.Requests[corev1.ResourceCPU])
	}
	if mem, ok := reqs.Requests[corev1.ResourceMemory]; !ok || mem.Cmp(resource.MustParse("1Gi")) != 0 {
		t.Errorf("expected Memory 1Gi, got %v", reqs.Requests[corev1.ResourceMemory])
	}
	if stor, ok := reqs.Requests[corev1.ResourceStorage]; !ok || stor.Cmp(resource.MustParse("20Gi")) != 0 {
		t.Errorf("expected Storage 20Gi, got %v", reqs.Requests[corev1.ResourceStorage])
	}
}

func TestSizingTemplateToResourceRequirements_ZeroValues(t *testing.T) {
	tmpl := &choristerv1alpha1.SizingTemplate{
		CPU:    resource.MustParse("100m"),
		Memory: resource.MustParse("128Mi"),
		// Storage is zero
	}

	reqs := SizingTemplateToResourceRequirements(tmpl)
	if _, ok := reqs.Requests[corev1.ResourceStorage]; ok {
		t.Error("expected storage to be absent when zero")
	}
}

func TestResolveResourceRequirements_ExplicitOverride(t *testing.T) {
	templates := map[string]choristerv1alpha1.SizingTemplateSet{
		"compute": {
			Templates: map[string]choristerv1alpha1.SizingTemplate{
				"small": {CPU: resource.MustParse("100m"), Memory: resource.MustParse("128Mi")},
			},
		},
	}

	explicit := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}

	// Explicit resources should win even when size is also specified
	reqs, err := ResolveResourceRequirements(explicit, "small", "compute", templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cpu := reqs.Requests[corev1.ResourceCPU]; cpu.Cmp(resource.MustParse("2")) != 0 {
		t.Errorf("expected explicit CPU 2, got %s", cpu.String())
	}
}

func TestResolveResourceRequirements_SizeTemplate(t *testing.T) {
	templates := map[string]choristerv1alpha1.SizingTemplateSet{
		"database": {
			Templates: map[string]choristerv1alpha1.SizingTemplate{
				"medium": {CPU: resource.MustParse("1"), Memory: resource.MustParse("2Gi"), Storage: resource.MustParse("50Gi")},
			},
		},
	}

	reqs, err := ResolveResourceRequirements(nil, "medium", "database", templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cpu := reqs.Requests[corev1.ResourceCPU]; cpu.Cmp(resource.MustParse("1")) != 0 {
		t.Errorf("expected CPU 1 from template, got %s", cpu.String())
	}
}

func TestResolveResourceRequirements_NeitherSpecified(t *testing.T) {
	templates := map[string]choristerv1alpha1.SizingTemplateSet{}

	reqs, err := ResolveResourceRequirements(nil, "", "compute", templates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reqs != nil {
		t.Errorf("expected nil when neither explicit nor size specified, got %v", reqs)
	}
}

func TestResolveResourceRequirements_InvalidSize(t *testing.T) {
	templates := map[string]choristerv1alpha1.SizingTemplateSet{
		"cache": {
			Templates: map[string]choristerv1alpha1.SizingTemplate{
				"small": {CPU: resource.MustParse("100m"), Memory: resource.MustParse("128Mi")},
			},
		},
	}

	_, err := ResolveResourceRequirements(nil, "xlarge", "cache", templates)
	if err == nil {
		t.Fatal("expected error for invalid size, got nil")
	}
	if !contains(err.Error(), "xlarge") {
		t.Errorf("expected error to include size name, got: %s", err.Error())
	}
}

func TestDefaultSizingTemplates(t *testing.T) {
	defaults := DefaultSizingTemplates()

	// All four resource types should be defined
	for _, resType := range []string{"compute", "database", "cache", "queue"} {
		set, ok := defaults[resType]
		if !ok {
			t.Errorf("expected default templates for %q", resType)
			continue
		}
		// Each should have small/medium/large
		for _, size := range []string{"small", "medium", "large"} {
			tmpl, ok := set.Templates[size]
			if !ok {
				t.Errorf("expected %q size for %q, not found", size, resType)
				continue
			}
			if tmpl.CPU.IsZero() {
				t.Errorf("expected non-zero CPU for %s/%s", resType, size)
			}
			if tmpl.Memory.IsZero() {
				t.Errorf("expected non-zero Memory for %s/%s", resType, size)
			}
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
