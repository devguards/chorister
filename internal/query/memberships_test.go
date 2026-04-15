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

func testMembership(name, app, domain, identity, role, phase string, expiresAt *metav1.Time) *choristerv1alpha1.ChoDomainMembership {
	return &choristerv1alpha1.ChoDomainMembership{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: choristerv1alpha1.ChoDomainMembershipSpec{
			Application: app,
			Domain:      domain,
			Identity:    identity,
			Role:        role,
			ExpiresAt:   expiresAt,
		},
		Status: choristerv1alpha1.ChoDomainMembershipStatus{Phase: phase},
	}
}

// TestListMemberships_AllMembers verifies that all active memberships are returned with no filters.
func TestListMemberships_AllMembers(t *testing.T) {
	s := newScheme()
	m1 := testMembership("m1", "myproduct", "payments", "alice@co.com", "developer", "Active", nil)
	m2 := testMembership("m2", "myproduct", "payments", "bob@co.com", "viewer", "Active", nil)
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(m1, m2).Build()
	q := NewQuerier(fc)

	members, err := q.ListMemberships(context.Background(), MemberFilter{App: "myproduct"})
	if err != nil {
		t.Fatalf("ListMemberships error: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("expected 2 members, got %d", len(members))
	}
}

// TestListMemberships_FilterByRole verifies role-based filtering.
func TestListMemberships_FilterByRole(t *testing.T) {
	s := newScheme()
	m1 := testMembership("m1", "myproduct", "payments", "alice@co.com", "developer", "Active", nil)
	m2 := testMembership("m2", "myproduct", "payments", "bob@co.com", "viewer", "Active", nil)
	m3 := testMembership("m3", "myproduct", "auth", "carol@co.com", "developer", "Active", nil)
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(m1, m2, m3).Build()
	q := NewQuerier(fc)

	members, err := q.ListMemberships(context.Background(), MemberFilter{App: "myproduct", Role: "developer"})
	if err != nil {
		t.Fatalf("ListMemberships error: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("expected 2 developer members, got %d", len(members))
	}
	for _, m := range members {
		if m.Role != "developer" {
			t.Errorf("expected role=developer, got %q", m.Role)
		}
	}
}

// TestListMemberships_FilterByDomain verifies domain filtering.
func TestListMemberships_FilterByDomain(t *testing.T) {
	s := newScheme()
	m1 := testMembership("m1", "myproduct", "payments", "alice@co.com", "developer", "Active", nil)
	m2 := testMembership("m2", "myproduct", "auth", "bob@co.com", "viewer", "Active", nil)
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(m1, m2).Build()
	q := NewQuerier(fc)

	members, err := q.ListMemberships(context.Background(), MemberFilter{App: "myproduct", Domain: "payments"})
	if err != nil {
		t.Fatalf("ListMemberships error: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("expected 1 member in payments domain, got %d", len(members))
	}
	if members[0].Identity != "alice@co.com" {
		t.Errorf("expected alice, got %q", members[0].Identity)
	}
}

// TestListMemberships_ExpiryComputation verifies that expiry is computed correctly.
func TestListMemberships_ExpiryComputation(t *testing.T) {
	s := newScheme()

	// Expiring soon (10 days from now)
	futureTime := metav1.Time{Time: time.Now().Add(10 * 24 * time.Hour)}
	// Already expired (48 hours ago)
	pastTime := metav1.Time{Time: time.Now().Add(-48 * time.Hour)}

	m1 := testMembership("m1", "myproduct", "payments", "alice@co.com", "developer", "Active", &futureTime)
	m2 := testMembership("m2", "myproduct", "payments", "bob@co.com", "viewer", "Expired", &pastTime)
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(m1, m2).Build()
	q := NewQuerier(fc)

	members, err := q.ListMemberships(context.Background(), MemberFilter{App: "myproduct", IncludeExpired: true})
	if err != nil {
		t.Fatalf("ListMemberships error: %v", err)
	}

	if len(members) != 2 {
		t.Fatalf("expected 2 members (including expired), got %d", len(members))
	}

	for _, m := range members {
		switch m.Identity {
		case "alice@co.com":
			if m.DaysUntilExpiry <= 0 {
				t.Errorf("alice should have positive days until expiry, got %d", m.DaysUntilExpiry)
			}
			if m.Phase != "Active" {
				t.Errorf("alice should be Active, got %q", m.Phase)
			}
		case "bob@co.com":
			if m.DaysUntilExpiry >= 0 {
				t.Errorf("bob should have negative days until expiry (expired), got %d", m.DaysUntilExpiry)
			}
			if m.Phase != "Expired" {
				t.Errorf("bob should be Expired, got %q", m.Phase)
			}
		}
	}
}

// TestListMemberships_ExpiredExcludedByDefault verifies that expired memberships are excluded by default.
func TestListMemberships_ExpiredExcludedByDefault(t *testing.T) {
	s := newScheme()
	pastTime := metav1.Time{Time: time.Now().Add(-48 * time.Hour)}
	m1 := testMembership("m1", "myproduct", "payments", "alice@co.com", "developer", "Active", nil)
	m2 := testMembership("m2", "myproduct", "payments", "bob@co.com", "viewer", "Expired", &pastTime)
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(m1, m2).Build()
	q := NewQuerier(fc)

	members, err := q.ListMemberships(context.Background(), MemberFilter{App: "myproduct"})
	if err != nil {
		t.Fatalf("ListMemberships error: %v", err)
	}
	for _, m := range members {
		if m.Phase == "Expired" {
			t.Errorf("expired member %q should not appear without IncludeExpired=true", m.Identity)
		}
	}
}
