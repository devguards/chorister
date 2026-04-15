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

package report

import (
	"context"
	"testing"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/query"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testCapacityScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = choristerv1alpha1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	return s
}

// TestQuotaReport_WithResourceQuota verifies that CPU/Memory quotas are populated from a ResourceQuota.
func TestQuotaReport_WithResourceQuota(t *testing.T) {
	s := testCapacityScheme()
	app := &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct"},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}},
			Policy:  choristerv1alpha1.ApplicationPolicy{Compliance: "essential", Promotion: choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"org-admin"}}},
		},
		Status: choristerv1alpha1.ChoApplicationStatus{
			DomainNamespaces: map[string]string{"payments": "myproduct-payments"},
		},
	}
	rq := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "myproduct-payments"},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
			Used: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1"),
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app, rq).Build()
	q := query.NewQuerier(fc)

	result, err := QuotaReport(context.Background(), q, "myproduct", "")
	if err != nil {
		t.Fatalf("QuotaReport error: %v", err)
	}
	if len(result.Domains) == 0 {
		t.Fatal("expected at least one domain")
	}
	d := result.Domains[0]
	if d.CPU.Used == "(no quota)" {
		t.Errorf("expected CPU quota to be populated, got '(no quota)'")
	}
}

// TestQuotaReport_NoQuota verifies graceful handling when there's no ResourceQuota.
func TestQuotaReport_NoQuota(t *testing.T) {
	s := testCapacityScheme()
	app := &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct"},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}},
			Policy:  choristerv1alpha1.ApplicationPolicy{Compliance: "essential", Promotion: choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"org-admin"}}},
		},
		Status: choristerv1alpha1.ChoApplicationStatus{
			DomainNamespaces: map[string]string{"payments": "myproduct-payments"},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()
	q := query.NewQuerier(fc)

	result, err := QuotaReport(context.Background(), q, "myproduct", "")
	if err != nil {
		t.Fatalf("QuotaReport error: %v", err)
	}
	if len(result.Domains) == 0 {
		t.Fatal("expected at least one domain")
	}
	d := result.Domains[0]
	if d.CPU.Limit != "(no quota)" {
		t.Errorf("expected CPU limit to be '(no quota)', got %q", d.CPU.Limit)
	}
}

// TestQuotaTableReport verifies table structure.
func TestQuotaTableReport(t *testing.T) {
	result := &QuotaResult{
		AppName: "myproduct",
		Domains: []DomainQuotaUsage{
			{
				Name:      "payments",
				Namespace: "myproduct-payments",
				CPU:       ResourceUsage{Used: "1", Limit: "4"},
				Memory:    ResourceUsage{Used: "2Gi", Limit: "8Gi"},
			},
		},
	}
	td := QuotaTableReport(result)
	if len(td.Headers) == 0 {
		t.Fatal("expected headers")
	}
	if len(td.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(td.Rows))
	}
	if td.Rows[0][0] != "payments" {
		t.Errorf("expected domain name payments, got %q", td.Rows[0][0])
	}
}

// TestQuotaReport_DomainFilter verifies that domain filtering works.
func TestQuotaReport_DomainFilter(t *testing.T) {
	s := testCapacityScheme()
	app := &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct"},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Domains: []choristerv1alpha1.DomainSpec{
				{Name: "payments"},
				{Name: "auth"},
			},
			Policy: choristerv1alpha1.ApplicationPolicy{Compliance: "essential", Promotion: choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"org-admin"}}},
		},
		Status: choristerv1alpha1.ChoApplicationStatus{
			DomainNamespaces: map[string]string{"payments": "myproduct-payments", "auth": "myproduct-auth"},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()
	q := query.NewQuerier(fc)

	result, err := QuotaReport(context.Background(), q, "myproduct", "payments")
	if err != nil {
		t.Fatalf("QuotaReport error: %v", err)
	}
	if len(result.Domains) != 1 {
		t.Errorf("expected 1 domain after filter, got %d", len(result.Domains))
	}
	if result.Domains[0].Name != "payments" {
		t.Errorf("expected domain name payments, got %q", result.Domains[0].Name)
	}
}
