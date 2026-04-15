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
	"testing"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/query"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAppListReport(t *testing.T) {
	apps := []choristerv1alpha1.ChoApplication{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "app1", CreationTimestamp: metav1.Now()},
			Spec: choristerv1alpha1.ChoApplicationSpec{
				Owners:  []string{"a@co.com"},
				Policy:  choristerv1alpha1.ApplicationPolicy{Compliance: "essential", Promotion: choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"org-admin"}}},
				Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}, {Name: "auth"}},
			},
			Status: choristerv1alpha1.ChoApplicationStatus{Phase: "Ready"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "app2", CreationTimestamp: metav1.Now()},
			Spec: choristerv1alpha1.ChoApplicationSpec{
				Owners:  []string{"b@co.com"},
				Policy:  choristerv1alpha1.ApplicationPolicy{Compliance: "regulated", Promotion: choristerv1alpha1.PromotionPolicy{RequiredApprovers: 2, AllowedRoles: []string{"org-admin"}}},
				Domains: []choristerv1alpha1.DomainSpec{{Name: "billing"}},
			},
			Status: choristerv1alpha1.ChoApplicationStatus{Phase: "Pending"},
		},
	}

	td := AppListReport(apps)
	if len(td.Headers) != 5 {
		t.Fatalf("Expected 5 headers, got %d", len(td.Headers))
	}
	if len(td.Rows) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(td.Rows))
	}
	// app1 row
	if td.Rows[0][0] != "app1" {
		t.Errorf("Expected row 0 name 'app1', got %q", td.Rows[0][0])
	}
	if td.Rows[0][1] != "2" {
		t.Errorf("Expected row 0 domains '2', got %q", td.Rows[0][1])
	}
	if td.Rows[0][2] != "essential" {
		t.Errorf("Expected row 0 compliance 'essential', got %q", td.Rows[0][2])
	}
}

func TestAppDetailReport(t *testing.T) {
	app := &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct"},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Owners: []string{"admin@co.com"},
			Policy: choristerv1alpha1.ApplicationPolicy{
				Compliance: "standard",
				Promotion: choristerv1alpha1.PromotionPolicy{
					RequiredApprovers:   2,
					AllowedRoles:        []string{"org-admin", "domain-admin"},
					RequireSecurityScan: true,
				},
			},
			Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}, {Name: "auth"}},
		},
		Status: choristerv1alpha1.ChoApplicationStatus{Phase: "Ready"},
	}
	domains := []query.DomainInfo{
		{Name: "payments", Application: "myproduct"},
		{Name: "auth", Application: "myproduct"},
	}

	ss := AppDetailReport(app, domains)
	if ss.Name != "myproduct" {
		t.Errorf("Expected name 'myproduct', got %q", ss.Name)
	}
	if ss.Details["Compliance"] != "standard" {
		t.Errorf("Expected compliance 'standard', got %q", ss.Details["Compliance"])
	}
	if ss.Details["Domain Count"] != "2" {
		t.Errorf("Expected domain count '2', got %q", ss.Details["Domain Count"])
	}
	if ss.Details["Security Scan Required"] != "true" {
		t.Errorf("Expected security scan required 'true', got %q", ss.Details["Security Scan Required"])
	}
}

func TestDomainListReport(t *testing.T) {
	domains := []query.DomainInfo{
		{Name: "payments", Application: "myproduct", Namespace: "myproduct-payments", Sensitivity: "confidential", Phase: "Active", ResourceCount: 5},
		{Name: "auth", Application: "myproduct", Namespace: "myproduct-auth", Sensitivity: "internal", Phase: "Active", ResourceCount: 2, Isolated: true},
	}

	td := DomainListReport(domains)
	if len(td.Headers) != 7 {
		t.Fatalf("Expected 7 headers, got %d", len(td.Headers))
	}
	if len(td.Rows) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(td.Rows))
	}
	// Check isolated marker
	if td.Rows[1][6] != "⚠ yes" {
		t.Errorf("Expected isolated marker '⚠ yes' for auth, got %q", td.Rows[1][6])
	}
	if td.Rows[0][6] != "" {
		t.Errorf("Expected empty isolated marker for payments, got %q", td.Rows[0][6])
	}
}

func TestDomainDetailReport(t *testing.T) {
	domain := query.DomainInfo{
		Name:        "payments",
		Application: "myproduct",
		Namespace:   "myproduct-payments",
		Sensitivity: "confidential",
		Phase:       "Active",
	}
	resources := &query.DomainResources{
		Computes:  make([]choristerv1alpha1.ChoCompute, 2),
		Databases: make([]choristerv1alpha1.ChoDatabase, 1),
	}

	ss := DomainDetailReport(domain, resources)
	if ss.Name != "payments" {
		t.Errorf("Expected name 'payments', got %q", ss.Name)
	}
	if ss.Details["Computes"] != "2" {
		t.Errorf("Expected computes '2', got %q", ss.Details["Computes"])
	}
	if ss.Details["Total"] != "3" {
		t.Errorf("Expected total '3', got %q", ss.Details["Total"])
	}
}

