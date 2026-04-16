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
func buildResource(name, kind, namespace string, fields map[string]any) *unstructured.Unstructured {
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
	sandbox := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp-payments-sandbox-alice", map[string]any{
			"replicas": int64(3),
		}),
	}
	production := []*unstructured.Unstructured{}

	result := Compare(sandbox, production)
	if len(result.Diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(result.Diffs))
	}
	if result.Diffs[0].Action != ActionAdded {
		t.Errorf("expected action Added, got %s", result.Diffs[0].Action)
	}
	if result.Diffs[0].Kind != "Deployment" {
		t.Errorf("expected kind Deployment, got %s", result.Diffs[0].Kind)
	}
	if result.Diffs[0].Name != "api" {
		t.Errorf("expected name api, got %s", result.Diffs[0].Name)
	}
}

func TestDiff_Removed(t *testing.T) {
	sandbox := []*unstructured.Unstructured{}
	production := []*unstructured.Unstructured{
		buildResource("legacy-worker", "Deployment", "myapp-payments", map[string]any{
			"replicas": int64(1),
		}),
	}

	result := Compare(sandbox, production)
	if len(result.Diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(result.Diffs))
	}
	if result.Diffs[0].Action != ActionRemoved {
		t.Errorf("expected action Removed, got %s", result.Diffs[0].Action)
	}
	if result.Diffs[0].Name != "legacy-worker" {
		t.Errorf("expected name legacy-worker, got %s", result.Diffs[0].Name)
	}
}

func TestDiff_Changed(t *testing.T) {
	sandbox := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp-payments-sandbox-alice", map[string]any{
			"replicas": int64(5),
		}),
	}
	production := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp-payments", map[string]any{
			"replicas": int64(3),
		}),
	}

	result := Compare(sandbox, production)
	if len(result.Diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(result.Diffs))
	}
	if result.Diffs[0].Action != ActionChanged {
		t.Errorf("expected action Changed, got %s", result.Diffs[0].Action)
	}
	if len(result.Diffs[0].Fields) == 0 {
		t.Error("expected changed fields to be non-empty")
	}
	// Should contain spec.replicas
	found := false
	for _, f := range result.Diffs[0].Fields {
		if f == "spec.replicas" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected field spec.replicas in changed fields, got %v", result.Diffs[0].Fields)
	}
}

func TestDiff_NoDifferences(t *testing.T) {
	sandbox := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp-payments-sandbox-alice", map[string]any{
			"replicas": int64(3),
		}),
	}
	production := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp-payments", map[string]any{
			"replicas": int64(3),
		}),
	}

	result := Compare(sandbox, production)
	if len(result.Diffs) != 0 {
		t.Fatalf("expected 0 diffs, got %d: %+v", len(result.Diffs), result.Diffs)
	}
}

func TestDiff_RenameShowsRemoveAndAdd(t *testing.T) {
	sandbox := []*unstructured.Unstructured{
		buildResource("api-v2", "Deployment", "myapp-payments-sandbox-alice", map[string]any{
			"replicas": int64(3),
		}),
	}
	production := []*unstructured.Unstructured{
		buildResource("api-v1", "Deployment", "myapp-payments", map[string]any{
			"replicas": int64(3),
		}),
	}

	result := Compare(sandbox, production)
	if len(result.Diffs) != 2 {
		t.Fatalf("expected 2 diffs (remove+add), got %d: %+v", len(result.Diffs), result.Diffs)
	}
	hasAdded := false
	hasRemoved := false
	for _, d := range result.Diffs {
		if d.Action == ActionAdded && d.Name == "api-v2" {
			hasAdded = true
		}
		if d.Action == ActionRemoved && d.Name == "api-v1" {
			hasRemoved = true
		}
	}
	if !hasAdded {
		t.Error("expected api-v2 to show as Added")
	}
	if !hasRemoved {
		t.Error("expected api-v1 to show as Removed")
	}
}

func TestDiff_CompilationRevisionChange(t *testing.T) {
	sandbox := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp-payments-sandbox-alice", map[string]any{
			"replicas": int64(3),
		}),
	}
	production := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp-payments", map[string]any{
			"replicas": int64(3),
		}),
	}

	sandboxRevision := "v2.0.0"
	productionRevision := "v1.0.0"

	result := CompareWithRevisions(sandbox, production, sandboxRevision, productionRevision)
	if result.SandboxRevision != sandboxRevision {
		t.Errorf("expected sandbox revision %s, got %s", sandboxRevision, result.SandboxRevision)
	}
	if result.ProductionRevision != productionRevision {
		t.Errorf("expected production revision %s, got %s", productionRevision, result.ProductionRevision)
	}
}

func TestDiff_CompilationRevisionSame(t *testing.T) {
	sandbox := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp-sandbox", map[string]any{"replicas": int64(1)}),
	}
	production := []*unstructured.Unstructured{
		buildResource("api", "Deployment", "myapp", map[string]any{"replicas": int64(1)}),
	}

	result := CompareWithRevisions(sandbox, production, "v1.0.0", "v1.0.0")
	if result.SandboxRevision != "v1.0.0" {
		t.Errorf("expected sandbox revision v1.0.0, got %s", result.SandboxRevision)
	}
	if result.ProductionRevision != "v1.0.0" {
		t.Errorf("expected production revision v1.0.0, got %s", result.ProductionRevision)
	}
}

func TestDiff_FormatWithRevisionMismatch(t *testing.T) {
	result := Result{
		Diffs: []ResourceDiff{
			{Kind: "Deployment", Name: "api", Action: ActionChanged, Fields: []string{"spec.replicas"}},
		},
		SandboxRevision:    "v2.0.0",
		ProductionRevision: "v1.0.0",
	}

	output := Format(result)
	if !contains(output, "compiledWithRevision differs") {
		t.Errorf("expected revision mismatch warning, got:\n%s", output)
	}
	if !contains(output, "sandbox=v2.0.0") {
		t.Errorf("expected sandbox revision in output, got:\n%s", output)
	}
	if !contains(output, "production=v1.0.0") {
		t.Errorf("expected production revision in output, got:\n%s", output)
	}
}

func TestDiff_FormatWithSameRevision(t *testing.T) {
	result := Result{
		Diffs: []ResourceDiff{
			{Kind: "Deployment", Name: "api", Action: ActionAdded},
		},
		SandboxRevision:    "v1.0.0",
		ProductionRevision: "v1.0.0",
	}

	output := Format(result)
	if contains(output, "compiledWithRevision") {
		t.Errorf("should not show revision warning when revisions match, got:\n%s", output)
	}
}

func TestDiff_FormatOutput(t *testing.T) {
	result := Result{
		Diffs: []ResourceDiff{
			{Kind: "Deployment", Name: "api", Action: ActionAdded},
			{Kind: "Deployment", Name: "worker", Action: ActionChanged, Fields: []string{"spec.replicas"}},
			{Kind: "Service", Name: "legacy", Action: ActionRemoved},
		},
	}

	output := Format(result)
	if output == "" {
		t.Fatal("expected non-empty formatted output")
	}

	for _, want := range []string{"Added", "Changed", "Removed"} {
		if !contains(output, want) {
			t.Errorf("formatted output should contain %q, got:\n%s", want, output)
		}
	}
}

func TestDiff_FormatEmpty(t *testing.T) {
	result := Result{}
	output := Format(result)
	if !contains(output, "No differences") {
		t.Errorf("expected 'No differences' message, got: %s", output)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsHelper(s, substr)
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
