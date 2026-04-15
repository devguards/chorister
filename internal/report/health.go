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
	"fmt"
	"strconv"
	"strings"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/query"
)

// AppListReport produces a table for the app list command.
// Columns: NAME, DOMAINS, COMPLIANCE, PHASE, AGE
func AppListReport(apps []choristerv1alpha1.ChoApplication) TableData {
	td := TableData{
		Headers: []string{"NAME", "DOMAINS", "COMPLIANCE", "PHASE", "AGE"},
		Rows:    make([][]string, 0, len(apps)),
	}
	for _, app := range apps {
		td.Rows = append(td.Rows, []string{
			app.Name,
			strconv.Itoa(len(app.Spec.Domains)),
			app.Spec.Policy.Compliance,
			phase(app.Status.Phase),
			FormatAge(app.CreationTimestamp.Time),
		})
	}
	return td
}

// AppDetailReport produces a status summary for a single app.
func AppDetailReport(app *choristerv1alpha1.ChoApplication, domains []query.DomainInfo) StatusSummary {
	details := map[string]string{
		"Compliance":         app.Spec.Policy.Compliance,
		"HA Strategy":        haStrategy(app.Spec.Policy.HA),
		"Required Approvers": strconv.Itoa(app.Spec.Policy.Promotion.RequiredApprovers),
		"Allowed Roles":      strings.Join(app.Spec.Policy.Promotion.AllowedRoles, ", "),
		"Domain Count":       strconv.Itoa(len(domains)),
		"Owners":             strings.Join(app.Spec.Owners, ", "),
	}

	if app.Spec.Policy.Promotion.RequireSecurityScan {
		details["Security Scan Required"] = "true"
	}
	if app.Spec.Policy.Promotion.RequireTicketRef {
		details["Ticket Reference Required"] = "true"
	}

	return StatusSummary{
		Name:       app.Name,
		Phase:      phase(app.Status.Phase),
		Conditions: ConditionsFromMeta(app.Status.Conditions),
		Details:    details,
	}
}

// DomainListReport produces a table for the domain list command.
// Columns: DOMAIN, APPLICATION, NAMESPACE, SENSITIVITY, PHASE, RESOURCES, ISOLATED
func DomainListReport(domains []query.DomainInfo) TableData {
	td := TableData{
		Headers: []string{"DOMAIN", "APPLICATION", "NAMESPACE", "SENSITIVITY", "PHASE", "RESOURCES", "ISOLATED"},
		Rows:    make([][]string, 0, len(domains)),
	}
	for _, d := range domains {
		isolated := ""
		if d.Isolated {
			isolated = "⚠ yes"
		}
		td.Rows = append(td.Rows, []string{
			d.Name,
			d.Application,
			d.Namespace,
			d.Sensitivity,
			d.Phase,
			strconv.Itoa(d.ResourceCount),
			isolated,
		})
	}
	return td
}

// DomainDetailReport produces a status summary for a single domain.
func DomainDetailReport(domain query.DomainInfo, resources *query.DomainResources) StatusSummary {
	details := map[string]string{
		"Application": domain.Application,
		"Namespace":   domain.Namespace,
		"Sensitivity": domain.Sensitivity,
		"Isolated":    fmt.Sprintf("%v", domain.Isolated),
		"Computes":    strconv.Itoa(len(resources.Computes)),
		"Databases":   strconv.Itoa(len(resources.Databases)),
		"Queues":      strconv.Itoa(len(resources.Queues)),
		"Caches":      strconv.Itoa(len(resources.Caches)),
		"Storages":    strconv.Itoa(len(resources.Storages)),
		"Networks":    strconv.Itoa(len(resources.Networks)),
		"Total":       strconv.Itoa(resources.TotalCount()),
	}

	return StatusSummary{
		Name:    domain.Name,
		Phase:   domain.Phase,
		Details: details,
	}
}

