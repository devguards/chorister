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

// Package validation enforces chorister policy invariants at compile time.
package validation

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/compiler"
)

// ValidateConsumesSupplies checks that every consumes reference has a matching supplies declaration.
func ValidateConsumesSupplies(app *choristerv1alpha1.ChoApplication) []string {
	domainMap := make(map[string]*choristerv1alpha1.DomainSpec, len(app.Spec.Domains))
	for i := range app.Spec.Domains {
		domainMap[app.Spec.Domains[i].Name] = &app.Spec.Domains[i]
	}

	var errs []string
	for _, domain := range app.Spec.Domains {
		for _, consume := range domain.Consumes {
			supplier, exists := domainMap[consume.Domain]
			if !exists {
				errs = append(errs, fmt.Sprintf("domain %q consumes %q but domain %q does not exist", domain.Name, consume.Domain, consume.Domain))
				continue
			}
			if supplier.Supplies == nil {
				errs = append(errs, fmt.Sprintf("domain %q consumes %q but %q does not declare supplies", domain.Name, consume.Domain, consume.Domain))
				continue
			}
			if supplier.Supplies.Port != consume.Port {
				errs = append(errs, fmt.Sprintf("domain %q consumes %q on port %d but %q supplies on port %d", domain.Name, consume.Domain, consume.Port, consume.Domain, supplier.Supplies.Port))
			}
		}
	}
	return errs
}

// ValidateCycleDetection checks for circular dependencies in the consumes graph.
// Returns an error with the cycle path if a cycle is detected.
func ValidateCycleDetection(app *choristerv1alpha1.ChoApplication) error {
	// Build adjacency list
	graph := make(map[string][]string)
	for _, domain := range app.Spec.Domains {
		for _, consume := range domain.Consumes {
			graph[domain.Name] = append(graph[domain.Name], consume.Domain)
		}
	}

	// DFS-based cycle detection
	const (
		white = 0 // unvisited
		gray  = 1 // in current path
		black = 2 // fully explored
	)

	color := make(map[string]int)
	parent := make(map[string]string)

	var dfs func(node string) (string, bool)
	dfs = func(node string) (string, bool) {
		color[node] = gray
		for _, neighbor := range graph[node] {
			if color[neighbor] == gray {
				// Found a cycle — reconstruct path
				cycle := []string{neighbor, node}
				cur := node
				for cur != neighbor {
					cur = parent[cur]
					cycle = append(cycle, cur)
				}
				// Reverse to get proper order
				for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
					cycle[i], cycle[j] = cycle[j], cycle[i]
				}
				return strings.Join(cycle, " → "), true
			}
			if color[neighbor] == white {
				parent[neighbor] = node
				if path, found := dfs(neighbor); found {
					return path, true
				}
			}
		}
		color[node] = black
		return "", false
	}

	for _, domain := range app.Spec.Domains {
		if color[domain.Name] == white {
			if path, found := dfs(domain.Name); found {
				return fmt.Errorf("dependency cycle detected: %s", path)
			}
		}
	}
	return nil
}

// ValidateIngressAuth checks that internet-facing ingress has an auth block.
func ValidateIngressAuth(network *choristerv1alpha1.ChoNetwork) []string {
	if network.Spec.Ingress == nil {
		return nil
	}
	if network.Spec.Ingress.From != "internet" {
		return nil
	}
	if network.Spec.Ingress.Auth == nil || network.Spec.Ingress.Auth.JWT == nil {
		return []string{fmt.Sprintf(
			"ChoNetwork %q: internet ingress on port %d requires an auth block with JWT configuration",
			network.Name, network.Spec.Ingress.Port,
		)}
	}
	return nil
}

// ValidateIngressAllowedIdP checks that the ingress JWT issuer is in the application's allowed IdP list.
func ValidateIngressAllowedIdP(network *choristerv1alpha1.ChoNetwork, appPolicy choristerv1alpha1.ApplicationPolicy) []string {
	if network.Spec.Ingress == nil || network.Spec.Ingress.Auth == nil || network.Spec.Ingress.Auth.JWT == nil {
		return nil
	}
	if appPolicy.Network == nil || appPolicy.Network.Ingress == nil || len(appPolicy.Network.Ingress.AllowedIdPs) == 0 {
		return nil // no IdP restrictions
	}

	issuer := network.Spec.Ingress.Auth.JWT.Issuer
	for _, idp := range appPolicy.Network.Ingress.AllowedIdPs {
		if idp.Issuer == issuer {
			return nil
		}
	}

	var allowed []string
	for _, idp := range appPolicy.Network.Ingress.AllowedIdPs {
		allowed = append(allowed, idp.Issuer)
	}
	return []string{fmt.Sprintf(
		"ChoNetwork %q: JWT issuer %q is not in the application's allowed IdP list. Allowed: %s",
		network.Name, issuer, strings.Join(allowed, ", "),
	)}
}

