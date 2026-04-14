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