// DomainResourcesTable produces a table listing all resources in a domain.
func DomainResourcesTable(resources *query.DomainResources) TableData {
	td := TableData{
		Headers: []string{"TYPE", "NAME", "STATUS"},
	}
	for _, c := range resources.Computes {
		status := "Not Ready"
		if c.Status.Ready {
			status = "Ready"
		}
		td.Rows = append(td.Rows, []string{"Compute", c.Name, status})
	}
	for _, d := range resources.Databases {
		status := "Not Ready"
		if d.Status.Ready {
			status = "Ready"
		}
		if d.Status.Lifecycle != "" {
			status = d.Status.Lifecycle
		}
		td.Rows = append(td.Rows, []string{"Database", d.Name, status})
	}
	for _, q := range resources.Queues {
		status := "Not Ready"
		if q.Status.Ready {
			status = "Ready"
		}
		td.Rows = append(td.Rows, []string{"Queue", q.Name, status})
	}
	for _, c := range resources.Caches {
		status := "Not Ready"
		if c.Status.Ready {
			status = "Ready"
		}
		td.Rows = append(td.Rows, []string{"Cache", c.Name, status})
	}
	for _, s := range resources.Storages {
		status := "Not Ready"
		if s.Status.Ready {
			status = "Ready"
		}
		if s.Status.Lifecycle != "" {
			status = s.Status.Lifecycle
		}
		td.Rows = append(td.Rows, []string{"Storage", s.Name, status})
	}
	for _, n := range resources.Networks {
		status := "Not Ready"
		if n.Status.Ready {
			status = "Ready"
		}
		td.Rows = append(td.Rows, []string{"Network", n.Name, status})
	}
	return td
}

func phase(p string) string {
	if p == "" {
		return "Pending"
	}
	return p
}

func haStrategy(ha *choristerv1alpha1.HAPolicy) string {
	if ha == nil {
		return "single-cluster"
	}
	return ha.Strategy
}

// ClusterStatusReport produces a status summary for the ChoCluster.
func ClusterStatusReport(cluster *choristerv1alpha1.ChoCluster) StatusSummary {
	details := map[string]string{
		"Controller Revision": cluster.Spec.ControllerRevision,
		"CIS Benchmark":       cluster.Status.CISBenchmark,
		"Observability Ready": fmt.Sprintf("%v", cluster.Status.ObservabilityReady),
	}

	if cluster.Spec.ControllerRevision == "" {
		details["Controller Revision"] = "(default)"
	}
	if cluster.Status.CISBenchmark == "" {
		details["CIS Benchmark"] = "(not run)"
	}

	if cluster.Status.OperatorStatus != nil {
		for name, status := range cluster.Status.OperatorStatus {
			details["Operator/"+name] = status
		}
	}

	return StatusSummary{
		Name:       cluster.Name,
		Phase:      phase(cluster.Status.Phase),
		Conditions: ConditionsFromMeta(cluster.Status.Conditions),
		Details:    details,
	}
}

// OperatorListReport produces a table for the operator list command.
// Columns: NAME, VERSION, STATUS
func OperatorListReport(operators []query.OperatorInfo) TableData {
	td := TableData{
		Headers: []string{"NAME", "VERSION", "STATUS"},
		Rows:    make([][]string, 0, len(operators)),
	}
	for _, op := range operators {
		version := op.Version
		if version == "" {
			version = "-"
		}
		td.Rows = append(td.Rows, []string{op.Name, version, op.Status})
	}
	return td
}

// SandboxListReport produces a table for the sandbox list command.
// Columns: NAME, OWNER, DOMAIN, AGE, LAST-APPLY, COST/MO, IDLE
func SandboxListReport(sandboxes []query.SandboxInfo) TableData {
	td := TableData{
		Headers: []string{"NAME", "OWNER", "DOMAIN", "AGE", "LAST-APPLY", "COST/MO", "IDLE"},
		Rows:    make([][]string, 0, len(sandboxes)),
	}
	for _, sb := range sandboxes {
		lastApply := "-"
		if sb.LastApplyTime != nil {
			lastApply = FormatAge(sb.LastApplyTime.Time)
		}
		idle := ""
		if sb.IdleWarning {
			idle = "⚠ idle"
		}
		td.Rows = append(td.Rows, []string{
			sb.Name,
			sb.Owner,
			sb.Domain,
			FormatAge(sb.Age),
			lastApply,
			FormatCost(sb.EstimatedMonthlyCost),
			idle,
		})
	}
	return td
}