// ValidateEgressWildcard checks that egress allowlist does not contain wildcards.
func ValidateEgressWildcard(network *choristerv1alpha1.ChoNetwork) []string {
	if network.Spec.Egress == nil {
		return nil
	}
	for _, dest := range network.Spec.Egress.Allowlist {
		if dest == "*" {
			return []string{fmt.Sprintf(
				"ChoNetwork %q: wildcard egress (*) is not permitted. Declare specific destinations.",
				network.Name,
			)}
		}
	}
	return nil
}

// ValidateEgressAllowedDestinations checks that network egress destinations are approved by the application policy.
func ValidateEgressAllowedDestinations(network *choristerv1alpha1.ChoNetwork, appPolicy choristerv1alpha1.ApplicationPolicy) []string {
	if network.Spec.Egress == nil || len(network.Spec.Egress.Allowlist) == 0 {
		return nil
	}
	if appPolicy.Network == nil || appPolicy.Network.Egress == nil || len(appPolicy.Network.Egress.Allowlist) == 0 {
		return nil
	}

	approved := make(map[string]struct{}, len(appPolicy.Network.Egress.Allowlist))
	allowedHosts := make([]string, 0, len(appPolicy.Network.Egress.Allowlist))
	for _, target := range appPolicy.Network.Egress.Allowlist {
		approved[target.Host] = struct{}{}
		allowedHosts = append(allowedHosts, target.Host)
	}

	var errs []string
	for _, destination := range network.Spec.Egress.Allowlist {
		if _, ok := approved[destination]; ok {
			continue
		}
		errs = append(errs, fmt.Sprintf(
			"ChoNetwork %q: egress destination %q is not in the application's approved allowlist. Allowed: %s",
			network.Name,
			destination,
			strings.Join(allowedHosts, ", "),
		))
	}

	return errs
}

// ValidateComplianceEscalation checks domain sensitivity doesn't weaken app compliance.
// Compliance levels: essential < standard < regulated
// Sensitivity levels: public < internal < confidential < restricted
// A regulated app cannot have a public domain.
func ValidateComplianceEscalation(app *choristerv1alpha1.ChoApplication) []string {
	complianceLevel := complianceToLevel(app.Spec.Policy.Compliance)
	var errs []string

	for _, domain := range app.Spec.Domains {
		sensitivityLevel := sensitivityToLevel(domain.Sensitivity)
		if sensitivityLevel < complianceLevel {
			errs = append(errs, fmt.Sprintf(
				"domain %q sensitivity %q is weaker than application compliance %q",
				domain.Name, domain.Sensitivity, app.Spec.Policy.Compliance,
			))
		}
	}
	return errs
}

// ValidateRestrictedMembershipExpiry checks that restricted domain memberships have expiresAt set.
func ValidateRestrictedMembershipExpiry(membership *choristerv1alpha1.ChoDomainMembership, domainSensitivity string) []string {
	if domainSensitivity == "restricted" && membership.Spec.ExpiresAt == nil {
		return []string{fmt.Sprintf(
			"membership %q for restricted domain requires expiresAt to be set",
			membership.Name,
		)}
	}
	return nil
}

func complianceToLevel(c string) int {
	switch c {
	case "essential":
		return 1
	case "standard":
		return 2
	case "regulated":
		return 3
	default:
		return 0
	}
}

func sensitivityToLevel(s string) int {
	switch s {
	case "public":
		return 1
	case "internal":
		return 2
	case "confidential":
		return 3
	case "restricted":
		return 4
	default:
		return 2 // default to internal
	}
}

// ValidateArchiveRetentionMinimum checks that archive retention is at least 30 days.
func ValidateArchiveRetentionMinimum(app *choristerv1alpha1.ChoApplication) []string {
	if app.Spec.Policy.ArchiveRetention == "" {
		return nil // default 30d is acceptable
	}
	duration, err := ParseRetentionDuration(app.Spec.Policy.ArchiveRetention)
	if err != nil {
		return []string{fmt.Sprintf(
			"invalid archiveRetention format %q: %v",
			app.Spec.Policy.ArchiveRetention, err,
		)}
	}
	minRetention := 30 * 24 * time.Hour
	if duration < minRetention {
		return []string{fmt.Sprintf(
			"archiveRetention %q is below minimum 30 days; set to at least \"30d\"",
			app.Spec.Policy.ArchiveRetention,
		)}
	}
	return nil
}

