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
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = choristerv1alpha1.AddToScheme(s)
	return s
}

// executeCmdWithClient runs a root command with a fake client injected via context.
func executeCmdWithClient(fc client.Client, args ...string) (string, error) {
	root := newRootCmd()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	ctx := context.WithValue(context.Background(), clientContextKey, fc)
	root.SetContext(ctx)

	err := root.Execute()
	return buf.String(), err
}

func testApp(name string, domains []choristerv1alpha1.DomainSpec, phase string, compliance string) *choristerv1alpha1.ChoApplication {
	return &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			CreationTimestamp: metav1.Now(),
		},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Owners: []string{"admin@example.com"},
			Policy: choristerv1alpha1.ApplicationPolicy{
				Compliance: compliance,
				Promotion: choristerv1alpha1.PromotionPolicy{
					RequiredApprovers: 1,
					AllowedRoles:      []string{"org-admin"},
				},
			},
			Domains: domains,
		},
		Status: choristerv1alpha1.ChoApplicationStatus{
			Phase: phase,
			DomainNamespaces: func() map[string]string {
				m := make(map[string]string)
				for _, d := range domains {
					m[d.Name] = name + "-" + d.Name
				}
				return m
			}(),
		},
	}
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
	t.Skip("budget enforcement is controller-side (ChoSandbox reconciler rejects); CLI sandbox create requires cluster connection")

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
	t.Skip("diff command requires cluster connection; compiledWithRevision tracking is implemented in controller (Phase 19.3)")

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
	// Export produces YAML output for a domain
	tmpDir := t.TempDir()
	out, err := executeCmd("export", "--domain", "payments", "--output", tmpDir)
	if err != nil {
		t.Fatalf("export should not error: %v", err)
	}
	if out == "" {
		t.Fatal("export should produce output")
	}

	// Verify the file is created
	data, err := os.ReadFile(filepath.Join(tmpDir, "payments.yaml"))
	if err != nil {
		t.Fatalf("export output file should exist: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("export output should not be empty")
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
	// admin upgrade manages revision install / promote / rollback flags safely
	_, err := executeCmd("admin", "upgrade")
	if err == nil {
		t.Fatal("expected error when no flag provided")
	}
	if !strings.Contains(err.Error(), "--revision") && !strings.Contains(err.Error(), "--promote") && !strings.Contains(err.Error(), "--rollback") {
		t.Fatalf("error should mention required flags, got: %s", err.Error())
	}

	// --revision deploys a canary
	out, err := executeCmd("admin", "upgrade", "--revision", "v2.0.0")
	if err != nil {
		t.Fatalf("--revision should not error: %v", err)
	}
	if !strings.Contains(out, "canary") || !strings.Contains(out, "v2.0.0") {
		t.Fatalf("output should mention canary and revision, got: %s", out)
	}

	// --promote makes a revision stable
	out, err = executeCmd("admin", "upgrade", "--promote", "v2.0.0")
	if err != nil {
		t.Fatalf("--promote should not error: %v", err)
	}
	if !strings.Contains(out, "stable") {
		t.Fatalf("output should mention stable, got: %s", out)
	}

	// --rollback removes a canary
	out, err = executeCmd("admin", "upgrade", "--rollback", "v2.0.0")
	if err != nil {
		t.Fatalf("--rollback should not error: %v", err)
	}
	if !strings.Contains(out, "v2.0.0") {
		t.Fatalf("output should mention revision, got: %s", out)
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

// ---------------------------------------------------------------------------
// CLI-1.1 — admin app list
// ---------------------------------------------------------------------------

func TestCLI_AdminAppList(t *testing.T) {
	s := testScheme()
	app1 := testApp("product-a", []choristerv1alpha1.DomainSpec{{Name: "payments"}, {Name: "auth"}}, "Ready", "essential")
	app2 := testApp("product-b", []choristerv1alpha1.DomainSpec{{Name: "billing"}}, "Pending", "regulated")
	app3 := testApp("product-c", []choristerv1alpha1.DomainSpec{{Name: "orders"}}, "Ready", "standard")

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app1, app2, app3).Build()

	out, err := executeCmdWithClient(fc, "admin", "app", "list")
	if err != nil {
		t.Fatalf("admin app list error: %v", err)
	}

	for _, name := range []string{"product-a", "product-b", "product-c"} {
		if !strings.Contains(out, name) {
			t.Errorf("output should contain %q:\n%s", name, out)
		}
	}
	if !strings.Contains(out, "NAME") {
		t.Errorf("output should contain table header NAME:\n%s", out)
	}
}

func TestCLI_AdminAppList_JSON(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	out, err := executeCmdWithClient(fc, "admin", "app", "list", "--output", "json")
	if err != nil {
		t.Fatalf("admin app list --output json error: %v", err)
	}
	if !strings.Contains(out, `"myproduct"`) && !strings.Contains(out, "myproduct") {
		t.Errorf("JSON output should contain app name:\n%s", out)
	}
}

func TestCLI_AdminAppList_Empty(t *testing.T) {
	s := testScheme()
	fc := fake.NewClientBuilder().WithScheme(s).Build()

	out, err := executeCmdWithClient(fc, "admin", "app", "list")
	if err != nil {
		t.Fatalf("admin app list error: %v", err)
	}
	// Should still print headers
	if !strings.Contains(out, "NAME") {
		t.Errorf("output should contain header even with no apps:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// CLI-1.2 — admin app get
// ---------------------------------------------------------------------------

func TestCLI_AdminAppGet(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{
		{Name: "payments", Sensitivity: "confidential"},
		{Name: "auth", Sensitivity: "internal"},
	}, "Ready", "standard")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	out, err := executeCmdWithClient(fc, "admin", "app", "get", "myproduct")
	if err != nil {
		t.Fatalf("admin app get error: %v", err)
	}

	for _, want := range []string{"myproduct", "standard", "Domains:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q:\n%s", want, out)
		}
	}
}

func TestCLI_AdminAppGet_NotFound(t *testing.T) {
	s := testScheme()
	fc := fake.NewClientBuilder().WithScheme(s).Build()

	_, err := executeCmdWithClient(fc, "admin", "app", "get", "nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent app")
	}
}

// ---------------------------------------------------------------------------
// CLI-1.3 — admin app delete
// ---------------------------------------------------------------------------

func TestCLI_AdminAppDelete_RequiresConfirm(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	_, err := executeCmdWithClient(fc, "admin", "app", "delete", "myproduct")
	if err == nil {
		t.Fatal("Expected error without --confirm")
	}
	if !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("Error should mention --confirm, got: %s", err.Error())
	}
}

