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
	"fmt"
	"strconv"

	"github.com/chorister-dev/chorister/internal/query"
)

// CostEstimate holds an estimated monthly cost string.
type CostEstimate struct {
	MonthlyCost string // formatted as "$12.50/mo"
}

// DomainCost holds the production and sandbox cost breakdown for a domain.
type DomainCost struct {
	Name             string
	Production       CostEstimate
	SandboxCostTotal string
}

// SandboxCost holds cost info for a single sandbox.
type SandboxCost struct {
	Name        string
	Domain      string
	Owner       string
	MonthlyCost string
	Idle        bool
}

// FinOpsResult aggregates cost data for an application.
type FinOpsResult struct {
	AppName          string
	TotalMonthlyCost string
	Domains          []DomainCost
	Sandboxes        []SandboxCost
}

// FinOpsReport builds a cost breakdown for the given application.
// It aggregates sandbox costs from ChoSandbox.status.estimatedMonthlyCost
// and derives domain production estimates from resource counts and cluster rates.
func FinOpsReport(ctx context.Context, q *query.Querier, appName string) (*FinOpsResult, error) {
	// Get all domains for the app
	domains, err := q.ListDomainsByApp(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}

	// Get all sandboxes for the app
	allSandboxes, err := q.ListAllSandboxes(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("list sandboxes: %w", err)
	}

	result := &FinOpsResult{AppName: appName}
	totalCents := 0

	// Group sandboxes by domain
	sandboxByDomain := map[string][]query.SandboxInfo{}
	for _, sb := range allSandboxes {
		sandboxByDomain[sb.Domain] = append(sandboxByDomain[sb.Domain], sb)
		if sb.EstimatedMonthlyCost != "" {
			result.Sandboxes = append(result.Sandboxes, SandboxCost{
				Name:        sb.Name,
				Domain:      sb.Domain,
				Owner:       sb.Owner,
				MonthlyCost: FormatCost(sb.EstimatedMonthlyCost),
				Idle:        sb.IdleWarning,
			})
			totalCents += parseCostCents(sb.EstimatedMonthlyCost)
		}
	}

	// Estimate per-domain costs
	for _, d := range domains {
		sbList := sandboxByDomain[d.Name]
		sbTotal := 0
		for _, sb := range sbList {
			sbTotal += parseCostCents(sb.EstimatedMonthlyCost)
		}

		// Production cost: use resource count as rough proxy
		// (without real CPU/memory metrics, we estimate based on compute count)
		prodCost := estimateProductionCost(d.ResourceCount)
		totalCents += prodCost

		result.Domains = append(result.Domains, DomainCost{
			Name: d.Name,
			Production: CostEstimate{
				MonthlyCost: formatCostFromCents(prodCost),
			},
			SandboxCostTotal: formatCostFromCents(sbTotal),
		})
	}

	result.TotalMonthlyCost = formatCostFromCents(totalCents)
	return result, nil
}

// estimateProductionCost returns a rough monthly cost estimate in cents
// based on the number of resources in a domain.
func estimateProductionCost(resourceCount int) int {
	// $15/resource/month as a rough estimate
	return resourceCount * 1500
}

// parseCostCents parses a cost string like "12.50" (USD) and returns cents.
func parseCostCents(s string) int {
	if s == "" || s == "0" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int(f * 100)
}

// formatCostFromCents formats cents as a "$12.50/mo" string.
func formatCostFromCents(cents int) string {
	if cents == 0 {
		return "$0.00/mo"
	}
	return fmt.Sprintf("$%d.%02d/mo", cents/100, cents%100)
}

// FinOpsTableReport produces a domain-level cost breakdown table.
// Columns: DOMAIN, PROD-COST, SANDBOX-COST
func FinOpsTableReport(result *FinOpsResult) TableData {
	td := TableData{
		Headers: []string{"DOMAIN", "PROD-COST", "SANDBOX-COST"},
		Rows:    make([][]string, 0, len(result.Domains)),
	}
	for _, d := range result.Domains {
		td.Rows = append(td.Rows, []string{
			d.Name,
			d.Production.MonthlyCost,
			d.SandboxCostTotal,
		})
	}
	return td
}