// ValidateArchivedResourceDependencies checks that no active compute resources
// share a domain with archived stateful resources.
func ValidateArchivedResourceDependencies(
	databases []choristerv1alpha1.ChoDatabase,
	queues []choristerv1alpha1.ChoQueue,
	storages []choristerv1alpha1.ChoStorage,
) []string {
	var errs []string
	for i := range databases {
		if databases[i].Status.Lifecycle == "Archived" {
			errs = append(errs, fmt.Sprintf(
				"ChoDatabase %q in namespace %q is archived; remove references from dependent resources before promoting",
				databases[i].Name, databases[i].Namespace,
			))
		}
	}
	for i := range queues {
		if queues[i].Status.Lifecycle == "Archived" {
			errs = append(errs, fmt.Sprintf(
				"ChoQueue %q in namespace %q is archived; remove references from dependent resources before promoting",
				queues[i].Name, queues[i].Namespace,
			))
		}
	}
	for i := range storages {
		if storages[i].Status.Lifecycle == "Archived" {
			errs = append(errs, fmt.Sprintf(
				"ChoStorage %q in namespace %q is archived; remove references from dependent resources before promoting",
				storages[i].Name, storages[i].Namespace,
			))
		}
	}
	return errs
}

// ParseRetentionDuration parses a retention duration string (e.g. "30d", "1y", "90d").
func ParseRetentionDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("invalid days value: %w", err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	if strings.HasSuffix(s, "y") {
		years, err := strconv.Atoi(strings.TrimSuffix(s, "y"))
		if err != nil {
			return 0, fmt.Errorf("invalid years value: %w", err)
		}
		return time.Duration(years) * 365 * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// ValidateSizingTemplate checks that a size reference resolves to a defined template.
// resourceType is the template category (e.g. "database", "cache", "queue", "compute").
func ValidateSizingTemplate(sizeName, resourceType string, templates map[string]choristerv1alpha1.SizingTemplateSet) []string {
	if sizeName == "" {
		return nil
	}
	_, err := compiler.ResolveSizingTemplate(sizeName, resourceType, templates)
	if err != nil {
		return []string{err.Error()}
	}
	return nil
}

// ValidateResourcesVsQuota checks that resource requests do not exceed the domain quota.
// resources is the effective resource requirements (either explicit or resolved from template).
func ValidateResourcesVsQuota(resources *corev1.ResourceRequirements, quota *choristerv1alpha1.DomainQuota, resourceName string) []string {
	if resources == nil || quota == nil {
		return nil
	}

	var errs []string
	requests := resources.Requests

	if cpuReq, ok := requests[corev1.ResourceCPU]; ok {
		if !quota.CPU.IsZero() && cpuReq.Cmp(quota.CPU) > 0 {
			errs = append(errs, fmt.Sprintf(
				"%s: CPU request %s exceeds domain quota %s",
				resourceName, cpuReq.String(), quota.CPU.String(),
			))
		}
	}

	if memReq, ok := requests[corev1.ResourceMemory]; ok {
		if !quota.Memory.IsZero() && memReq.Cmp(quota.Memory) > 0 {
			errs = append(errs, fmt.Sprintf(
				"%s: memory request %s exceeds domain quota %s",
				resourceName, memReq.String(), quota.Memory.String(),
			))
		}
	}

	if storageReq, ok := requests[corev1.ResourceStorage]; ok {
		if !quota.Storage.IsZero() && storageReq.Cmp(quota.Storage) > 0 {
			errs = append(errs, fmt.Sprintf(
				"%s: storage request %s exceeds domain quota %s",
				resourceName, storageReq.String(), quota.Storage.String(),
			))
		}
	}

	return errs
}

// ValidateExplicitResourcesVsQuota validates explicit resource overrides against the domain quota.
// This provides a clear error message that includes quota details.
func ValidateExplicitResourcesVsQuota(
	explicitResources *corev1.ResourceRequirements,
	quota *choristerv1alpha1.DomainQuota,
	resourceName string,
) []string {
	if explicitResources == nil || quota == nil {
		return nil
	}

	errs := ValidateResourcesVsQuota(explicitResources, quota, resourceName)
	if len(errs) > 0 {
		// Enhance error messages with quota info
		quotaInfo := fmt.Sprintf("domain quota: CPU=%s, memory=%s, storage=%s",
			formatQuantityOrUnset(quota.CPU),
			formatQuantityOrUnset(quota.Memory),
			formatQuantityOrUnset(quota.Storage),
		)
		errs = append(errs, quotaInfo)
	}
	return errs
}

func formatQuantityOrUnset(q resource.Quantity) string {
	if q.IsZero() {
		return "unset"
	}
	return q.String()
}
