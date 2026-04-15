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
	"time"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestListSandboxesByDomain(t *testing.T) {
	s := newScheme()
	sb1 := &choristerv1alpha1.ChoSandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct-payments-alice", Namespace: "default", CreationTimestamp: metav1.Now()},
		Spec:       choristerv1alpha1.ChoSandboxSpec{Application: "myproduct", Domain: "payments", Name: "alice", Owner: "alice@co.com"},
		Status:     choristerv1alpha1.ChoSandboxStatus{Phase: "Active", Namespace: "myproduct-payments-sbx-alice"},
	}
	sb2 := &choristerv1alpha1.ChoSandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct-payments-bob", Namespace: "default", CreationTimestamp: metav1.Now()},
		Spec:       choristerv1alpha1.ChoSandboxSpec{Application: "myproduct", Domain: "payments", Name: "bob", Owner: "bob@co.com"},
		Status:     choristerv1alpha1.ChoSandboxStatus{Phase: "Active", Namespace: "myproduct-payments-sbx-bob"},
	}
	sb3 := &choristerv1alpha1.ChoSandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct-auth-carol", Namespace: "default", CreationTimestamp: metav1.Now()},
		Spec:       choristerv1alpha1.ChoSandboxSpec{Application: "myproduct", Domain: "auth", Name: "carol", Owner: "carol@co.com"},
		Status:     choristerv1alpha1.ChoSandboxStatus{Phase: "Active"},
	}

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(sb1, sb2, sb3).Build()
	q := NewQuerier(fc)

	results, err := q.ListSandboxesByDomain(context.Background(), "myproduct", "payments")
	if err != nil {
		t.Fatalf("ListSandboxesByDomain error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Expected 2 sandboxes for payments, got %d", len(results))
	}
}

func TestListSandboxesByDomain_IdleWarning(t *testing.T) {
	s := newScheme()
	oldTime := metav1.NewTime(time.Now().Add(-10 * 24 * time.Hour))
	sb := &choristerv1alpha1.ChoSandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct-payments-stale", Namespace: "default", CreationTimestamp: metav1.Now()},
		Spec:       choristerv1alpha1.ChoSandboxSpec{Application: "myproduct", Domain: "payments", Name: "stale", Owner: "dev@co.com"},
		Status:     choristerv1alpha1.ChoSandboxStatus{Phase: "Active", LastApplyTime: &oldTime},
	}

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(sb).Build()
	q := NewQuerier(fc)

	results, err := q.ListSandboxesByDomain(context.Background(), "myproduct", "payments")
	if err != nil {
		t.Fatalf("ListSandboxesByDomain error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Expected 1 sandbox, got %d", len(results))
	}
	if !results[0].IdleWarning {
		t.Error("Expected idle warning for stale sandbox")
	}
}

func TestGetSandbox(t *testing.T) {
	s := newScheme()
	sb := &choristerv1alpha1.ChoSandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct-payments-alice", Namespace: "default", CreationTimestamp: metav1.Now()},
		Spec:       choristerv1alpha1.ChoSandboxSpec{Application: "myproduct", Domain: "payments", Name: "alice", Owner: "alice@co.com"},
		Status: choristerv1alpha1.ChoSandboxStatus{
			Phase:                "Active",
			Namespace:            "myproduct-payments-sbx-alice",
			EstimatedMonthlyCost: "12.50",
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllResourcesReady"},
			},
		},
	}

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(sb).Build()
	q := NewQuerier(fc)

	detail, err := q.GetSandbox(context.Background(), "myproduct", "payments", "alice")
	if err != nil {
		t.Fatalf("GetSandbox error: %v", err)
	}
	if detail.Name != "alice" {
		t.Errorf("Expected name 'alice', got %q", detail.Name)
	}
	if detail.EstimatedMonthlyCost != "12.50" {
		t.Errorf("Expected cost '12.50', got %q", detail.EstimatedMonthlyCost)
	}
	if len(detail.Conditions) != 1 {
		t.Errorf("Expected 1 condition, got %d", len(detail.Conditions))
	}
}

func TestGetSandbox_NotFound(t *testing.T) {
	s := newScheme()
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	q := NewQuerier(fc)

	_, err := q.GetSandbox(context.Background(), "myproduct", "payments", "nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent sandbox")
	}
}
