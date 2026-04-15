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

package query

import (
	"context"
	"testing"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestListDomainsByApp(t *testing.T) {
	s := newScheme()
	app := &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct"},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Owners: []string{"admin@example.com"},
			Policy: choristerv1alpha1.ApplicationPolicy{
				Compliance: "standard",
				Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"org-admin"}},
			},
			Domains: []choristerv1alpha1.DomainSpec{
				{Name: "payments", Sensitivity: "confidential"},
				{Name: "auth", Sensitivity: "internal"},
			},
		},
		Status: choristerv1alpha1.ChoApplicationStatus{
			Phase: "Ready",
			DomainNamespaces: map[string]string{
				"payments": "myproduct-payments",
				"auth":     "myproduct-auth",
			},
		},
	}

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()
	q := NewQuerier(fc)

	domains, err := q.ListDomainsByApp(context.Background(), "myproduct")
	if err != nil {
		t.Fatalf("ListDomainsByApp error: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("Expected 2 domains, got %d", len(domains))
	}

	for _, d := range domains {
		if d.Application != "myproduct" {
			t.Errorf("Expected application 'myproduct', got %s", d.Application)
		}
		if d.Phase != "Active" {
			t.Errorf("Expected phase 'Active' for domain %s, got %s", d.Name, d.Phase)
		}
	}
}

func TestListAllDomains_AllApps(t *testing.T) {
	s := newScheme()
	app1 := newTestApp("app1", []choristerv1alpha1.DomainSpec{
		{Name: "payments"},
		{Name: "auth"},
		{Name: "billing"},
	}, "Ready", "essential")
	app2 := newTestApp("app2", []choristerv1alpha1.DomainSpec{
		{Name: "orders"},
		{Name: "shipping"},
		{Name: "returns"},
	}, "Ready", "standard")

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app1, app2).Build()
	q := NewQuerier(fc)

	domains, err := q.ListAllDomains(context.Background(), "")
	if err != nil {
		t.Fatalf("ListAllDomains error: %v", err)
	}
	if len(domains) != 6 {
		t.Fatalf("Expected 6 domains, got %d", len(domains))
	}
}

func TestListAllDomains_FilteredByApp(t *testing.T) {
	s := newScheme()
	app1 := newTestApp("app1", []choristerv1alpha1.DomainSpec{
		{Name: "payments"},
		{Name: "auth"},
		{Name: "billing"},
	}, "Ready", "essential")
	app2 := newTestApp("app2", []choristerv1alpha1.DomainSpec{
		{Name: "orders"},
		{Name: "shipping"},
		{Name: "returns"},
	}, "Ready", "standard")

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app1, app2).Build()
	q := NewQuerier(fc)

	domains, err := q.ListAllDomains(context.Background(), "app1")
	if err != nil {
		t.Fatalf("ListAllDomains filtered error: %v", err)
	}
	if len(domains) != 3 {
		t.Fatalf("Expected 3 domains for app1, got %d", len(domains))
	}
}

func TestListDomains_IsolatedDetection(t *testing.T) {
	s := newScheme()
	app := &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "myproduct",
			Annotations: map[string]string{
				"chorister.dev/isolate-payments": "true",
			},
		},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Owners: []string{"admin@example.com"},
			Policy: choristerv1alpha1.ApplicationPolicy{
				Compliance: "essential",
				Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"org-admin"}},
			},
			Domains: []choristerv1alpha1.DomainSpec{
				{Name: "payments"},
				{Name: "auth"},
			},
		},
	}

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()
	q := NewQuerier(fc)

	domains, err := q.ListDomainsByApp(context.Background(), "myproduct")
	if err != nil {
		t.Fatalf("ListDomainsByApp error: %v", err)
	}

	var isolatedCount int
	for _, d := range domains {
		if d.Isolated {
			isolatedCount++
		}
	}
	if isolatedCount != 1 {
		t.Fatalf("Expected 1 isolated domain, got %d", isolatedCount)
	}
}
