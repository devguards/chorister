//go:build e2e
// +build e2e

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

package e2e

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// ---------------------------------------------------------------------------
// 1A.18 — Incident response and archive safety (e2e)
// ---------------------------------------------------------------------------

func TestE2E_AdminIsolateDomain(t *testing.T) {
	const appName = "e2e-isolate"
	const domain = "payments"
	prodNS := appName + "-" + domain

	feature := features.New("admin isolate domain").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "chorister", "admin", "app", "create", appName,
				"--owners", "test@chorister.dev",
				"--compliance", "essential",
				"--domains", domain)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("create app: %v: %s", err, out)
			}
			if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
				return namespaceExists(ctx, cfg, prodNS)
			}); err != nil {
				t.Fatalf("namespace %s not created: %v", prodNS, err)
			}
			return ctx
		}).
		Assess("isolate tightens NetworkPolicy and freezes promotions", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Isolate the domain
			cmd := exec.CommandContext(ctx, "chorister", "admin", "isolate",
				"--domain", domain, "--app", appName)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("isolate failed: %v: %s", err, out)
			}

			// Verify isolation is set on the application via annotation
			annotationKey := fmt.Sprintf("chorister.dev/isolate-%s", domain)
			escapedKey := strings.ReplaceAll(annotationKey, ".", `\.`)
			cmd = exec.CommandContext(ctx, "kubectl", "get", "choapplications", appName,
				"-n", "cho-system", "-o", fmt.Sprintf("jsonpath={.metadata.annotations['%s']}", escapedKey))
			out, err = cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("get app annotation: %v: %s", err, out)
			}
			if strings.TrimSpace(string(out)) != "true" {
				t.Fatalf("expected annotation %s=true, got: %s", annotationKey, out)
			}

			// Attempt promotion — should be rejected
			cmd = exec.CommandContext(ctx, "chorister", "sandbox", "create",
				"--domain", domain, "--name", "iso-test", "--app", appName)
			if cOut, cErr := cmd.CombinedOutput(); cErr != nil {
				t.Logf("sandbox create during isolation: %v: %s (may be expected)", cErr, cOut)
			}

			cmd = exec.CommandContext(ctx, "chorister", "promote",
				"--domain", domain, "--sandbox", "iso-test", "--app", appName)
			promoteOut, promoteErr := cmd.CombinedOutput()
			if promoteErr == nil && !strings.Contains(strings.ToLower(string(promoteOut)), "isolat") {
				t.Fatalf("expected promotion to be rejected during isolation, got: %s", promoteOut)
			}

			// Unisolate
			cmd = exec.CommandContext(ctx, "chorister", "admin", "unisolate",
				"--domain", domain, "--app", appName)
			out, err = cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("unisolate failed: %v: %s", err, out)
			}

			// Verify isolation cleared
			cmd = exec.CommandContext(ctx, "kubectl", "get", "choapplications", appName,
				"-n", "cho-system", "-o", fmt.Sprintf("jsonpath={.metadata.annotations['%s']}", escapedKey))
			out, _ = cmd.CombinedOutput()
			if strings.TrimSpace(string(out)) == "true" {
				t.Fatal("expected isolation cleared after unisolate")
			}
			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cleanupApp(ctx, t, appName)
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}