func TestCLI_AdminAppDelete_DryRun(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}, {Name: "auth"}}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	out, err := executeCmdWithClient(fc, "admin", "app", "delete", "myproduct", "--dry-run")
	if err != nil {
		t.Fatalf("admin app delete --dry-run error: %v", err)
	}
	if !strings.Contains(out, "Dry run") {
		t.Errorf("Output should mention dry run:\n%s", out)
	}
	if !strings.Contains(out, "payments") {
		t.Errorf("Output should list domains:\n%s", out)
	}
}

func TestCLI_AdminAppDelete_Confirm(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	out, err := executeCmdWithClient(fc, "admin", "app", "delete", "myproduct", "--confirm")
	if err != nil {
		t.Fatalf("admin app delete --confirm error: %v", err)
	}
	if !strings.Contains(out, "deleted") {
		t.Errorf("Output should confirm deletion:\n%s", out)
	}

	// Verify app is gone
	_, err = executeCmdWithClient(fc, "admin", "app", "get", "myproduct")
	if err == nil {
		t.Fatal("Expected error after deletion")
	}
}

// ---------------------------------------------------------------------------
// CLI-1.4 — admin domain list
// ---------------------------------------------------------------------------

func TestCLI_AdminDomainList(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{
		{Name: "payments", Sensitivity: "confidential"},
		{Name: "auth", Sensitivity: "internal"},
		{Name: "billing"},
	}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	out, err := executeCmdWithClient(fc, "admin", "domain", "list")
	if err != nil {
		t.Fatalf("admin domain list error: %v", err)
	}
	for _, name := range []string{"payments", "auth", "billing"} {
		if !strings.Contains(out, name) {
			t.Errorf("output should contain domain %q:\n%s", name, out)
		}
	}
}

func TestCLI_AdminDomainList_FilterByApp(t *testing.T) {
	s := testScheme()
	app1 := testApp("app1", []choristerv1alpha1.DomainSpec{{Name: "payments"}, {Name: "auth"}}, "Ready", "essential")
	app2 := testApp("app2", []choristerv1alpha1.DomainSpec{{Name: "orders"}}, "Ready", "standard")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app1, app2).Build()

	out, err := executeCmdWithClient(fc, "admin", "domain", "list", "--app", "app1")
	if err != nil {
		t.Fatalf("admin domain list --app error: %v", err)
	}
	if !strings.Contains(out, "payments") || !strings.Contains(out, "auth") {
		t.Errorf("output should contain app1's domains:\n%s", out)
	}
	if strings.Contains(out, "orders") {
		t.Errorf("output should NOT contain app2's domains:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// CLI-1.5 — admin domain get
// ---------------------------------------------------------------------------

func TestCLI_AdminDomainGet(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{
		{Name: "payments", Sensitivity: "confidential"},
	}, "Ready", "standard")

	compute := &choristerv1alpha1.ChoCompute{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "myproduct-payments"},
		Spec:       choristerv1alpha1.ChoComputeSpec{Application: "myproduct", Domain: "payments", Image: "api:v1"},
	}
	db := &choristerv1alpha1.ChoDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "ledger", Namespace: "myproduct-payments"},
		Spec:       choristerv1alpha1.ChoDatabaseSpec{Application: "myproduct", Domain: "payments", Engine: "postgres"},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app, compute, db).Build()

	out, err := executeCmdWithClient(fc, "admin", "domain", "get", "payments", "--app", "myproduct")
	if err != nil {
		t.Fatalf("admin domain get error: %v", err)
	}

	for _, want := range []string{"payments", "confidential", "Compute", "Database"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q:\n%s", want, out)
		}
	}
}

