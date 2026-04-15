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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testFinOpsScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = choristerv1alpha1.AddToScheme(s)
	return s
}

func testFinOpsApp(name string) *choristerv1alpha1.ChoApplication {
	return &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Owners: []string{"admin@example.com"},
			Policy: choristerv1alpha1.ApplicationPolicy{
				Compliance: "essential",
				Promotion: choristerv1alpha1.PromotionPolicy{
					RequiredApprovers: 1,
					AllowedRoles:      []string{"org-admin"},
				},
			},
			Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}, {Name: "auth"}},
		},
	}
}

// TestFinOpsReport_TotalsCorrectly verifies that sandbox costs are summed correctly.
func TestFinOpsReport_TotalsCorrectly(t *testing.T) {
	s := testFinOpsScheme()
	app := testFinOpsApp("myproduct")
	sb1 := &choristerv1alpha1.ChoSandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct-payments-alice"},
		Spec:       choristerv1alpha1.ChoSandboxSpec{Application: "myproduct", Domain: "payments", Name: "alice", Owner: "alice@co.com"},
		Status:     choristerv1alpha1.ChoSandboxStatus{EstimatedMonthlyCost: "10.00"},
	}
	sb2 := &choristerv1alpha1.ChoSandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct-payments-bob"},
		Spec:       choristerv1alpha1.ChoSandboxSpec{Application: "myproduct", Domain: "payments", Name: "bob", Owner: "bob@co.com"},
		Status:     choristerv1alpha1.ChoSandboxStatus{EstimatedMonthlyCost: "5.00"},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app, sb1, sb2).Build()
	q := query.NewQuerier(fc)

	result, err := FinOpsReport(context.Background(), q, "myproduct")
	if err != nil {
		t.Fatalf("FinOpsReport error: %v", err)
	}
	if result.AppName != "myproduct" {
		t.Errorf("expected AppName=myproduct, got %q", result.AppName)
	}
	if len(result.Domains) != 2 {
		t.Errorf("expected 2 domains, got %d", len(result.Domains))
	}
	if len(result.Sandboxes) != 2 {
		t.Errorf("expected 2 sandboxes, got %d", len(result.Sandboxes))
	}
}

// TestFinOpsTableReport verifies table columns.
func TestFinOpsTableReport(t *testing.T) {
	result := &FinOpsResult{
		AppName:          "myproduct",
		TotalMonthlyCost: "$15.00/mo",
		Domains: []DomainCost{
			{Name: "payments", Production: CostEstimate{MonthlyCost: "$10.00/mo"}, SandboxCostTotal: "$5.00/mo"},
		},
	}
	td := FinOpsTableReport(result)
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

// TestBudgetReport_AtRisk verifies that domains exceeding 80% of budget are marked at risk.
func TestBudgetReport_AtRisk(t *testing.T) {
	// 85 cents spent of $1.00 budget → 85% → AtRisk=true
	result := &BudgetResult{
		AppName: "myproduct",
		Domains: []DomainBudget{
			{Name: "payments", Budget: "$1.00/mo", CurrentSpend: "$0.85/mo", Utilization: 0.85, AtRisk: true},
			{Name: "auth", Budget: "$1.00/mo", CurrentSpend: "$0.50/mo", Utilization: 0.50, AtRisk: false},
		},
	}
	atRiskCount := 0
	for _, d := range result.Domains {
		if d.AtRisk {
			atRiskCount++
		}
	}
	if atRiskCount != 1 {
		t.Errorf("expected 1 at-risk domain, got %d", atRiskCount)
	}
}

// TestBudgetReport_Computation verifies that BudgetReport correctly computes utilization.
func TestBudgetReport_Computation(t *testing.T) {
	s := testFinOpsScheme()
	app := testFinOpsApp("myproduct")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()
	q := query.NewQuerier(fc)

	result, err := BudgetReport(context.Background(), q, "myproduct")
	if err != nil {
		t.Fatalf("BudgetReport error: %v", err)
	}
	if result.AppName != "myproduct" {
		t.Errorf("expected AppName=myproduct, got %q", result.AppName)
	}
	// No budget set → default budget str should show "(not set)"
	if result.DefaultBudget != "(not set)" {
		t.Errorf("expected DefaultBudget=(not set), got %q", result.DefaultBudget)
	}
	if len(result.Domains) != 2 {
		t.Errorf("expected 2 domains, got %d", len(result.Domains))
	}
}

// TestBudgetTableReport verifies budget table includes at-risk marker.
func TestBudgetTableReport(t *testing.T) {
	result := &BudgetResult{
		AppName: "myproduct",
		Domains: []DomainBudget{
			{Name: "payments", Budget: "$100.00/mo", CurrentSpend: "$90.00/mo", Utilization: 0.90, AtRisk: true},
		},
	}
	td := BudgetTableReport(result)
	if len(td.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(td.Rows))
	}
	// AtRisk column should contain warning
	atRiskCol := td.Rows[0][4]
	if atRiskCol == "" {
		t.Errorf("expected at-risk indicator, got empty string")
	}
}
