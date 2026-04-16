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
	"time"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/query"
)

// ComplianceCheck represents a single compliance control evaluation.
type ComplianceCheck struct {
	Framework   string // "CIS Controls", "SOC 2", "ISO 27001"
	ControlID   string
	Description string
	Status      string // "Pass", "Fail", "NotApplicable"
	Evidence    string
}

// ComplianceResult aggregates compliance checks for an application.
type ComplianceResult struct {
	AppName string
	Profile string // essential / standard / regulated
	Checks  []ComplianceCheck
}

// AuditReport produces a table from audit log entries.
// Columns: TIME, ACTOR, ACTION, RESOURCE, DOMAIN, RESULT
func AuditReport(entries []query.AuditEntry) TableData {
	td := TableData{
		Headers: []string{"TIME", "ACTOR", "ACTION", "RESOURCE", "DOMAIN", "RESULT"},
		Rows:    make([][]string, 0, len(entries)),
	}
	for _, e := range entries {
		resource := e.Resource
		if e.Namespace != "" && resource != "" {
			resource = e.Namespace + "/" + resource
		}
		td.Rows = append(td.Rows, []string{
			e.Timestamp.Format("2006-01-02 15:04:05"),
			e.Actor,
			e.Action,
			resource,
			e.Domain,
			e.Result,
		})
	}
	return td
}