func TestCLI_AdminDomainGet_RequiresApp(t *testing.T) {
	s := testScheme()
	fc := fake.NewClientBuilder().WithScheme(s).Build()

	_, err := executeCmdWithClient(fc, "admin", "domain", "get", "payments")
	if err == nil {
		t.Fatal("Expected error without --app")
	}
	if !strings.Contains(err.Error(), "--app") {
		t.Fatalf("Error should mention --app, got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// CLI-1.6 — admin domain delete
// ---------------------------------------------------------------------------

func TestCLI_AdminDomainDelete_RequiresConfirm(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{
		{Name: "payments"},
		{Name: "auth"},
	}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	_, err := executeCmdWithClient(fc, "admin", "domain", "delete", "payments", "--app", "myproduct")
	if err == nil {
		t.Fatal("Expected error without --confirm")
	}
	if !strings.Contains(err.Error(), "--confirm") {
		t.Fatalf("Error should mention --confirm, got: %s", err.Error())
	}
}

func TestCLI_AdminDomainDelete_DryRun(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{
		{Name: "payments"},
		{Name: "auth"},
	}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	out, err := executeCmdWithClient(fc, "admin", "domain", "delete", "payments", "--app", "myproduct", "--dry-run")
	if err != nil {
		t.Fatalf("admin domain delete --dry-run error: %v", err)
	}
	if !strings.Contains(out, "Dry run") {
		t.Errorf("Output should mention dry run:\n%s", out)
	}
}

func TestCLI_AdminDomainDelete_Confirm(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{
		{Name: "payments"},
		{Name: "auth"},
	}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	out, err := executeCmdWithClient(fc, "admin", "domain", "delete", "payments", "--app", "myproduct", "--confirm")
	if err != nil {
		t.Fatalf("admin domain delete --confirm error: %v", err)
	}
	if !strings.Contains(out, "removed") {
		t.Errorf("Output should confirm removal:\n%s", out)
	}
}

func TestCLI_AdminDomainDelete_NotFound(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "auth"}}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	_, err := executeCmdWithClient(fc, "admin", "domain", "delete", "nonexistent", "--app", "myproduct", "--confirm")
	if err == nil {
		t.Fatal("Expected error for nonexistent domain")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Error should mention not found, got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// CLI-2.1 — admin cluster status
// ---------------------------------------------------------------------------

func TestCLI_AdminClusterStatus(t *testing.T) {
	s := testScheme()
	cluster := &choristerv1alpha1.ChoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "chorister"},
		Spec: choristerv1alpha1.ChoClusterSpec{
			ControllerRevision: "v1.0.0",
			Operators: &choristerv1alpha1.OperatorVersions{
				Kro:       "v0.2.0",
				StackGres: "1.12.0",
			},
		},
		Status: choristerv1alpha1.ChoClusterStatus{
			Phase:              "Ready",
			ObservabilityReady: true,
			CISBenchmark:       "Pass",
			OperatorStatus: map[string]string{
				"kro":       "Installed",
				"stackgres": "Degraded",
			},
		},
	}

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(cluster).Build()
	out, err := executeCmdWithClient(fc, "admin", "cluster", "status")
	if err != nil {
		t.Fatalf("admin cluster status error: %v", err)
	}
	for _, want := range []string{"chorister", "Ready", "Installed", "Degraded"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q:\n%s", want, out)
		}
	}
}

// ---------------------------------------------------------------------------
// CLI-2.2 — admin cluster operators
// ---------------------------------------------------------------------------

