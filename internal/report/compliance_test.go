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
	"time"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/query"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testReportScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = choristerv1alpha1.AddToScheme(s)
	return s
}

func testReportApp(name, compliance string, domains []choristerv1alpha1.DomainSpec) *choristerv1alpha1.ChoApplication {
	return &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Owners: []string{"admin@example.com"},
			Policy: choristerv1alpha1.ApplicationPolicy{
				Compliance: compliance,
				Promotion: choristerv1alpha1.PromotionPolicy{
					RequiredApprovers: 1,
					AllowedRoles:      []string{"org-admin"},
				},
			},
			Domains: domains,
		},
	}
}

// TestComplianceReport_Essential verifies essential profile includes CIS Controls checks.
func TestComplianceReport_Essential(t *testing.T) {
	s := testReportScheme()
	app := testReportApp("myproduct", "essential", []choristerv1alpha1.DomainSpec{{Name: "payments"}})
	cluster := &choristerv1alpha1.ChoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "chorister-cluster"},
		Status: choristerv1alpha1.ChoClusterStatus{
			CISBenchmark:   "Pass",
			OperatorStatus: map[string]string{"gatekeeper": "installed"},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app, cluster).Build()
	q := query.NewQuerier(fc)

	result, err := ComplianceReport(context.Background(), q, app)
	if err != nil {
		t.Fatalf("ComplianceReport error: %v", err)
	}
	if result.Profile != "essential" {
		t.Errorf("expected profile=essential, got %q", result.Profile)
	}
	if len(result.Checks) == 0 {
		t.Fatal("expected at least one compliance check")
	}

	// CC-1 should pass (gatekeeper installed)
	foundCC1 := false
	for _, c := range result.Checks {
		if c.ControlID == "CC-1" {
			foundCC1 = true
			if c.Status != "Pass" {
				t.Errorf("CC-1 should Pass when gatekeeper=installed, got %q", c.Status)
			}
		}
	}
	if !foundCC1 {
		t.Error("expected CC-1 check to be present")
	}
}

// TestComplianceReport_Regulated verifies additional ISO 27001 checks for regulated profile.
func TestComplianceReport_Regulated(t *testing.T) {
	s := testReportScheme()
	app := testReportApp("myproduct", "regulated", []choristerv1alpha1.DomainSpec{
		{Name: "payments", Sensitivity: "restricted"},
	})
	app.Spec.Policy.Promotion.RequireSecurityScan = true
	app.Spec.Policy.AuditRetention = "2y"
	cluster := &choristerv1alpha1.ChoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "chorister-cluster"},
		Status: choristerv1alpha1.ChoClusterStatus{
			CISBenchmark:   "Pass",
			OperatorStatus: map[string]string{"gatekeeper": "installed", "tetragon": "installed"},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app, cluster).Build()
	q := query.NewQuerier(fc)

	result, err := ComplianceReport(context.Background(), q, app)
	if err != nil {
		t.Fatalf("ComplianceReport error: %v", err)
	}

	// Regulated profile should include ISO 27001 checks
	hasISO := false
	for _, c := range result.Checks {
		if c.Framework == "ISO 27001" {
			hasISO = true
			break
		}
	}
	if !hasISO {
		t.Error("regulated profile should include ISO 27001 checks")
	}
}

// TestComplianceStatusSummary verifies compliant vs non-compliant classification.
func TestComplianceStatusSummary_Compliant(t *testing.T) {
	result := &ComplianceResult{
		AppName: "myproduct",
		Profile: "essential",
		Checks: []ComplianceCheck{
			{Framework: "CIS Controls", ControlID: "CC-1", Status: "Pass", Description: "test"},
			{Framework: "CIS Controls", ControlID: "CC-2", Status: "Pass", Description: "test"},
		},
	}
	ss := ComplianceStatusSummary(result)
	if ss.Phase != "Compliant" {
		t.Errorf("expected Compliant, got %q", ss.Phase)
	}
	if ss.Details["Fail"] != "0" {
		t.Errorf("expected Fail=0, got %q", ss.Details["Fail"])
	}
}

func TestComplianceStatusSummary_NonCompliant(t *testing.T) {
	result := &ComplianceResult{
		AppName: "myproduct",
		Profile: "essential",
		Checks: []ComplianceCheck{
			{Framework: "CIS Controls", ControlID: "CC-1", Status: "Pass"},
			{Framework: "CIS Controls", ControlID: "CC-2", Status: "Fail"},
		},
	}
	ss := ComplianceStatusSummary(result)
	if ss.Phase != "NonCompliant" {
		t.Errorf("expected NonCompliant, got %q", ss.Phase)
	}
}

// TestAuditReport verifies AuditReport table structure.
func TestAuditReport(t *testing.T) {
	entries := []query.AuditEntry{
		{
			Timestamp:   time.Now(),
			Actor:       "alice@co.com",
			Action:      "promote",
			Resource:    "ledger",
			Namespace:   "payments",
			Domain:      "payments",
			Application: "myproduct",
			Result:      "success",
		},
	}
	td := AuditReport(entries)
	if len(td.Headers) == 0 {
		t.Fatal("expected headers")
	}
	if len(td.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(td.Rows))
	}
	if td.Rows[0][1] != "alice@co.com" {
		t.Errorf("expected actor alice@co.com, got %q", td.Rows[0][1])
	}
}

// TestMembershipAuditReport_Flags verifies that expired + restricted-no-expiry memberships are flagged.
func TestMembershipAuditReport_Flags(t *testing.T) {
	s := testReportScheme()
	app := testReportApp("myproduct", "regulated", []choristerv1alpha1.DomainSpec{
		{Name: "payments", Sensitivity: "restricted"},
	})

	// Member without expiry on restricted domain
	m1 := &choristerv1alpha1.ChoDomainMembership{
		ObjectMeta: metav1.ObjectMeta{Name: "m1"},
		Spec: choristerv1alpha1.ChoDomainMembershipSpec{
			Application: "myproduct", Domain: "payments", Identity: "alice@co.com", Role: "developer",
		},
		Status: choristerv1alpha1.ChoDomainMembershipStatus{Phase: "Active"},
	}

	// Expired member
	pastTime := metav1.Time{Time: time.Now().Add(-48 * time.Hour)}
	m2 := &choristerv1alpha1.ChoDomainMembership{
		ObjectMeta: metav1.ObjectMeta{Name: "m2"},
		Spec: choristerv1alpha1.ChoDomainMembershipSpec{
			Application: "myproduct", Domain: "payments", Identity: "bob@co.com", Role: "viewer",
			ExpiresAt: &pastTime,
		},
		Status: choristerv1alpha1.ChoDomainMembershipStatus{Phase: "Expired"},
	}

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app, m1, m2).Build()
	q := query.NewQuerier(fc)

	result, err := MembershipAuditReport(context.Background(), q, "myproduct")
	if err != nil {
		t.Fatalf("MembershipAuditReport error: %v", err)
	}

	if result.ExpiredCount == 0 {
		t.Error("expected at least 1 expired membership")
	}
	if result.StaleCount == 0 {
		t.Error("expected at least 1 no-expiry-on-restricted membership")
	}

	identities := map[string]bool{}
	for _, e := range result.Entries {
		identities[e.Identity] = true
	}
	if !identities["alice@co.com"] {
		t.Error("alice should be flagged (no expiry on restricted domain)")
	}
	if !identities["bob@co.com"] {
		t.Error("bob should be flagged (expired)")
	}
}
