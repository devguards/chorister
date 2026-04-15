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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestListApplications(t *testing.T) {
	s := newScheme()
	app1 := newTestApp("product-a", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	app2 := newTestApp("product-b", []choristerv1alpha1.DomainSpec{{Name: "auth"}}, "Ready", "standard")
	app3 := newTestApp("product-c", []choristerv1alpha1.DomainSpec{{Name: "billing"}}, "Pending", "regulated")

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app1, app2, app3).Build()
	q := NewQuerier(fc)

	apps, err := q.ListApplications(context.Background())
	if err != nil {
		t.Fatalf("ListApplications error: %v", err)
	}
	if len(apps) != 3 {
		t.Fatalf("Expected 3 apps, got %d", len(apps))
	}
}

func TestGetApplication(t *testing.T) {
	s := newScheme()
	app := newTestApp("myproduct", []choristerv1alpha1.DomainSpec{
		{Name: "payments"},
		{Name: "auth"},
	}, "Ready", "standard")

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()
	q := NewQuerier(fc)

	got, err := q.GetApplication(context.Background(), "myproduct")
	if err != nil {
		t.Fatalf("GetApplication error: %v", err)
	}
	if got.Name != "myproduct" {
		t.Fatalf("Expected name 'myproduct', got %s", got.Name)
	}
	if len(got.Spec.Domains) != 2 {
		t.Fatalf("Expected 2 domains, got %d", len(got.Spec.Domains))
	}
}

func TestGetApplication_NotFound(t *testing.T) {
	s := newScheme()
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	q := NewQuerier(fc)

	_, err := q.GetApplication(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent application")
	}
}