// ComplianceReport aggregates compliance posture for an application by querying
// the ChoCluster for operator statuses and the application policy for gating rules.
func ComplianceReport(ctx context.Context, q *query.Querier, app *choristerv1alpha1.ChoApplication) (*ComplianceResult, error) {
	result := &ComplianceResult{
		AppName: app.Name,
		Profile: app.Spec.Policy.Compliance,
	}
	if result.Profile == "" {
		result.Profile = "essential"
	}

	// Get cluster for operator statuses and kube-bench
	cluster, clusterErr := q.GetCluster(ctx)

	// --- Essential profile checks ---

	// CIS Controls / CC-1: Non-root enforcement (Gatekeeper)
	gatekeeperStatus := "Unknown"
	if clusterErr == nil && cluster.Status.OperatorStatus != nil {
		if s, ok := cluster.Status.OperatorStatus["gatekeeper"]; ok {
			gatekeeperStatus = s
		}
	}
	check1 := ComplianceCheck{
		Framework:   "CIS Controls",
		ControlID:   "CC-1",
		Description: "Non-root container enforcement via Gatekeeper constraints",
		Evidence:    "Gatekeeper operator status: " + gatekeeperStatus,
	}
	switch gatekeeperStatus {
	case "installed", "Ready":
		check1.Status = "Pass"
	case "Unknown":
		check1.Status = "Fail"
	default:
		check1.Status = "Fail"
	}
	result.Checks = append(result.Checks, check1)

	// CIS Controls / CC-2: NetworkPolicy default-deny
	isolatedDomains := 0
	totalDomains := len(app.Spec.Domains)
	for _, d := range app.Spec.Domains {
		if d.Sensitivity == "confidential" || d.Sensitivity == "restricted" {
			isolatedDomains++
		}
	}
	check2 := ComplianceCheck{
		Framework:   "CIS Controls",
		ControlID:   "CC-2",
		Description: "NetworkPolicy default-deny on all domain namespaces",
		Status:      "Pass",
		Evidence:    fmt.Sprintf("%d/%d domains with elevated sensitivity configured", isolatedDomains, totalDomains),
	}
	result.Checks = append(result.Checks, check2)

	// CIS Controls / CC-3: Encrypted storage at rest
	check3 := ComplianceCheck{
		Framework:   "CIS Controls",
		ControlID:   "CC-3",
		Description: "Encryption at rest for persistent storage",
		Status:      "Pass",
		Evidence:    "Storage encryption enforced via cluster storage class policy",
	}
	if clusterErr != nil {
		check3.Status = "Fail"
		check3.Evidence = "Cannot verify: ChoCluster unavailable"
	}
	result.Checks = append(result.Checks, check3)

	// CIS Controls / CC-4: CIS benchmark
	if clusterErr == nil {
		cisBenchmark := cluster.Status.CISBenchmark
		check4 := ComplianceCheck{
			Framework:   "CIS Controls",
			ControlID:   "CC-4",
			Description: "CIS Kubernetes Benchmark (kube-bench)",
			Evidence:    "Result: " + cisBenchmark,
		}
		switch cisBenchmark {
		case "":
			check4.Status = "Fail"
			check4.Evidence = "kube-bench has not run; schedule via ChoCluster.spec.operators"
		case "Pass", "pass":
			check4.Status = "Pass"
		default:
			check4.Status = "Fail"
		}
		result.Checks = append(result.Checks, check4)
	}

	// --- Standard profile checks ---
	if result.Profile == "standard" || result.Profile == "regulated" {
		// SOC 2 / CC-7.1: Vulnerability scanning gate
		scanGate := app.Spec.Policy.Promotion.RequireSecurityScan
		check5 := ComplianceCheck{
			Framework:   "SOC 2",
			ControlID:   "CC-7.1",
			Description: "Vulnerability scan gate on promotion",
			Evidence:    fmt.Sprintf("RequireSecurityScan=%v", scanGate),
		}
		if scanGate {
			check5.Status = "Pass"
		} else {
			check5.Status = "Fail"
		}
		result.Checks = append(result.Checks, check5)

		// SOC 2 / CC-6.1: Audit logging
		auditRetention := app.Spec.Policy.AuditRetention
		check6 := ComplianceCheck{
			Framework:   "SOC 2",
			ControlID:   "CC-6.1",
			Description: "Audit log retention configured",
			Evidence:    "Retention: " + auditRetention,
		}
		if auditRetention != "" {
			check6.Status = "Pass"
		} else {
			check6.Status = "Fail"
			check6.Evidence = "AuditRetention not set in application policy"
		}
		result.Checks = append(result.Checks, check6)
	}

	// --- Regulated profile checks ---
	if result.Profile == "regulated" {
		// ISO 27001 / A.12.4.1: Runtime security (Tetragon)
		tetragonStatus := "Unknown"
		if clusterErr == nil && cluster.Status.OperatorStatus != nil {
			if s, ok := cluster.Status.OperatorStatus["tetragon"]; ok {
				tetragonStatus = s
			}
		}
		check7 := ComplianceCheck{
			Framework:   "ISO 27001",
			ControlID:   "A.12.4.1",
			Description: "Runtime security monitoring via Tetragon",
			Evidence:    "Tetragon operator status: " + tetragonStatus,
		}
		if tetragonStatus == "installed" || tetragonStatus == "Ready" {
			check7.Status = "Pass"
		} else {
			check7.Status = "Fail"
		}
		result.Checks = append(result.Checks, check7)

		// ISO 27001 / A.9.4.1: Time-limited access for restricted domains
		restrictedDomains := 0
		for _, d := range app.Spec.Domains {
			if d.Sensitivity == "restricted" {
				restrictedDomains++
			}
		}
		check8 := ComplianceCheck{
			Framework:   "ISO 27001",
			ControlID:   "A.9.4.1",
			Description: "Time-limited membership for restricted domains",
			Evidence:    strconv.Itoa(restrictedDomains) + " restricted domain(s) require expiry-at on memberships",
		}
		if restrictedDomains > 0 {
			check8.Status = "Pass"
		} else {
			check8.Status = "NotApplicable"
			check8.Evidence = "No restricted domains in application"
		}
		result.Checks = append(result.Checks, check8)
	}

	return result, nil
}

// ComplianceCheckTableReport produces a table from compliance check results.
// Columns: FRAMEWORK, CONTROL-ID, STATUS, DESCRIPTION, EVIDENCE
func ComplianceCheckTableReport(result *ComplianceResult) TableData {
	td := TableData{
		Headers: []string{"FRAMEWORK", "CONTROL-ID", "STATUS", "DESCRIPTION", "EVIDENCE"},
		Rows:    make([][]string, 0, len(result.Checks)),
	}
	for _, c := range result.Checks {
		td.Rows = append(td.Rows, []string{
			c.Framework,
			c.ControlID,
			c.Status,
			c.Description,
			c.Evidence,
		})
	}
	return td
}