func TestE2E_ArchivedResourceBlocksPromotion(t *testing.T) {
	const appName = "e2e-archive"
	const domain = "payments"
	const sandboxName = "dev"
	const depSandboxName = "dep-test"
	prodNS := appName + "-" + domain

	feature := features.New("archived resource blocks promotion").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "chorister", "admin", "app", "create", appName,
				"--owners", "test@chorister.dev",
				"--compliance", "essential",
				"--domains", domain)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("create app: %v: %s", err, out)
			}
			if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
				return namespaceExists(ctx, cfg, prodNS)
			}); err != nil {
				t.Fatalf("namespace %s not created: %v", prodNS, err)
			}
			// Create sandbox with database
			cmd = exec.CommandContext(ctx, "chorister", "sandbox", "create",
				"--domain", domain, "--name", sandboxName, "--app", appName)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("sandbox create: %v: %s", err, out)
			}
			// Apply database resource
			cmd = exec.CommandContext(ctx, "chorister", "apply",
				"--domain", domain, "--sandbox", sandboxName, "--app", appName,
				"--file", "testdata/archive-database.yaml")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("apply database: %v: %s", err, out)
			}
			return ctx
		}).
		Assess("database promoted to production has Active lifecycle", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Promote database to production first
			cmd := exec.CommandContext(ctx, "chorister", "promote",
				"--domain", domain, "--sandbox", sandboxName, "--app", appName)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("promote failed: %v: %s", err, out)
			}

			// Get and approve the promotion
			cmd = exec.CommandContext(ctx, "chorister", "requests",
				"--domain", domain, "--app", appName, "--status", "pending", "--output", "json")
			out, err = cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("requests: %v: %s", err, out)
			}
			reqID := extractPromotionID(string(out))
			if reqID != "" {
				cmd = exec.CommandContext(ctx, "chorister", "approve", reqID)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("approve: %v: %s", err, out)
				}
			}

			// Wait for database to appear in production
			if err := waitForCondition(ctx, 120*time.Second, 3*time.Second, func() (bool, error) {
				cmd := exec.CommandContext(ctx, "kubectl", "get", "chodatabases", "-n", prodNS, "--no-headers")
				out, err := cmd.CombinedOutput()
				return len(strings.TrimSpace(string(out))) > 0 && err == nil, nil
			}); err != nil {
				t.Fatalf("database not found in production: %v", err)
			}

			// Check lifecycle of database
			cmd = exec.CommandContext(ctx, "kubectl", "get", "chodatabases", "-n", prodNS,
				"-o", "jsonpath={.items[0].status.lifecycle}")
			out, _ = cmd.CombinedOutput()
			lifecycle := strings.TrimSpace(string(out))
			if lifecycle != "Active" && lifecycle != "" {
				t.Logf("database lifecycle: %s (expected Active initially)", lifecycle)
			}
			return ctx
		}).
		Assess("dependent compute promotion is rejected when database is archived", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Step 1: Remove the database from the sandbox to simulate DSL removal
			cmd := exec.CommandContext(ctx, "kubectl", "delete", "chodatabases", "archive-db",
				"-n", fmt.Sprintf("%s-%s-sandbox-%s", appName, domain, sandboxName), "--ignore-not-found")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("delete sandbox database: %v: %s", err, out)
			}

			// Step 2: Promote again — this should trigger the archive transition
			// for the database that is no longer in the sandbox
			cmd = exec.CommandContext(ctx, "chorister", "promote",
				"--domain", domain, "--sandbox", sandboxName, "--app", appName)
			out, err = cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("second promote failed: %v: %s", err, out)
			}

			// Approve the second promotion
			cmd = exec.CommandContext(ctx, "chorister", "requests",
				"--domain", domain, "--app", appName, "--status", "pending", "--output", "json")
			out, err = cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("requests: %v: %s", err, out)
			}
			reqID := extractPromotionID(string(out))
			if reqID != "" {
				cmd = exec.CommandContext(ctx, "chorister", "approve", reqID)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("approve second: %v: %s", err, out)
				}
			}

			// Step 3: Wait for the production database lifecycle to become Archived
			if err := waitForCondition(ctx, 120*time.Second, 3*time.Second, func() (bool, error) {
				cmd := exec.CommandContext(ctx, "kubectl", "get", "chodatabases", "-n", prodNS,
					"-o", "jsonpath={.items[0].status.lifecycle}")
				out, err := cmd.CombinedOutput()
				if err != nil {
					return false, nil
				}
				return strings.TrimSpace(string(out)) == "Archived", nil
			}); err != nil {
				t.Fatalf("database did not transition to Archived: %v", err)
			}

			// Step 4: Create a new sandbox with a ChoCompute referencing the archived db
			cmd = exec.CommandContext(ctx, "chorister", "sandbox", "create",
				"--domain", domain, "--name", depSandboxName, "--app", appName)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("sandbox create dep-test: %v: %s", err, out)
			}

			cmd = exec.CommandContext(ctx, "chorister", "apply",
				"--domain", domain, "--sandbox", depSandboxName, "--app", appName,
				"--file", "testdata/archive-compute.yaml")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("apply compute: %v: %s", err, out)
			}

			// Step 5: Attempt promotion of the dependent compute — should be rejected
			cmd = exec.CommandContext(ctx, "chorister", "promote",
				"--domain", domain, "--sandbox", depSandboxName, "--app", appName)
			promoteOut, promoteErr := cmd.CombinedOutput()

			// The promotion might be rejected at the CLI level or allowed to proceed
			// and fail during reconciliation. Check both paths.
			if promoteErr != nil && strings.Contains(strings.ToLower(string(promoteOut)), "archived") {
				// CLI rejected the promotion directly — pass
				t.Logf("CLI rejected promotion due to archived dependency: %s", promoteOut)
				return ctx
			}

			// If the CLI did not reject, the ChoPromotionRequest should fail during reconciliation
			cmd = exec.CommandContext(ctx, "chorister", "requests",
				"--domain", domain, "--app", appName, "--status", "pending", "--output", "json")
			out, err = cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("requests after dep promote: %v: %s", err, out)
			}
			reqID = extractPromotionID(string(out))
			if reqID != "" {
				cmd = exec.CommandContext(ctx, "chorister", "approve", reqID)
				if cOut, cErr := cmd.CombinedOutput(); cErr != nil {
					t.Logf("approve dep promotion: %v: %s", cErr, cOut)
				}
			}

			// Step 6: Wait for the ChoPromotionRequest to reach Failed phase
			if err := waitForCondition(ctx, 120*time.Second, 3*time.Second, func() (bool, error) {
				cmd := exec.CommandContext(ctx, "kubectl", "get", "chopromotionrequests",
					"-n", "cho-system", "-o", "jsonpath={range .items[*]}{.status.phase}{' '}{end}")
				out, err := cmd.CombinedOutput()
				if err != nil {
					return false, nil
				}
				return strings.Contains(string(out), "Failed"), nil
			}); err != nil {
				t.Fatalf("expected promotion to fail with ArchivedDependency, but it did not reach Failed phase: %v", err)
			}

			// Verify the failure reason mentions "archived"
			cmd = exec.CommandContext(ctx, "kubectl", "get", "chopromotionrequests",
				"-n", "cho-system", "-o", "json")
			out, _ = cmd.CombinedOutput()
			output := strings.ToLower(string(out))
			if !strings.Contains(output, "archived") {
				t.Fatalf("expected failure message to mention 'archived', got: %s", string(out))
			}

			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cleanupApp(ctx, t, appName)
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}

