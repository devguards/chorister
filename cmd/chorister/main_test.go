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

package main

import (
	"bytes"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 1A.19 — CLI argument parsing and safety rails
// ---------------------------------------------------------------------------

// executeCmd runs a root command tree with the given args and returns combined output + error.
func executeCmd(args ...string) (string, error) {
	root := newRootCmd()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestCLI_ApplyRefusesProductionNamespace(t *testing.T) {
	_, err := executeCmd("apply", "--domain", "payments", "--sandbox", "production")
	if err == nil {
		t.Fatal("expected apply to refuse production target, got nil error")
	}
	if !strings.Contains(err.Error(), "production") {
		t.Fatalf("error should mention 'production', got: %s", err.Error())
	}

	// Also test "prod"
	_, err = executeCmd("apply", "--domain", "payments", "--sandbox", "prod")
	if err == nil {
		t.Fatal("expected apply to refuse 'prod' target, got nil error")
	}
	if !strings.Contains(err.Error(), "production") {
		t.Fatalf("error should mention 'production', got: %s", err.Error())
	}
}

func TestCLI_ApplyRequiresSandboxFlag(t *testing.T) {
	_, err := executeCmd("apply", "--domain", "payments")
	if err == nil {
		t.Fatal("expected error when --sandbox is omitted")
	}
	if !strings.Contains(err.Error(), "--sandbox") {
		t.Fatalf("error should mention --sandbox, got: %s", err.Error())
	}
}

func TestCLI_SandboxCreateRequiresDomain(t *testing.T) {
	_, err := executeCmd("sandbox", "create", "--name", "alice")
	if err == nil {
		t.Fatal("expected error when --domain is omitted for sandbox create")
	}
	if !strings.Contains(err.Error(), "--domain") {
		t.Fatalf("error should mention --domain, got: %s", err.Error())
	}
}

func TestCLI_SandboxCreateBudgetExceeded(t *testing.T) {
	t.Skip("awaiting Phase 20: FinOps cost estimation and budget enforcement")

	// sandbox create rejected when estimated monthly cost would exceed domain budget
	_, err := executeCmd("sandbox", "create", "--domain", "payments", "--name", "expensive")
	if err == nil {
		t.Fatal("expected budget exceeded error")
	}
}

func TestCLI_DiffOutputFormat(t *testing.T) {
	t.Skip("awaiting Phase 3: diff command implementation")

	out, err := executeCmd("diff", "--domain", "payments", "--sandbox", "alice")
	if err != nil {
		t.Fatalf("diff should not error: %v", err)
	}

	// Diff output is human-readable: added/changed/removed
	for _, keyword := range []string{"Added", "Changed", "Removed"} {
		if !strings.Contains(out, keyword) {
			t.Errorf("diff output should contain %q, got:\n%s", keyword, out)
		}
	}
}

func TestCLI_DiffOutputIncludesCompilationRevision(t *testing.T) {
	t.Skip("awaiting Phase 21: compilation revision tracking")

	// diff surfaces controller revision drift when manifests change without DSL edits
	out, err := executeCmd("diff", "--domain", "payments", "--sandbox", "alice")
	if err != nil {
		t.Fatalf("diff should not error: %v", err)
	}
	if !strings.Contains(out, "compiledWithRevision") {
		t.Errorf("diff output should include compiledWithRevision, got:\n%s", out)
	}
}

func TestCLI_PromoteCreatesCRD(t *testing.T) {
	t.Skip("awaiting Phase 8: promote command → ChoPromotionRequest creation")

	// promote command creates ChoPromotionRequest CRD
	_, err := executeCmd("promote", "--domain", "payments", "--sandbox", "alice")
	if err != nil {
		t.Fatalf("promote should not error: %v", err)
	}
	// Assert ChoPromotionRequest was created (requires k8s client stub)
}

func TestCLI_ExportOutputsValidYAML(t *testing.T) {
	t.Skip("awaiting Phase 15.4: export command implementation")

	// export produces valid K8s manifests
	_, err := executeCmd("export", "--domain", "payments")
	if err != nil {
		t.Fatalf("export should not error: %v", err)
	}
}

func TestCLI_SetupIdempotent(t *testing.T) {
	// Dry-run mode validates the command structure without cluster interaction
	_, err1 := executeCmd("setup", "--dry-run")
	if err1 != nil {
		t.Fatalf("first setup --dry-run should not error: %v", err1)
	}
	_, err2 := executeCmd("setup", "--dry-run")
	if err2 != nil {
		t.Fatalf("second setup --dry-run should not error: %v", err2)
	}
}

func TestCLI_AdminMemberAudit_FlagsStale(t *testing.T) {
	t.Skip("awaiting membership audit implementation")

	// admin member audit reports stale memberships / expired access
	_, err := executeCmd("admin", "member", "audit")
	if err != nil {
		t.Fatalf("admin member audit should not error: %v", err)
	}
}

func TestCLI_AdminResourceDeleteArchived(t *testing.T) {
	t.Skip("awaiting Phase 18: archive lifecycle implementation")

	// admin resource delete --archived requires explicit target
	_, err := executeCmd("admin", "resource", "delete")
	if err == nil {
		t.Fatal("expected error when --archived is omitted")
	}
	if !strings.Contains(err.Error(), "--archived") {
		t.Fatalf("error should mention --archived, got: %s", err.Error())
	}
}

func TestCLI_AdminUpgradeBlueGreen(t *testing.T) {
	t.Skip("awaiting Phase 21: blue-green controller upgrade implementation")

	// admin upgrade manages revision install / promote / rollback flags safely
	_, err := executeCmd("admin", "upgrade")
	if err == nil {
		t.Fatal("expected error when no flag provided")
	}
	if !strings.Contains(err.Error(), "--revision") && !strings.Contains(err.Error(), "--promote") && !strings.Contains(err.Error(), "--rollback") {
		t.Fatalf("error should mention required flags, got: %s", err.Error())
	}
}

func TestCLI_ErrorMessages_Actionable(t *testing.T) {
	t.Skip("awaiting Phase 2+: actionable error message framework")

	// User-facing errors include blocked action, violated invariant, and next remediation step
	_, err := executeCmd("apply", "--domain", "payments")
	if err == nil {
		t.Fatal("expected error")
	}

	errMsg := err.Error()
	// Error should contain actionable hint
	if !strings.Contains(errMsg, "sandbox") {
		t.Errorf("error should contain remediation hint mentioning 'sandbox', got: %s", errMsg)
	}
}