// SandboxCostTableReport produces a sandbox-level cost breakdown table.
// Columns: NAME, DOMAIN, OWNER, COST/MO, IDLE
func SandboxCostTableReport(sandboxes []SandboxCost) TableData {
	td := TableData{
		Headers: []string{"NAME", "DOMAIN", "OWNER", "COST/MO", "IDLE"},
		Rows:    make([][]string, 0, len(sandboxes)),
	}
	for _, s := range sandboxes {
		idle := ""
		if s.Idle {
			idle = "⚠ idle"
		}
		td.Rows = append(td.Rows, []string{s.Name, s.Domain, s.Owner, s.MonthlyCost, idle})
	}
	return td
}

// DomainBudget holds budget utilization for a domain.
type DomainBudget struct {
	Name         string
	Budget       string  // monthly budget
	CurrentSpend string  // current estimated spend
	Utilization  float64 // 0.0 to 1.0+
	AtRisk       bool    // true if utilization > 80%
}

// BudgetResult aggregates budget status for an application.
type BudgetResult struct {
	AppName       string
	DefaultBudget string
	Domains       []DomainBudget
}

// BudgetReport computes budget utilization for each domain in the application.
// The default budget comes from ChoApplication.spec.policy.sandbox.defaultBudgetPerDomain.
// Current spend is the sum of sandbox estimated costs for each domain.
func BudgetReport(ctx context.Context, q *query.Querier, appName string) (*BudgetResult, error) {
	app, err := q.GetApplication(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("get application: %w", err)
	}

	defaultBudgetCents := 0
	defaultBudgetStr := "(not set)"
	if app.Spec.Policy.Sandbox != nil && app.Spec.Policy.Sandbox.DefaultBudgetPerDomain != nil {
		defaultBudgetCents = int(*app.Spec.Policy.Sandbox.DefaultBudgetPerDomain * 100)
		defaultBudgetStr = formatCostFromCents(defaultBudgetCents)
	}

	domains, err := q.ListDomainsByApp(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}

	allSandboxes, err := q.ListAllSandboxes(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("list sandboxes: %w", err)
	}

	// Sum sandbox costs per domain
	spendByDomain := map[string]int{}
	for _, sb := range allSandboxes {
		spendByDomain[sb.Domain] += parseCostCents(sb.EstimatedMonthlyCost)
	}

	result := &BudgetResult{
		AppName:       appName,
		DefaultBudget: defaultBudgetStr,
	}

	for _, d := range domains {
		spend := spendByDomain[d.Name]
		util := 0.0
		if defaultBudgetCents > 0 {
			util = float64(spend) / float64(defaultBudgetCents)
		}
		result.Domains = append(result.Domains, DomainBudget{
			Name:         d.Name,
			Budget:       defaultBudgetStr,
			CurrentSpend: formatCostFromCents(spend),
			Utilization:  util,
			AtRisk:       util > 0.80,
		})
	}

	return result, nil
}

// BudgetTableReport produces a budget utilization table.
// Columns: DOMAIN, BUDGET, SPEND, UTILIZATION, AT-RISK
func BudgetTableReport(result *BudgetResult) TableData {
	td := TableData{
		Headers: []string{"DOMAIN", "BUDGET", "SPEND", "UTILIZATION", "AT-RISK"},
		Rows:    make([][]string, 0, len(result.Domains)),
	}
	for _, d := range result.Domains {
		util := fmt.Sprintf("%.0f%%", d.Utilization*100)
		atRisk := ""
		if d.AtRisk {
			atRisk = "⚠ yes"
		}
		td.Rows = append(td.Rows, []string{d.Name, d.Budget, d.CurrentSpend, util, atRisk})
	}
	return td
}
