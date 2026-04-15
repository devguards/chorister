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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = choristerv1alpha1.AddToScheme(s)
	return s
}

func TestNewQuerier(t *testing.T) {
	s := newScheme()
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	q := NewQuerier(fc)
	if q == nil {
		t.Fatal("NewQuerier returned nil")
	}
}

func TestQuerier_GetNotFound(t *testing.T) {
	s := newScheme()
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	q := NewQuerier(fc)

	_, err := q.GetApplication(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent application")
	}
}

func TestQuerier_ListEmpty(t *testing.T) {
	s := newScheme()
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	q := NewQuerier(fc)

	apps, err := q.ListApplications(context.Background())
	if err != nil {
		t.Fatalf("ListApplications should not error on empty: %v", err)
	}
	if len(apps) != 0 {
		t.Fatalf("Expected 0 apps, got %d", len(apps))
	}
}

func TestWrapError(t *testing.T) {
	err := wrapError("ChoApplication", "myapp", "", nil)
	if err != nil {
		t.Fatal("wrapError should return nil for nil error")
	}

	err = wrapError("ChoApplication", "myapp", "", context.Canceled)
	if err == nil {
		t.Fatal("wrapError should return non-nil for non-nil error")
	}
	if got := err.Error(); got != "ChoApplication myapp: context canceled" {
		t.Fatalf("unexpected error: %s", got)
	}

	err = wrapError("ChoCompute", "api", "ns1", context.Canceled)
	if err == nil {
		t.Fatal("wrapError should return non-nil for non-nil error")
	}
	if got := err.Error(); got != "ChoCompute ns1/api: context canceled" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func newTestApp(name string, domains []choristerv1alpha1.DomainSpec, phase string, compliance string) *choristerv1alpha1.ChoApplication {
	return &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.Now(),
		},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Owners:  []string{"admin@example.com"},
			Policy:  choristerv1alpha1.ApplicationPolicy{Compliance: compliance, Promotion: choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"org-admin"}}},
			Domains: domains,
		},
		Status: choristerv1alpha1.ChoApplicationStatus{
			Phase: phase,
		},
	}
}