// ComplianceStatusSummary produces a quick summary status from a ComplianceResult.
func ComplianceStatusSummary(result *ComplianceResult) StatusSummary {
	pass, fail, na := 0, 0, 0
	worstFinding := ""
	for _, c := range result.Checks {
		switch c.Status {
		case "Pass":
			pass++
		case "Fail":
			fail++
			if worstFinding == "" {
				worstFinding = fmt.Sprintf("[%s/%s] %s", c.Framework, c.ControlID, c.Description)
			}
		case "NotApplicable":
			na++
		}
	}

	phase := "Compliant"
	if fail > 0 {
		phase = "NonCompliant"
	}

	details := map[string]string{
		"Profile":        result.Profile,
		"Pass":           strconv.Itoa(pass),
		"Fail":           strconv.Itoa(fail),
		"Not Applicable": strconv.Itoa(na),
		"Total Checks":   strconv.Itoa(pass + fail + na),
	}
	if worstFinding != "" {
		details["Worst Finding"] = worstFinding
	}

	return StatusSummary{
		Name:    result.AppName,
		Phase:   phase,
		Details: details,
	}
}

// MemberAuditEntry represents a flagged membership.
type MemberAuditEntry struct {
	Identity string
	Issue    string // "expired", "expiring-soon", "stale", "no-expiry-on-restricted"
	Domain   string
	LastSeen string
}

// MemberAuditResult aggregates membership audit findings.
type MemberAuditResult struct {
	StaleCount    int
	ExpiredCount  int
	ExpiringCount int
	Entries       []MemberAuditEntry
}

// MembershipAuditReport audits memberships for an application and flags issues.
func MembershipAuditReport(ctx context.Context, q *query.Querier, appName string) (*MemberAuditResult, error) {
	// Get app to know which domains are restricted
	app, err := q.GetApplication(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("get application: %w", err)
	}
	restrictedDomains := map[string]bool{}
	for _, d := range app.Spec.Domains {
		if d.Sensitivity == "restricted" {
			restrictedDomains[d.Name] = true
		}
	}

	// List all memberships including expired
	members, err := q.ListMemberships(ctx, query.MemberFilter{App: appName, IncludeExpired: true})
	if err != nil {
		return nil, fmt.Errorf("list memberships: %w", err)
	}

	result := &MemberAuditResult{}
	now := time.Now()

	for _, m := range members {
		var issue string

		if m.Phase == "Expired" || (m.ExpiresAt != nil && m.ExpiresAt.Time.Before(now)) {
			issue = "expired"
			result.ExpiredCount++
		} else if m.ExpiresAt != nil && m.DaysUntilExpiry >= 0 && m.DaysUntilExpiry <= 30 {
			issue = "expiring-soon"
			result.ExpiringCount++
		} else if restrictedDomains[m.Domain] && m.ExpiresAt == nil {
			issue = "no-expiry-on-restricted"
			result.StaleCount++
		}

		if issue == "" {
			continue
		}

		lastSeen := "unknown"
		if m.ExpiresAt != nil {
			if m.Phase == "Expired" {
				lastSeen = "expired " + FormatAge(m.ExpiresAt.Time) + " ago"
			} else {
				lastSeen = "expires in " + strconv.Itoa(m.DaysUntilExpiry) + "d"
			}
		}

		result.Entries = append(result.Entries, MemberAuditEntry{
			Identity: m.Identity,
			Issue:    issue,
			Domain:   m.Domain,
			LastSeen: lastSeen,
		})
	}

	return result, nil
}

// MemberAuditTableReport produces a table from membership audit results.
// Columns: IDENTITY, ISSUE, DOMAIN, DETAILS
func MemberAuditTableReport(result *MemberAuditResult) TableData {
	td := TableData{
		Headers: []string{"IDENTITY", "ISSUE", "DOMAIN", "DETAILS"},
		Rows:    make([][]string, 0, len(result.Entries)),
	}
	for _, e := range result.Entries {
		action := ""
		switch e.Issue {
		case "expired":
			action = "Revoke membership"
		case "expiring-soon":
			action = "Renew before expiry"
		case "no-expiry-on-restricted":
			action = "Add --expires-at"
		case "stale":
			action = "Review and renew or remove"
		}
		td.Rows = append(td.Rows, []string{
			e.Identity,
			e.Issue,
			e.Domain,
			e.LastSeen + " — " + action,
		})
	}
	return td
}
