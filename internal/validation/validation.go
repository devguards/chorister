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
	"strings"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
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