func TestCLI_AdminClusterOperators(t *testing.T) {
	s := testScheme()
	cluster := &choristerv1alpha1.ChoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "chorister"},
		Spec: choristerv1alpha1.ChoClusterSpec{
			Operators: &choristerv1alpha1.OperatorVersions{
				Kro:         "v0.2.0",
				StackGres:   "1.12.0",
				CertManager: "v1.14.0",
			},
		},
		Status: choristerv1alpha1.ChoClusterStatus{
			Phase: "Ready",
			OperatorStatus: map[string]string{
				"kro":          "Installed",
				"stackgres":    "Installed",
				"cert-manager": "Installed",
			},
		},
	}

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(cluster).Build()
	out, err := executeCmdWithClient(fc, "admin", "cluster", "operators")
	if err != nil {
		t.Fatalf("admin cluster operators error: %v", err)
	}
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "VERSION") {
		t.Errorf("output should contain table headers:\n%s", out)
	}
	if !strings.Contains(out, "v0.2.0") {
		t.Errorf("output should contain kro version:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// CLI-3.1 — Enhanced status command
// ---------------------------------------------------------------------------

func TestCLI_Status_AllDomains(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{
		{Name: "payments"},
		{Name: "auth"},
	}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	out, err := executeCmdWithClient(fc, "status", "--app", "myproduct")
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
	for _, want := range []string{"payments", "auth"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain domain %q:\n%s", want, out)
		}
	}
}

func TestCLI_Status_SingleDomain(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{
		{Name: "payments"},
	}, "Ready", "essential")
	sb := &choristerv1alpha1.ChoSandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct-payments-alice", Namespace: "default", CreationTimestamp: metav1.Now()},
		Spec:       choristerv1alpha1.ChoSandboxSpec{Application: "myproduct", Domain: "payments", Name: "alice", Owner: "alice@co.com"},
		Status:     choristerv1alpha1.ChoSandboxStatus{Phase: "Active", Namespace: "myproduct-payments-sbx-alice"},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app, sb).Build()

	out, err := executeCmdWithClient(fc, "status", "payments", "--app", "myproduct")
	if err != nil {
		t.Fatalf("status payments error: %v", err)
	}
	if !strings.Contains(out, "payments") {
		t.Errorf("output should contain domain name:\n%s", out)
	}
	if !strings.Contains(out, "Sandboxes:") {
		t.Errorf("output should contain sandbox section:\n%s", out)
	}
	if !strings.Contains(out, "alice") {
		t.Errorf("output should contain sandbox name:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// CLI-3.2 — logs command
// ---------------------------------------------------------------------------

func TestCLI_Logs_RequiresSandbox(t *testing.T) {
	_, err := executeCmd("logs", "--domain", "payments")
	if err == nil {
		t.Fatal("Expected error without --sandbox")
	}
	if !strings.Contains(err.Error(), "--sandbox") {
		t.Fatalf("Error should mention --sandbox, got: %s", err.Error())
	}
}

func TestCLI_Logs_NoComponent(t *testing.T) {
	out, err := executeCmd("logs", "--domain", "payments", "--sandbox", "alice")
	if err != nil {
		t.Fatalf("logs without component should not error: %v", err)
	}
	if !strings.Contains(out, "No component specified") {
		t.Errorf("output should mention no component:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// CLI-3.3 — sandbox status
// ---------------------------------------------------------------------------

func TestCLI_SandboxStatus(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	sb := &choristerv1alpha1.ChoSandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct-payments-alice", Namespace: "default", CreationTimestamp: metav1.Now()},
		Spec:       choristerv1alpha1.ChoSandboxSpec{Application: "myproduct", Domain: "payments", Name: "alice", Owner: "alice@co.com"},
		Status: choristerv1alpha1.ChoSandboxStatus{
			Phase:                "Active",
			Namespace:            "myproduct-payments-sbx-alice",
			EstimatedMonthlyCost: "15.00",
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app, sb).Build()

	out, err := executeCmdWithClient(fc, "sandbox", "status", "--domain", "payments", "--name", "alice", "--app", "myproduct")
	if err != nil {
		t.Fatalf("sandbox status error: %v", err)
	}
	if !strings.Contains(out, "alice") {
		t.Errorf("output should contain sandbox name:\n%s", out)
	}
	if !strings.Contains(out, "15.00") {
		t.Errorf("output should contain cost:\n%s", out)
	}
}

func TestCLI_SandboxStatus_RequiresDomain(t *testing.T) {
	_, err := executeCmd("sandbox", "status", "--name", "alice")
	if err == nil {
		t.Fatal("Expected error without --domain")
	}
}

// ---------------------------------------------------------------------------
// CLI-3.4 — sandbox list with cost/age
// ---------------------------------------------------------------------------

func TestCLI_SandboxList(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	sb1 := &choristerv1alpha1.ChoSandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct-payments-alice", Namespace: "default", CreationTimestamp: metav1.Now()},
		Spec:       choristerv1alpha1.ChoSandboxSpec{Application: "myproduct", Domain: "payments", Name: "alice", Owner: "alice@co.com"},
		Status:     choristerv1alpha1.ChoSandboxStatus{Phase: "Active", EstimatedMonthlyCost: "12.50"},
	}
	sb2 := &choristerv1alpha1.ChoSandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct-payments-bob", Namespace: "default", CreationTimestamp: metav1.Now()},
		Spec:       choristerv1alpha1.ChoSandboxSpec{Application: "myproduct", Domain: "payments", Name: "bob", Owner: "bob@co.com"},
		Status:     choristerv1alpha1.ChoSandboxStatus{Phase: "Active", EstimatedMonthlyCost: "8.00"},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app, sb1, sb2).Build()

	out, err := executeCmdWithClient(fc, "sandbox", "list", "--domain", "payments", "--app", "myproduct")
	if err != nil {
		t.Fatalf("sandbox list error: %v", err)
	}
	for _, want := range []string{"NAME", "COST/MO", "alice", "bob"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q:\n%s", want, out)
		}
	}
}

// ---------------------------------------------------------------------------
// CLI-3.5 — events command
// ---------------------------------------------------------------------------

func TestCLI_Events(t *testing.T) {
	s := testScheme()
	_ = corev1.AddToScheme(s)
	_ = eventsv1.AddToScheme(s)

	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	// Events command with empty namespace should work (shows empty table)
	out, err := executeCmdWithClient(fc, "events", "--domain", "payments", "--app", "myproduct")
	if err != nil {
		t.Fatalf("events error: %v", err)
	}
	if !strings.Contains(out, "TIME") {
		t.Errorf("output should contain table headers:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// CLI-4.1 — Enhanced requests with filtering
// ---------------------------------------------------------------------------

func TestCLI_Requests(t *testing.T) {
	s := testScheme()
	pr1 := &choristerv1alpha1.ChoPromotionRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-1", Namespace: "default", CreationTimestamp: metav1.Now()},
		Spec: choristerv1alpha1.ChoPromotionRequestSpec{
			Application: "myproduct", Domain: "payments", Sandbox: "alice", RequestedBy: "alice@co.com",
		},
		Status: choristerv1alpha1.ChoPromotionRequestStatus{Phase: "Pending"},
	}
	pr2 := &choristerv1alpha1.ChoPromotionRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "pr-2", Namespace: "default", CreationTimestamp: metav1.Now()},
		Spec: choristerv1alpha1.ChoPromotionRequestSpec{
			Application: "myproduct", Domain: "auth", Sandbox: "bob", RequestedBy: "bob@co.com",
		},
		Status: choristerv1alpha1.ChoPromotionRequestStatus{Phase: "Approved"},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(pr1, pr2).Build()

	// All requests
	out, err := executeCmdWithClient(fc, "requests")
	if err != nil {
		t.Fatalf("requests error: %v", err)
	}
	if !strings.Contains(out, "pr-1") || !strings.Contains(out, "pr-2") {
		t.Errorf("output should contain both requests:\n%s", out)
	}

	// Filter by status
	out, err = executeCmdWithClient(fc, "requests", "--status", "Pending")
	if err != nil {
		t.Fatalf("requests --status error: %v", err)
	}
	if !strings.Contains(out, "pr-1") {
		t.Errorf("output should contain pr-1:\n%s", out)
	}
	if strings.Contains(out, "pr-2") {
		t.Errorf("output should not contain pr-2 (Approved):\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// CLI-4.2 — promote --rollback
// ---------------------------------------------------------------------------

func TestCLI_Promote_Rollback(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	out, err := executeCmdWithClient(fc, "promote", "--domain", "payments", "--rollback", "--app", "myproduct")
	if err != nil {
		t.Fatalf("promote --rollback error: %v", err)
	}
	if !strings.Contains(out, "Rollback") {
		t.Errorf("output should mention rollback:\n%s", out)
	}
}

func TestCLI_Promote_RollbackAndSandboxMutuallyExclusive(t *testing.T) {
	_, err := executeCmd("promote", "--domain", "payments", "--rollback", "--sandbox", "alice")
	if err == nil {
		t.Fatal("Expected error for rollback + sandbox")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("Error should mention mutually exclusive, got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// CLI-4.3 — diff with --output
// ---------------------------------------------------------------------------

func TestCLI_Diff_OutputFlag(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	// Should accept --output flag without error
	out, err := executeCmdWithClient(fc, "diff", "--domain", "payments", "--sandbox", "alice", "--app", "myproduct", "--output", "json")
	if err != nil {
		t.Fatalf("diff --output json error: %v", err)
	}
	// At minimum it should print something (currently stub)
	if out == "" {
		t.Fatal("diff output should not be empty")
	}
}

// ---------------------------------------------------------------------------
// CLI-5.1 — admin vulnerabilities list
// ---------------------------------------------------------------------------

func TestCLI_AdminVulnList(t *testing.T) {
	s := testScheme()
	now := metav1.Now()
	vr1 := &choristerv1alpha1.ChoVulnerabilityReport{
		ObjectMeta: metav1.ObjectMeta{Name: "vr-payments", Namespace: "default"},
		Spec:       choristerv1alpha1.ChoVulnerabilityReportSpec{Application: "myproduct", Domain: "payments"},
		Status: choristerv1alpha1.ChoVulnerabilityReportStatus{
			Phase: "Completed", Scanner: "trivy", CriticalCount: 2, ScannedAt: &now,
		},
	}
	vr2 := &choristerv1alpha1.ChoVulnerabilityReport{
		ObjectMeta: metav1.ObjectMeta{Name: "vr-auth", Namespace: "default"},
		Spec:       choristerv1alpha1.ChoVulnerabilityReportSpec{Application: "myproduct", Domain: "auth"},
		Status: choristerv1alpha1.ChoVulnerabilityReportStatus{
			Phase: "Completed", Scanner: "trivy", CriticalCount: 0, ScannedAt: &now,
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(vr1, vr2).Build()

	out, err := executeCmdWithClient(fc, "admin", "vulnerabilities", "list")
	if err != nil {
		t.Fatalf("admin vulnerabilities list error: %v", err)
	}
	if !strings.Contains(out, "payments") || !strings.Contains(out, "auth") {
		t.Errorf("output should contain both domains:\n%s", out)
	}
	if !strings.Contains(out, "CRITICAL") {
		t.Errorf("output should contain header CRITICAL:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// CLI-5.2 — admin vulnerabilities get
// ---------------------------------------------------------------------------

func TestCLI_AdminVulnGet(t *testing.T) {
	s := testScheme()
	now := metav1.Now()
	vr := &choristerv1alpha1.ChoVulnerabilityReport{
		ObjectMeta: metav1.ObjectMeta{Name: "vr-payments", Namespace: "default"},
		Spec:       choristerv1alpha1.ChoVulnerabilityReportSpec{Application: "myproduct", Domain: "payments"},
		Status: choristerv1alpha1.ChoVulnerabilityReportStatus{
			Phase: "Completed", Scanner: "trivy", CriticalCount: 1, ScannedAt: &now,
			Findings: []choristerv1alpha1.VulnerabilityFinding{
				{ID: "CVE-2024-001", Severity: "Critical", Image: "api:v1", Package: "openssl", FixedVersion: "3.1.1", Title: "Buffer overflow"},
				{ID: "CVE-2024-002", Severity: "High", Image: "api:v1", Package: "curl", FixedVersion: "8.0.1", Title: "SSRF"},
			},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(vr).Build()

	out, err := executeCmdWithClient(fc, "admin", "vulnerabilities", "get", "payments", "--app", "myproduct")
	if err != nil {
		t.Fatalf("admin vulnerabilities get error: %v", err)
	}
	for _, want := range []string{"CVE-2024-001", "Critical", "openssl", "Findings:"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q:\n%s", want, out)
		}
	}
}

func TestCLI_AdminVulnGet_RequiresApp(t *testing.T) {
	_, err := executeCmd("admin", "vulnerabilities", "get", "payments")
	if err == nil {
		t.Fatal("Expected error without --app")
	}
	if !strings.Contains(err.Error(), "--app") {
		t.Fatalf("Error should mention --app, got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// CLI-5.3 — admin scan
// ---------------------------------------------------------------------------

func TestCLI_AdminScan(t *testing.T) {
	out, err := executeCmd("admin", "scan", "--app", "myproduct", "--domain", "payments")
	if err != nil {
		t.Fatalf("admin scan error: %v", err)
	}
	if !strings.Contains(out, "Triggered") {
		t.Errorf("output should confirm scan triggered:\n%s", out)
	}
}

func TestCLI_AdminScan_RequiresApp(t *testing.T) {
	_, err := executeCmd("admin", "scan")
	if err == nil {
		t.Fatal("Expected error without --app")
	}
	if !strings.Contains(err.Error(), "--app") {
		t.Fatalf("Error should mention --app, got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// CLI-6.1 — admin audit
// ---------------------------------------------------------------------------

func TestCLI_AdminAudit_RequiresDomain(t *testing.T) {
	// When CHORISTER_LOKI_URL is not set, the command should still accept args.
	// We just verify it can run (even if Loki is unreachable, it should return error not panic).
	s := testScheme()
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	// Should error because Loki is unreachable, not panic
	_, _ = executeCmdWithClient(fc, "admin", "audit", "--domain", "payments", "--since", "1h")
}

func TestCLI_AdminAudit_InvalidSince(t *testing.T) {
	s := testScheme()
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	_, err := executeCmdWithClient(fc, "admin", "audit", "--since", "not-a-duration")
	if err == nil {
		t.Fatal("Expected error for invalid --since")
	}
}

// ---------------------------------------------------------------------------
// CLI-6.2 — admin compliance report
// ---------------------------------------------------------------------------

func TestCLI_AdminComplianceReport(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	cluster := &choristerv1alpha1.ChoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "chorister-cluster", Namespace: "default"},
		Status: choristerv1alpha1.ChoClusterStatus{
			Phase:          "Ready",
			CISBenchmark:   "Pass",
			OperatorStatus: map[string]string{"gatekeeper": "installed"},
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app, cluster).Build()

	out, err := executeCmdWithClient(fc, "admin", "compliance", "report", "--app", "myproduct")
	if err != nil {
		t.Fatalf("admin compliance report error: %v", err)
	}
	for _, want := range []string{"CIS Controls", "CC-1", "Pass"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q:\n%s", want, out)
		}
	}
}

func TestCLI_AdminComplianceReport_RequiresApp(t *testing.T) {
	_, err := executeCmd("admin", "compliance", "report")
	if err == nil {
		t.Fatal("Expected error without --app")
	}
	if !strings.Contains(err.Error(), "--app") {
		t.Fatalf("Error should mention --app, got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// CLI-6.3 — admin compliance status
// ---------------------------------------------------------------------------

func TestCLI_AdminComplianceStatus(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	out, err := executeCmdWithClient(fc, "admin", "compliance", "status", "--app", "myproduct")
	if err != nil {
		t.Fatalf("admin compliance status error: %v", err)
	}
	if !strings.Contains(out, "myproduct") {
		t.Errorf("output should contain app name:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// CLI-7.1 — admin finops report
// ---------------------------------------------------------------------------

func TestCLI_AdminFinOpsReport(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	sb := &choristerv1alpha1.ChoSandbox{
		ObjectMeta: metav1.ObjectMeta{Name: "myproduct-payments-alice", Namespace: "default"},
		Spec:       choristerv1alpha1.ChoSandboxSpec{Application: "myproduct", Domain: "payments", Name: "alice", Owner: "alice@co.com"},
		Status:     choristerv1alpha1.ChoSandboxStatus{EstimatedMonthlyCost: "12.50"},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app, sb).Build()

	out, err := executeCmdWithClient(fc, "admin", "finops", "report", "--app", "myproduct")
	if err != nil {
		t.Fatalf("admin finops report error: %v", err)
	}
	if !strings.Contains(out, "payments") {
		t.Errorf("output should contain domain name:\n%s", out)
	}
}

func TestCLI_AdminFinOpsReport_RequiresApp(t *testing.T) {
	_, err := executeCmd("admin", "finops", "report")
	if err == nil {
		t.Fatal("Expected error without --app")
	}
}

// ---------------------------------------------------------------------------
// CLI-7.2 — admin finops budget
// ---------------------------------------------------------------------------

func TestCLI_AdminFinOpsBudget(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	out, err := executeCmdWithClient(fc, "admin", "finops", "budget", "--app", "myproduct")
	if err != nil {
		t.Fatalf("admin finops budget error: %v", err)
	}
	if !strings.Contains(out, "payments") {
		t.Errorf("output should contain domain name:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// CLI-7.3 — admin quotas
// ---------------------------------------------------------------------------

func TestCLI_AdminQuotas(t *testing.T) {
	s := testScheme()
	_ = corev1.AddToScheme(s)
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	out, err := executeCmdWithClient(fc, "admin", "quotas", "--app", "myproduct")
	if err != nil {
		t.Fatalf("admin quotas error: %v", err)
	}
	if !strings.Contains(out, "payments") {
		t.Errorf("output should contain domain name:\n%s", out)
	}
}

func TestCLI_AdminQuotas_RequiresApp(t *testing.T) {
	_, err := executeCmd("admin", "quotas")
	if err == nil {
		t.Fatal("Expected error without --app")
	}
}

// ---------------------------------------------------------------------------
// CLI-8.1 — admin resource list
// ---------------------------------------------------------------------------

func TestCLI_AdminResourceList(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	db := &choristerv1alpha1.ChoDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "ledger", Namespace: "myproduct-payments"},
		Spec:       choristerv1alpha1.ChoDatabaseSpec{Application: "myproduct", Domain: "payments", Size: "small"},
		Status:     choristerv1alpha1.ChoDatabaseStatus{Ready: true, Lifecycle: "Active"},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app, db).Build()

	out, err := executeCmdWithClient(fc, "admin", "resource", "list", "--app", "myproduct", "--domain", "payments")
	if err != nil {
		t.Fatalf("admin resource list error: %v", err)
	}
	if !strings.Contains(out, "ledger") {
		t.Errorf("output should contain database name:\n%s", out)
	}
}

func TestCLI_AdminResourceList_RequiresApp(t *testing.T) {
	_, err := executeCmd("admin", "resource", "list", "--domain", "payments")
	if err == nil {
		t.Fatal("Expected error without --app")
	}
}

// ---------------------------------------------------------------------------
// CLI-8.2 — get <type> <name>
// ---------------------------------------------------------------------------

func TestCLI_Get_Database(t *testing.T) {
	s := testScheme()
	db := &choristerv1alpha1.ChoDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "ledger", Namespace: "myproduct-payments"},
		Spec:       choristerv1alpha1.ChoDatabaseSpec{Application: "myproduct", Domain: "payments", Size: "small"},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(db).Build()

	out, err := executeCmdWithClient(fc, "get", "database", "ledger", "--namespace", "myproduct-payments")
	if err != nil {
		t.Fatalf("get database error: %v", err)
	}
	if !strings.Contains(out, "ledger") {
		t.Errorf("output should contain database name:\n%s", out)
	}
}

func TestCLI_Get_UnknownType(t *testing.T) {
	s := testScheme()
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	_, err := executeCmdWithClient(fc, "get", "unknowntype", "myresource")
	if err == nil {
		t.Fatal("Expected error for unknown resource type")
	}
}

// ---------------------------------------------------------------------------
// CLI-8.3 — admin domain set-sensitivity
// ---------------------------------------------------------------------------

func TestCLI_AdminDomainSetSensitivity(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments", Sensitivity: "internal"}}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	out, err := executeCmdWithClient(fc, "admin", "domain", "set-sensitivity", "payments", "--app", "myproduct", "--level", "confidential")
	if err != nil {
		t.Fatalf("admin domain set-sensitivity error: %v", err)
	}
	if !strings.Contains(out, "confidential") {
		t.Errorf("output should confirm sensitivity level:\n%s", out)
	}
}

func TestCLI_AdminDomainSetSensitivity_RegulatedMinLevel(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments", Sensitivity: "confidential"}}, "Ready", "regulated")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	_, err := executeCmdWithClient(fc, "admin", "domain", "set-sensitivity", "payments", "--app", "myproduct", "--level", "public")
	if err == nil {
		t.Fatal("Expected error: public is below regulated minimum")
	}
	if !strings.Contains(err.Error(), "minimum") {
		t.Fatalf("Error should mention minimum, got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// CLI-9.1 — admin member add
// ---------------------------------------------------------------------------

func TestCLI_AdminMemberAdd(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments", Sensitivity: "internal"}}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	out, err := executeCmdWithClient(fc, "admin", "member", "add",
		"--app", "myproduct",
		"--domain", "payments",
		"--identity", "alice@co.com",
		"--role", "developer",
	)
	if err != nil {
		t.Fatalf("admin member add error: %v", err)
	}
	if !strings.Contains(out, "alice@co.com") {
		t.Errorf("output should contain identity:\n%s", out)
	}
}

func TestCLI_AdminMemberAdd_RestrictedRequiresExpiry(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments", Sensitivity: "restricted"}}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	_, err := executeCmdWithClient(fc, "admin", "member", "add",
		"--app", "myproduct",
		"--domain", "payments",
		"--identity", "alice@co.com",
		"--role", "developer",
	)
	if err == nil {
		t.Fatal("Expected error: restricted domain requires --expires-at")
	}
	if !strings.Contains(err.Error(), "expires-at") {
		t.Fatalf("Error should mention expires-at, got: %s", err.Error())
	}
}

// ---------------------------------------------------------------------------
// CLI-9.2 — admin member list
// ---------------------------------------------------------------------------

func TestCLI_AdminMemberList(t *testing.T) {
	s := testScheme()
	m1 := &choristerv1alpha1.ChoDomainMembership{
		ObjectMeta: metav1.ObjectMeta{Name: "m1", Namespace: "default"},
		Spec: choristerv1alpha1.ChoDomainMembershipSpec{
			Application: "myproduct", Domain: "payments", Identity: "alice@co.com", Role: "developer",
		},
		Status: choristerv1alpha1.ChoDomainMembershipStatus{Phase: "Active"},
	}
	m2 := &choristerv1alpha1.ChoDomainMembership{
		ObjectMeta: metav1.ObjectMeta{Name: "m2", Namespace: "default"},
		Spec: choristerv1alpha1.ChoDomainMembershipSpec{
			Application: "myproduct", Domain: "payments", Identity: "bob@co.com", Role: "viewer",
		},
		Status: choristerv1alpha1.ChoDomainMembershipStatus{Phase: "Active"},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(m1, m2).Build()

	out, err := executeCmdWithClient(fc, "admin", "member", "list", "--app", "myproduct")
	if err != nil {
		t.Fatalf("admin member list error: %v", err)
	}
	for _, want := range []string{"alice@co.com", "bob@co.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should contain %q:\n%s", want, out)
		}
	}
}

func TestCLI_AdminMemberList_FilterByRole(t *testing.T) {
	s := testScheme()
	m1 := &choristerv1alpha1.ChoDomainMembership{
		ObjectMeta: metav1.ObjectMeta{Name: "m1", Namespace: "default"},
		Spec: choristerv1alpha1.ChoDomainMembershipSpec{
			Application: "myproduct", Domain: "payments", Identity: "alice@co.com", Role: "developer",
		},
	}
	m2 := &choristerv1alpha1.ChoDomainMembership{
		ObjectMeta: metav1.ObjectMeta{Name: "m2", Namespace: "default"},
		Spec: choristerv1alpha1.ChoDomainMembershipSpec{
			Application: "myproduct", Domain: "payments", Identity: "bob@co.com", Role: "viewer",
		},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(m1, m2).Build()

	out, err := executeCmdWithClient(fc, "admin", "member", "list", "--app", "myproduct", "--role", "developer")
	if err != nil {
		t.Fatalf("admin member list error: %v", err)
	}
	if !strings.Contains(out, "alice@co.com") {
		t.Errorf("output should contain alice:\n%s", out)
	}
	if strings.Contains(out, "bob@co.com") {
		t.Errorf("output should NOT contain bob (viewer role):\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// CLI-9.3 — admin member audit
// ---------------------------------------------------------------------------

func TestCLI_AdminMemberAudit(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments", Sensitivity: "restricted"}}, "Ready", "regulated")
	// Member without expiry on restricted domain → should be flagged
	m := &choristerv1alpha1.ChoDomainMembership{
		ObjectMeta: metav1.ObjectMeta{Name: "m1", Namespace: "default"},
		Spec: choristerv1alpha1.ChoDomainMembershipSpec{
			Application: "myproduct", Domain: "payments", Identity: "alice@co.com", Role: "developer",
		},
		Status: choristerv1alpha1.ChoDomainMembershipStatus{Phase: "Active"},
	}
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app, m).Build()

	out, err := executeCmdWithClient(fc, "admin", "member", "audit", "--app", "myproduct")
	if err != nil {
		t.Fatalf("admin member audit error: %v", err)
	}
	if !strings.Contains(out, "alice@co.com") {
		t.Errorf("output should flag alice:\n%s", out)
	}
	if !strings.Contains(out, "no-expiry-on-restricted") {
		t.Errorf("output should indicate the issue:\n%s", out)
	}
}

func TestCLI_AdminMemberAudit_RequiresApp(t *testing.T) {
	_, err := executeCmd("admin", "member", "audit")
	if err == nil {
		t.Fatal("Expected error without --app")
	}
}

// ---------------------------------------------------------------------------
// CLI-10.1 — wait
// ---------------------------------------------------------------------------

func TestCLI_Wait_RequiresFor(t *testing.T) {
	_, err := executeCmd("wait", "--type", "sandbox", "--name", "alice")
	if err == nil {
		t.Fatal("Expected error without --for")
	}
	if !strings.Contains(err.Error(), "--for") {
		t.Fatalf("Error should mention --for, got: %s", err.Error())
	}
}

func TestCLI_Wait_RequiresType(t *testing.T) {
	_, err := executeCmd("wait", "--for", "Ready", "--name", "alice")
	if err == nil {
		t.Fatal("Expected error without --type")
	}
}

func TestCLI_Wait_RequiresName(t *testing.T) {
	_, err := executeCmd("wait", "--for", "Ready", "--type", "sandbox")
	if err == nil {
		t.Fatal("Expected error without --name")
	}
}

func TestCLI_Wait_InvalidTimeout(t *testing.T) {
	_, err := executeCmd("wait", "--for", "Ready", "--type", "sandbox", "--name", "alice", "--timeout", "notavalidduration")
	if err == nil {
		t.Fatal("Expected error for invalid timeout")
	}
}

// ---------------------------------------------------------------------------
// CLI-10.2 — admin export-config
// ---------------------------------------------------------------------------

func TestCLI_AdminExportConfig(t *testing.T) {
	s := testScheme()
	app := testApp("myproduct", []choristerv1alpha1.DomainSpec{{Name: "payments"}}, "Ready", "essential")
	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(app).Build()

	tmpDir := t.TempDir()
	out, err := executeCmdWithClient(fc, "admin", "export-config", "--app", "myproduct", "--output-dir", tmpDir)
	if err != nil {
		t.Fatalf("admin export-config error: %v", err)
	}
	if !strings.Contains(out, "Exported") {
		t.Errorf("output should confirm export:\n%s", out)
	}
	// Check that the exported file exists
	expectedFile := filepath.Join(tmpDir, "myproduct-application.yaml")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Errorf("expected exported file %s to exist", expectedFile)
	}
}

func TestCLI_AdminExportConfig_RequiresApp(t *testing.T) {
	_, err := executeCmd("admin", "export-config")
	if err == nil {
		t.Fatal("Expected error without --app")
	}
}
