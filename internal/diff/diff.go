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

// Package diff compares compiled manifests between sandbox and production.
package diff

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Action describes what happened to a resource.
type Action string

const (
	ActionAdded   Action = "Added"
	ActionRemoved Action = "Removed"
	ActionChanged Action = "Changed"
)

// ResourceDiff represents a difference for a single resource.
type ResourceDiff struct {
	Kind   string
	Name   string
	Action Action
	// Fields lists the changed field paths (only for ActionChanged).
	Fields []string
}

// Result holds the full diff output and optional compilation revision info.
type Result struct {
	Diffs              []ResourceDiff
	SandboxRevision    string
	ProductionRevision string
}

// resourceKey returns a unique key for matching resources across namespaces.
func resourceKey(obj *unstructured.Unstructured) string {
	return fmt.Sprintf("%s/%s", obj.GetKind(), obj.GetName())
}

// Compare compares sandbox and production resource lists.
// Resources are matched by Kind+Name (namespaces are expected to differ).
func Compare(sandbox, production []*unstructured.Unstructured) Result {
	sandboxMap := make(map[string]*unstructured.Unstructured, len(sandbox))
	for _, obj := range sandbox {
		sandboxMap[resourceKey(obj)] = obj
	}

	prodMap := make(map[string]*unstructured.Unstructured, len(production))
	for _, obj := range production {
		prodMap[resourceKey(obj)] = obj
	}

	var diffs []ResourceDiff

	// Check for added or changed resources
	for key, sObj := range sandboxMap {
		pObj, exists := prodMap[key]
		if !exists {
			diffs = append(diffs, ResourceDiff{
				Kind:   sObj.GetKind(),
				Name:   sObj.GetName(),
				Action: ActionAdded,
			})
			continue
		}

		// Compare spec fields (ignoring namespace, resourceVersion, etc.)
		changedFields := compareSpecs(sObj, pObj)
		if len(changedFields) > 0 {
			diffs = append(diffs, ResourceDiff{
				Kind:   sObj.GetKind(),
				Name:   sObj.GetName(),
				Action: ActionChanged,
				Fields: changedFields,
			})
		}
	}

	// Check for removed resources
	for key, pObj := range prodMap {
		if _, exists := sandboxMap[key]; !exists {
			diffs = append(diffs, ResourceDiff{
				Kind:   pObj.GetKind(),
				Name:   pObj.GetName(),
				Action: ActionRemoved,
			})
		}
	}

	// Sort for deterministic output
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].Action != diffs[j].Action {
			return diffs[i].Action < diffs[j].Action
		}
		if diffs[i].Kind != diffs[j].Kind {
			return diffs[i].Kind < diffs[j].Kind
		}
		return diffs[i].Name < diffs[j].Name
	})

	return Result{Diffs: diffs}
}

// CompareWithRevisions performs a diff and attaches revision metadata.
func CompareWithRevisions(sandbox, production []*unstructured.Unstructured, sandboxRev, prodRev string) Result {
	result := Compare(sandbox, production)
	result.SandboxRevision = sandboxRev
	result.ProductionRevision = prodRev
	return result
}

// compareSpecs compares the spec portion of two unstructured objects.
func compareSpecs(a, b *unstructured.Unstructured) []string {
	aSpec, aFound, _ := unstructured.NestedMap(a.Object, "spec")
	bSpec, bFound, _ := unstructured.NestedMap(b.Object, "spec")

	if !aFound && !bFound {
		return nil
	}
	if !aFound || !bFound {
		return []string{"spec"}
	}

	var changed []string
	collectDiffs("spec", aSpec, bSpec, &changed)
	return changed
}

// collectDiffs recursively finds field-level differences between two maps.
func collectDiffs(prefix string, a, b map[string]any, changed *[]string) {
	allKeys := make(map[string]bool)
	for k := range a {
		allKeys[k] = true
	}
	for k := range b {
		allKeys[k] = true
	}

	for k := range allKeys {
		path := prefix + "." + k
		aVal, aOK := a[k]
		bVal, bOK := b[k]

		if !aOK || !bOK {
			*changed = append(*changed, path)
			continue
		}

		aMap, aIsMap := aVal.(map[string]any)
		bMap, bIsMap := bVal.(map[string]any)
		if aIsMap && bIsMap {
			collectDiffs(path, aMap, bMap, changed)
			continue
		}

		if !reflect.DeepEqual(aVal, bVal) {
			*changed = append(*changed, path)
		}
	}
}

// Format returns a human-readable string of the diff result.
func Format(result Result) string {
	if len(result.Diffs) == 0 {
		return "No differences found."
	}

	var b strings.Builder
	for _, d := range result.Diffs {
		switch d.Action {
		case ActionAdded:
			fmt.Fprintf(&b, "  Added:   %s/%s\n", d.Kind, d.Name)
		case ActionRemoved:
			fmt.Fprintf(&b, "  Removed: %s/%s\n", d.Kind, d.Name)
		case ActionChanged:
			fmt.Fprintf(&b, "  Changed: %s/%s (%s)\n", d.Kind, d.Name, strings.Join(d.Fields, ", "))
		}
	}

	if result.SandboxRevision != "" || result.ProductionRevision != "" {
		if result.SandboxRevision != result.ProductionRevision {
			fmt.Fprintf(&b, "\n  Compilation revision: sandbox=%s, production=%s (compiledWithRevision differs)\n",
				result.SandboxRevision, result.ProductionRevision)
		}
	}

	return b.String()
}