// SandboxDetailReport produces a status summary for a single sandbox.
func SandboxDetailReport(detail *query.SandboxDetail) StatusSummary {
	details := map[string]string{
		"Owner":       detail.Owner,
		"Application": detail.Application,
		"Domain":      detail.Domain,
		"Namespace":   detail.Namespace,
		"Age":         FormatAge(detail.Age),
		"Cost/Month":  FormatCost(detail.EstimatedMonthlyCost),
	}

	if detail.LastApplyTime != nil {
		details["Last Apply"] = FormatAge(detail.LastApplyTime.Time) + " ago"
	}
	if detail.IdleWarning {
		details["Idle Warning"] = "⚠ sandbox has not been applied to in >7 days"
	}

	if detail.Resources != nil {
		details["Total Resources"] = strconv.Itoa(detail.Resources.TotalCount())
	}

	return StatusSummary{
		Name:       detail.Name,
		Phase:      detail.Phase,
		Conditions: ConditionsFromMeta(detail.Conditions),
		Details:    details,
	}
}

// DomainStatusReport produces a status summary for a domain including sandbox info.
func DomainStatusReport(domain query.DomainInfo, prodResources *query.DomainResources, sandboxes []query.SandboxInfo) StatusSummary {
	details := map[string]string{
		"Application":      domain.Application,
		"Namespace":        domain.Namespace,
		"Sensitivity":      domain.Sensitivity,
		"Isolated":         fmt.Sprintf("%v", domain.Isolated),
		"Prod Resources":   strconv.Itoa(prodResources.TotalCount()),
		"Active Sandboxes": strconv.Itoa(len(sandboxes)),
	}

	idle := 0
	for _, sb := range sandboxes {
		if sb.IdleWarning {
			idle++
		}
	}
	if idle > 0 {
		details["Idle Sandboxes"] = strconv.Itoa(idle)
	}

	return StatusSummary{
		Name:    domain.Name,
		Phase:   domain.Phase,
		Details: details,
	}
}

// EventListReport produces a table for the events command.
// Columns: TIME, TYPE, REASON, OBJECT, MESSAGE
func EventListReport(events []query.EventInfo) TableData {
	td := TableData{
		Headers: []string{"TIME", "TYPE", "REASON", "OBJECT", "MESSAGE"},
		Rows:    make([][]string, 0, len(events)),
	}
	for _, e := range events {
		td.Rows = append(td.Rows, []string{
			FormatAge(e.Time) + " ago",
			e.Type,
			e.Reason,
			e.InvolvedObject,
			e.Message,
		})
	}
	return td
}

// PromotionListReport produces a table for the promotion requests command.
// Columns: NAME, DOMAIN, REQUESTED-BY, STATUS, APPROVALS, AGE
func PromotionListReport(promotions []query.PromotionInfo) TableData {
	td := TableData{
		Headers: []string{"NAME", "DOMAIN", "REQUESTED-BY", "STATUS", "APPROVALS", "AGE"},
		Rows:    make([][]string, 0, len(promotions)),
	}
	for _, p := range promotions {
		td.Rows = append(td.Rows, []string{
			p.Name,
			p.Domain,
			p.RequestedBy,
			p.Phase,
			strconv.Itoa(p.ApprovalCount),
			FormatAge(p.CreatedAt),
		})
	}
	return td
}

// VulnSummaryReport produces a table for the vulnerability list command.
// Columns: DOMAIN, CRITICAL, HIGH, SCANNER, LAST-SCAN, STATUS
func VulnSummaryReport(reports []query.VulnReportInfo) TableData {
	td := TableData{
		Headers: []string{"DOMAIN", "CRITICAL", "HIGH", "SCANNER", "LAST-SCAN", "STATUS"},
		Rows:    make([][]string, 0, len(reports)),
	}
	for _, r := range reports {
		lastScan := "-"
		if r.ScannedAt != nil {
			lastScan = FormatAge(*r.ScannedAt) + " ago"
		}
		td.Rows = append(td.Rows, []string{
			r.Domain,
			strconv.Itoa(r.CriticalCount),
			strconv.Itoa(r.HighCount),
			r.Scanner,
			lastScan,
			r.Phase,
		})
	}
	return td
}