func TestClusterStatusReport(t *testing.T) {
	cluster := &choristerv1alpha1.ChoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "chorister"},
		Spec: choristerv1alpha1.ChoClusterSpec{
			ControllerRevision: "v1.0.0",
		},
		Status: choristerv1alpha1.ChoClusterStatus{
			Phase:              "Ready",
			ObservabilityReady: true,
			CISBenchmark:       "Pass",
			OperatorStatus: map[string]string{
				"kro":       "Installed",
				"stackgres": "Degraded",
			},
		},
	}

	ss := ClusterStatusReport(cluster)
	if ss.Name != "chorister" {
		t.Errorf("Expected name 'chorister', got %q", ss.Name)
	}
	if ss.Phase != "Ready" {
		t.Errorf("Expected phase 'Ready', got %q", ss.Phase)
	}
	if ss.Details["Controller Revision"] != "v1.0.0" {
		t.Errorf("Expected controller revision 'v1.0.0', got %q", ss.Details["Controller Revision"])
	}
	if ss.Details["Operator/stackgres"] != "Degraded" {
		t.Errorf("Expected stackgres Degraded, got %q", ss.Details["Operator/stackgres"])
	}
}

func TestOperatorListReport(t *testing.T) {
	ops := []query.OperatorInfo{
		{Name: "kro", Version: "v0.2.0", Status: "Installed"},
		{Name: "stackgres", Version: "1.12.0", Status: "Degraded"},
		{Name: "cert-manager", Version: "", Status: "Unknown"},
	}

	td := OperatorListReport(ops)
	if len(td.Headers) != 3 {
		t.Fatalf("Expected 3 headers, got %d", len(td.Headers))
	}
	if len(td.Rows) != 3 {
		t.Fatalf("Expected 3 rows, got %d", len(td.Rows))
	}
	// cert-manager with no version should show "-"
	if td.Rows[2][1] != "-" {
		t.Errorf("Expected '-' for empty version, got %q", td.Rows[2][1])
	}
}

func TestSandboxListReport(t *testing.T) {
	now := metav1.Now()
	sandboxes := []query.SandboxInfo{
		{Name: "alice", Owner: "alice@co.com", Domain: "payments", Age: now.Time, EstimatedMonthlyCost: "12.50"},
		{Name: "bob", Owner: "bob@co.com", Domain: "payments", Age: now.Time, LastApplyTime: &now, IdleWarning: true, EstimatedMonthlyCost: "8.00"},
	}

	td := SandboxListReport(sandboxes)
	if len(td.Headers) != 7 {
		t.Fatalf("Expected 7 headers, got %d", len(td.Headers))
	}
	if len(td.Rows) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(td.Rows))
	}
	// Check idle marker
	if td.Rows[1][6] != "⚠ idle" {
		t.Errorf("Expected idle marker for bob, got %q", td.Rows[1][6])
	}
	// Check cost formatting
	if td.Rows[0][5] != "$12.50/mo" {
		t.Errorf("Expected '$12.50/mo', got %q", td.Rows[0][5])
	}
}

func TestSandboxDetailReport(t *testing.T) {
	now := metav1.Now()
	detail := &query.SandboxDetail{
		SandboxInfo: query.SandboxInfo{
			Name: "alice", Owner: "alice@co.com", Domain: "payments",
			Application: "myproduct", Phase: "Active", Age: now.Time,
			EstimatedMonthlyCost: "15.00", IdleWarning: true,
			LastApplyTime: &now,
		},
		Resources: &query.DomainResources{
			Computes: make([]choristerv1alpha1.ChoCompute, 2),
		},
		Conditions: []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionTrue, Reason: "AllReady"},
		},
	}

	ss := SandboxDetailReport(detail)
	if ss.Name != "alice" {
		t.Errorf("Expected name 'alice', got %q", ss.Name)
	}
	if ss.Details["Idle Warning"] == "" {
		t.Error("Expected idle warning in details")
	}
	if ss.Details["Total Resources"] != "2" {
		t.Errorf("Expected total resources '2', got %q", ss.Details["Total Resources"])
	}
}

func TestPromotionListReport(t *testing.T) {
	promotions := []query.PromotionInfo{
		{Name: "pr-1", Domain: "payments", RequestedBy: "alice@co.com", Phase: "Pending", ApprovalCount: 0, CreatedAt: metav1.Now().Time},
		{Name: "pr-2", Domain: "auth", RequestedBy: "bob@co.com", Phase: "Approved", ApprovalCount: 2, CreatedAt: metav1.Now().Time},
	}

	td := PromotionListReport(promotions)
	if len(td.Headers) != 6 {
		t.Fatalf("Expected 6 headers, got %d", len(td.Headers))
	}
	if len(td.Rows) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(td.Rows))
	}
}

func TestVulnSummaryReport(t *testing.T) {
	reports := []query.VulnReportInfo{
		{Domain: "payments", CriticalCount: 2, HighCount: 3, Scanner: "trivy", Phase: "Completed"},
		{Domain: "auth", CriticalCount: 0, HighCount: 1, Scanner: "trivy", Phase: "Completed"},
	}

	td := VulnSummaryReport(reports)
	if len(td.Headers) != 6 {
		t.Fatalf("Expected 6 headers, got %d", len(td.Headers))
	}
	if len(td.Rows) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(td.Rows))
	}
	if td.Rows[0][1] != "2" {
		t.Errorf("Expected critical count '2', got %q", td.Rows[0][1])
	}
}

func TestEventListReport(t *testing.T) {
	events := []query.EventInfo{
		{Time: metav1.Now().Time, Type: "Normal", Reason: "Reconciled", InvolvedObject: "ChoCompute/api", Message: "OK"},
		{Time: metav1.Now().Time, Type: "Warning", Reason: "FailedCreate", InvolvedObject: "ChoDatabase/ledger", Message: "Failed"},
	}

	td := EventListReport(events)
	if len(td.Headers) != 5 {
		t.Fatalf("Expected 5 headers, got %d", len(td.Headers))
	}
	if len(td.Rows) != 2 {
		t.Fatalf("Expected 2 rows, got %d", len(td.Rows))
	}
}
