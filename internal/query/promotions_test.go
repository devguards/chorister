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

func TestListPromotionRequests(t *testing.T) {
	s := newScheme()
	pr1 := &choristerv1alpha1.ChoPromotionRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-1", Namespace: "default", CreationTimestamp: metav1.Now()},
		Spec: choristerv1alpha1.ChoPromotionRequestSpec{
			Application: "myproduct", Domain: "payments", Sandbox: "alice", RequestedBy: "alice@co.com",
		},
		Status: choristerv1alpha1.ChoPromotionRequestStatus{Phase: "Pending"},
	}
	pr2 := &choristerv1alpha1.ChoPromotionRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-2", Namespace: "default", CreationTimestamp: metav1.Now()},
		Spec: choristerv1alpha1.ChoPromotionRequestSpec{
			Application: "myproduct", Domain: "auth", Sandbox: "bob", RequestedBy: "bob@co.com",
		},
		Status: choristerv1alpha1.ChoPromotionRequestStatus{Phase: "Approved"},
	}
	pr3 := &choristerv1alpha1.ChoPromotionRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-3", Namespace: "default", CreationTimestamp: metav1.Now()},
		Spec: choristerv1alpha1.ChoPromotionRequestSpec{
			Application: "myproduct", Domain: "payments", Sandbox: "carol", RequestedBy: "carol@co.com",
		},
		Status: choristerv1alpha1.ChoPromotionRequestStatus{Phase: "Pending"},
	}
	pr4 := &choristerv1alpha1.ChoPromotionRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-4", Namespace: "default", CreationTimestamp: metav1.Now()},
		Spec: choristerv1alpha1.ChoPromotionRequestSpec{
			Application: "other-app", Domain: "billing", Sandbox: "dave", RequestedBy: "dave@co.com",
		},
		Status: choristerv1alpha1.ChoPromotionRequestStatus{Phase: "Rejected"},
	}

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(pr1, pr2, pr3, pr4).Build()
	q := NewQuerier(fc)

	// No filter
	all, err := q.ListPromotionRequests(context.Background(), PromotionFilter{})
	if err != nil {
		t.Fatalf("ListPromotionRequests error: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("Expected 4, got %d", len(all))
	}

	// Filter by status=Pending
	pending, err := q.ListPromotionRequests(context.Background(), PromotionFilter{Status: "Pending"})
	if err != nil {
		t.Fatalf("ListPromotionRequests error: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("Expected 2 pending, got %d", len(pending))
	}

	// Filter by domain=payments
	payments, err := q.ListPromotionRequests(context.Background(), PromotionFilter{Domain: "payments"})
	if err != nil {
		t.Fatalf("ListPromotionRequests error: %v", err)
	}
	if len(payments) != 2 {
		t.Fatalf("Expected 2 for payments domain, got %d", len(payments))
	}

	// Filter by app
	otherApp, err := q.ListPromotionRequests(context.Background(), PromotionFilter{App: "other-app"})
	if err != nil {
		t.Fatalf("ListPromotionRequests error: %v", err)
	}
	if len(otherApp) != 1 {
		t.Fatalf("Expected 1 for other-app, got %d", len(otherApp))
	}
}
