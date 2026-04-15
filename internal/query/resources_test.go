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

func TestListDomainResources(t *testing.T) {
	s := newScheme()
	ns := "myproduct-payments"

	compute1 := &choristerv1alpha1.ChoCompute{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: ns},
		Spec:       choristerv1alpha1.ChoComputeSpec{Application: "myproduct", Domain: "payments", Image: "api:v1"},
	}
	compute2 := &choristerv1alpha1.ChoCompute{
		ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: ns},
		Spec:       choristerv1alpha1.ChoComputeSpec{Application: "myproduct", Domain: "payments", Image: "worker:v1"},
	}
	db1 := &choristerv1alpha1.ChoDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "ledger", Namespace: ns},
		Spec:       choristerv1alpha1.ChoDatabaseSpec{Application: "myproduct", Domain: "payments", Engine: "postgres"},
	}

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(compute1, compute2, db1).Build()
	q := NewQuerier(fc)

	resources, err := q.ListDomainResources(context.Background(), ns)
	if err != nil {
		t.Fatalf("ListDomainResources error: %v", err)
	}
	if len(resources.Computes) != 2 {
		t.Fatalf("Expected 2 computes, got %d", len(resources.Computes))
	}
	if len(resources.Databases) != 1 {
		t.Fatalf("Expected 1 database, got %d", len(resources.Databases))
	}
	if resources.TotalCount() != 3 {
		t.Fatalf("Expected total count 3, got %d", resources.TotalCount())
	}
}

func TestListDomainResources_Empty(t *testing.T) {
	s := newScheme()
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	q := NewQuerier(fc)

	resources, err := q.ListDomainResources(context.Background(), "nonexistent-ns")
	if err != nil {
		t.Fatalf("ListDomainResources error: %v", err)
	}
	if resources.TotalCount() != 0 {
		t.Fatalf("Expected 0 resources, got %d", resources.TotalCount())
	}
}