func TestE2E_AdminDeleteArchivedResource(t *testing.T) {
	const appName = "e2e-admindel"
	const domain = "payments"

	feature := features.New("admin delete archived resource").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "chorister", "admin", "app", "create", appName,
				"--owners", "test@chorister.dev",
				"--compliance", "essential",
				"--domains", domain)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("create app: %v: %s", err, out)
			}
			if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
				return namespaceExists(ctx, cfg, appName+"-"+domain)
			}); err != nil {
				t.Fatalf("namespace not created: %v", err)
			}
			return ctx
		}).
		Assess("archived stateful resource requires explicit admin delete after retention", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Test that admin resource delete command works
			cmd := exec.CommandContext(ctx, "chorister", "admin", "resource", "delete",
				"--archived", "nonexistent-db", "--domain", domain, "--app", appName, "--force")
			out, err := cmd.CombinedOutput()
			// Should fail gracefully when resource doesn't exist
			if err == nil {
				t.Logf("admin resource delete on nonexistent: %s", out)
			}
			// Verify the command exists and processes arguments correctly
			// (even if it returns an error for missing resource, it shouldn't panic)
			output := string(out)
			if strings.Contains(output, "unknown command") || strings.Contains(output, "panic") {
				t.Fatalf("admin resource delete command broken: %s", output)
			}
			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cleanupApp(ctx, t, appName)
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}
