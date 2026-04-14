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

package diff

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ---------------------------------------------------------------------------
// 1A.3 — Diff engine unit tests
// ---------------------------------------------------------------------------

// buildResource creates a minimal unstructured resource for diff testing.
func buildResource(name, kind, namespace string, fields map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetKind(kind)
	obj.SetNamespace(namespace)
	obj.SetAPIVersion("apps/v1")
	if fields != nil {
		for k, v := range fields {
			_ = unstructured.SetNestedField(obj.Object, v, "spec", k)
		}
	}
	return obj
}

func TestDiff_Added(t *testing.T) {
	t.Skip("awaiting Phase 8.1: Diff engine — sandbox vs production")

	// Resource in sandbox but not production → shows "added"
	sandbox := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp-payments-sandbox-alice", map[string]interface{}{
			"replicas": int64(3),
		}),
	}
	production := []*unstructured.Unstructured{} // empty

	_, _ = sandbox, production
	// TODO: Call diff engine
	// Assert result contains one entry: kind=Deployment, name=api, action=added
}

func TestDiff_Removed(t *testing.T) {
	t.Skip("awaiting Phase 8.1: Diff engine — sandbox vs production")

	// Resource in production but not sandbox → shows "removed"
	sandbox := []*unstructured.Unstructured{}
	production := []*unstructured.Unstructured{
		buildResource("legacy-worker", "Deployment", "myapp-payments", map[string]interface{}{
			"replicas": int64(1),
		}),
	}

	_, _ = sandbox, production
	// TODO: Call diff engine
	// Assert result contains one entry: kind=Deployment, name=legacy-worker, action=removed
}

func TestDiff_Changed(t *testing.T) {
	t.Skip("awaiting Phase 8.1: Diff engine — sandbox vs production")

	// Same resource, field differs → shows field-level diff
	sandbox := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp-payments-sandbox-alice", map[string]interface{}{
			"replicas": int64(5),
		}),
	}
	production := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp-payments", map[string]interface{}{
			"replicas": int64(3),
		}),
	}

	_, _ = sandbox, production
	// TODO: Call diff engine
	// Assert result contains one entry: kind=Deployment, name=api, action=changed, field=spec.replicas
}

func TestDiff_NoDifferences(t *testing.T) {
	t.Skip("awaiting Phase 8.1: Diff engine — sandbox vs production")

	// Identical resources → empty result
	sandbox := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp-payments-sandbox-alice", map[string]interface{}{
			"replicas": int64(3),
		}),
	}
	production := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp-payments", map[string]interface{}{
			"replicas": int64(3),
		}),
	}

	_, _ = sandbox, production
	// TODO: Call diff engine, assert empty result
}

func TestDiff_RenameShowsRemoveAndAdd(t *testing.T) {
	t.Skip("awaiting Phase 8.1: Diff engine — sandbox vs production")

	// Resource renamed: old name in prod, new name in sandbox → remove + add
	sandbox := []*unstructured.Unstructured{
		buildResource("api-v2", "Deployment", "myapp-payments-sandbox-alice", map[string]interface{}{
			"replicas": int64(3),
		}),
	}
	production := []*unstructured.Unstructured{
		buildResource("api-v1", "Deployment", "myapp-payments", map[string]interface{}{
			"replicas": int64(3),
		}),
	}

	_, _ = sandbox, production
	// TODO: Call diff engine
	// Assert result contains two entries: api-v1 removed, api-v2 added
}

func TestDiff_CompilationRevisionChange(t *testing.T) {
	t.Skip("awaiting Phase 19.3: Compilation stability tracking")

	// Same DSL, different controller revision → surfaces compilation diff
	// This tests that the diff engine can detect when compiled output changes
	// due to a controller upgrade even when the user's DSL has not changed.
	sandbox := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp-payments-sandbox-alice", map[string]interface{}{
			"replicas": int64(3),
		}),
	}
	production := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp-payments", map[string]interface{}{
			"replicas": int64(3),
		}),
	}

	sandboxRevision := "v2.0.0"
	productionRevision := "v1.0.0"

	_, _, _, _ = sandbox, production, sandboxRevision, productionRevision
	// TODO: Call diff engine with revision info
	// Assert that compilation revision difference is surfaced
}
