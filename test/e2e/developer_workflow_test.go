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
	"os/exec"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// ---------------------------------------------------------------------------
// 1A.13 — Developer daily workflow (e2e, Kind+Cilium)
// ---------------------------------------------------------------------------

func TestE2E_DeveloperWorkflow(t *testing.T) {
	const appName = "e2e-devflow"
	const domain = "payments"
	const sandboxName = "alice"
	sandboxNS := appName + "-" + domain + "-sandbox-" + sandboxName
	prodNS := appName + "-" + domain

	feature := features.New("developer daily workflow").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Create the ChoApplication via CLI
			cmd := exec.CommandContext(ctx, "chorister", "admin", "app", "create", appName,
				"--owners", "test@chorister.dev",
				"--compliance", "essential",
				"--domains", domain)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("failed to create app: %v: %s", err, out)
			}
			// Wait for domain namespace
			if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
				return namespaceExists(ctx, cfg, prodNS)
			}); err != nil {
				t.Fatalf("domain namespace %s not created: %v", prodNS, err)
			}
			return ctx
		}).
		Assess("sandbox create via CLI", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "chorister", "sandbox", "create",
				"--domain", domain, "--name", sandboxName, "--app", appName)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("sandbox create failed: %v: %s", err, out)
			}
			// Wait for sandbox namespace
			if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
				return namespaceExists(ctx, cfg, sandboxNS)
			}); err != nil {
				t.Fatalf("sandbox namespace %s not created: %v", sandboxNS, err)
			}
			return ctx
		}).
		Assess("apply compute + database to sandbox", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "chorister", "apply",
				"--domain", domain, "--sandbox", sandboxName, "--app", appName,
				"--file", "testdata/devflow-resources.yaml")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("apply failed: %v: %s", err, out)
			}
			return ctx
		}).
		Assess("resources running in sandbox namespace", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Wait for a Deployment to appear in the sandbox namespace
			if err := waitForCondition(ctx, 120*time.Second, 3*time.Second, func() (bool, error) {
				var deps appsv1.DeploymentList
				if err := cfg.Client().Resources(sandboxNS).List(ctx, &deps); err != nil {
					return false, err
				}
				return len(deps.Items) > 0, nil
			}); err != nil {
				t.Fatalf("no Deployments found in sandbox namespace %s: %v", sandboxNS, err)
			}
			// Verify credentials Secret exists (from ChoDatabase)
			if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
				var secrets corev1.SecretList
				if err := cfg.Client().Resources(sandboxNS).List(ctx, &secrets); err != nil {
					return false, err
				}
				for _, s := range secrets.Items {
					if strings.Contains(s.Name, "database") && strings.Contains(s.Name, "credentials") {
						return true, nil
					}
				}
				return false, nil
			}); err != nil {
				t.Fatalf("database credentials Secret not found in %s: %v", sandboxNS, err)
			}
			return ctx
		}).
		Assess("diff shows differences from empty prod", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "chorister", "diff",
				"--domain", domain, "--sandbox", sandboxName, "--app", appName)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("diff failed: %v: %s", err, out)
			}
			output := string(out)
			if !strings.Contains(strings.ToLower(output), "added") && !strings.Contains(strings.ToLower(output), "create") && !strings.Contains(strings.ToLower(output), "+") {
				t.Fatalf("expected diff to show added resources, got: %s", output)
			}
			return ctx
		}).
		Assess("promote creates ChoPromotionRequest", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "chorister", "promote",
				"--domain", domain, "--sandbox", sandboxName, "--app", appName)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("promote failed: %v: %s", err, out)
			}
			// Verify promotion request appears
			cmd = exec.CommandContext(ctx, "chorister", "requests",
				"--domain", domain, "--app", appName, "--status", "pending")
			out, err = cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("requests failed: %v: %s", err, out)
			}
			if !strings.Contains(string(out), domain) {
				t.Fatalf("expected pending promotion request, got: %s", out)
			}
			return ctx
		}).
		Assess("approve promotion updates production", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Get the promotion request ID
			cmd := exec.CommandContext(ctx, "chorister", "requests",
				"--domain", domain, "--app", appName, "--status", "pending", "--output", "json")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("requests failed: %v: %s", err, out)
			}
			// Extract first request name (simple string search)
			reqID := extractPromotionID(string(out))
			if reqID == "" {
				t.Fatalf("could not extract promotion request ID from: %s", out)
			}
			// Approve it
			cmd = exec.CommandContext(ctx, "chorister", "approve", reqID)
			out, err = cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("approve failed: %v: %s", err, out)
			}
			// Wait for Deployment to appear in production namespace
			if err := waitForCondition(ctx, 120*time.Second, 3*time.Second, func() (bool, error) {
				var deps appsv1.DeploymentList
				if err := cfg.Client().Resources(prodNS).List(ctx, &deps); err != nil {
					return false, err
				}
				return len(deps.Items) > 0, nil
			}); err != nil {
				t.Fatalf("no Deployments in production namespace %s after approval: %v", prodNS, err)
			}
			return ctx
		}).
		Assess("diff shows no differences after promotion", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "chorister", "diff",
				"--domain", domain, "--sandbox", sandboxName, "--app", appName)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("diff failed: %v: %s", err, out)
			}
			output := strings.ToLower(string(out))
			if strings.Contains(output, "added") || strings.Contains(output, "removed") || strings.Contains(output, "changed") {
				t.Fatalf("expected no differences after promotion, got: %s", out)
			}
			return ctx
		}).
		Assess("sandbox destroy cleans up namespace", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "chorister", "sandbox", "destroy",
				"--domain", domain, "--name", sandboxName, "--app", appName)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("sandbox destroy failed: %v: %s", err, out)
			}
			// Wait for sandbox namespace to be deleted
			if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
				exists, err := namespaceExists(ctx, cfg, sandboxNS)
				return !exists, err
			}); err != nil {
				t.Fatalf("sandbox namespace %s not cleaned up: %v", sandboxNS, err)
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

// extractPromotionID extracts a promotion request name from JSON or tabular CLI output.
func extractPromotionID(output string) string {
	// Look for a name like "chopromotionrequest-..." or a name field in JSON
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		// JSON: "Name": "..." or "name": "..."
		lower := strings.ToLower(line)
		if strings.Contains(lower, `"name"`) {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				val := strings.Trim(strings.TrimSpace(parts[1]), `",`)
				if val != "" {
					return val
				}
			}
		}
		// Table: first column might be the name
		if strings.HasPrefix(line, "cho") || strings.Contains(line, "promotion") {
			fields := strings.Fields(line)
			if len(fields) > 0 && !strings.HasPrefix(fields[0], "NAME") {
				return fields[0]
			}
		}
	}
	return ""
}