// VulnDetailReport produces a table for individual vulnerability findings.
// Columns: IMAGE, CVE, SEVERITY, PACKAGE, FIX-VERSION, TITLE
func VulnDetailReport(detail *query.VulnReportDetail) TableData {
	td := TableData{
		Headers: []string{"IMAGE", "CVE", "SEVERITY", "PACKAGE", "FIX-VERSION", "TITLE"},
		Rows:    make([][]string, 0, len(detail.Findings)),
	}
	for _, f := range detail.Findings {
		fixVersion := f.FixedVersion
		if fixVersion == "" {
			fixVersion = "(none)"
		}
		td.Rows = append(td.Rows, []string{
			f.Image,
			f.ID,
			f.Severity,
			f.Package,
			fixVersion,
			f.Title,
		})
	}
	return td
}

// MemberListReport produces a table for the member list command.
// Columns: IDENTITY, ROLE, DOMAIN, SOURCE, EXPIRES, STATUS
func MemberListReport(members []query.MemberInfo) TableData {
	td := TableData{
		Headers: []string{"IDENTITY", "ROLE", "DOMAIN", "SOURCE", "EXPIRES", "STATUS"},
		Rows:    make([][]string, 0, len(members)),
	}
	for _, m := range members {
		expires := "-"
		if m.ExpiresAt != nil {
			if m.DaysUntilExpiry < 0 {
				expires = "expired"
			} else if m.DaysUntilExpiry <= 30 {
				expires = fmt.Sprintf("%dd ⚠", m.DaysUntilExpiry)
			} else {
				expires = fmt.Sprintf("%dd", m.DaysUntilExpiry)
			}
		}
		td.Rows = append(td.Rows, []string{
			m.Identity,
			m.Role,
			m.Domain,
			m.Source,
			expires,
			m.Phase,
		})
	}
	return td
}

// ResourceListReport produces a table listing all resources in a domain.
// Columns: TYPE, NAME, STATUS, LIFECYCLE, SIZE, AGE
func ResourceListReport(resources *query.DomainResources, archivedOnly bool) TableData {
	td := TableData{
		Headers: []string{"TYPE", "NAME", "STATUS", "LIFECYCLE", "SIZE", "AGE"},
	}

	addRow := func(rtype, name, status, lifecycle, size string) {
		if archivedOnly && lifecycle != "Archived" && lifecycle != "Deletable" {
			return
		}
		marker := ""
		if lifecycle == "Archived" || lifecycle == "Deletable" {
			marker = " ⚠"
		}
		td.Rows = append(td.Rows, []string{rtype, name, status, lifecycle + marker, size, "-"})
	}

	for _, c := range resources.Computes {
		status := "Not Ready"
		if c.Status.Ready {
			status = "Ready"
		}
		addRow("Compute", c.Name, status, "Active", c.Spec.Image)
	}
	for _, d := range resources.Databases {
		status := "Not Ready"
		if d.Status.Ready {
			status = "Ready"
		}
		lifecycle := d.Status.Lifecycle
		if lifecycle == "" {
			lifecycle = "Active"
		}
		addRow("Database", d.Name, status, lifecycle, string(d.Spec.Size))
	}
	for _, q := range resources.Queues {
		status := "Not Ready"
		if q.Status.Ready {
			status = "Ready"
		}
		lifecycle := q.Status.Lifecycle
		if lifecycle == "" {
			lifecycle = "Active"
		}
		addRow("Queue", q.Name, status, lifecycle, string(q.Spec.Size))
	}
	for _, c := range resources.Caches {
		status := "Not Ready"
		if c.Status.Ready {
			status = "Ready"
		}
		addRow("Cache", c.Name, status, "Active", string(c.Spec.Size))
	}
	for _, s := range resources.Storages {
		status := "Not Ready"
		if s.Status.Ready {
			status = "Ready"
		}
		lifecycle := s.Status.Lifecycle
		if lifecycle == "" {
			lifecycle = "Active"
		}
		size := "-"
		if s.Spec.Size != nil {
			size = s.Spec.Size.String()
		}
		addRow("Storage", s.Name, status, lifecycle, size)
	}
	for _, n := range resources.Networks {
		status := "Not Ready"
		if n.Status.Ready {
			status = "Ready"
		}
		addRow("Network", n.Name, status, "Active", "-")
	}

	return td
}
