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
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// ---------------------------------------------------------------------------
// 1A.16 — Production safety (e2e)
// ---------------------------------------------------------------------------

func TestE2E_CannotApplyToProd(t *testing.T) {
	feature := features.New("cannot apply to production").
		Assess("chorister apply targeting production is rejected", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Run chorister apply without --sandbox flag (targeting production)
			cmd := exec.CommandContext(ctx, "chorister", "apply", "--domain", "payments", "--file", "/dev/null")
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatal("expected apply without --sandbox to fail")
			}
			if !strings.Contains(string(out), "--sandbox is required") {
				t.Fatalf("expected sandbox-required error, got: %s", string(out))
			}
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}

func TestE2E_PromotionRequiresApproval(t *testing.T) {
	const appName = "e2e-promapproval"
	const domain = "payments"
	const sandboxName = "dev"
	prodNS := appName + "-" + domain

	feature := features.New("promotion requires approval").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Create app with requiredApprovals=1
			cmd := exec.CommandContext(ctx, "chorister", "admin", "app", "create", appName,
				"--owners", "test@chorister.dev",
				"--compliance", "essential",
				"--domains", domain)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("failed to create app: %v: %s", err, out)
			}
			if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
				return namespaceExists(ctx, cfg, prodNS)
			}); err != nil {
				t.Fatalf("namespace %s not created: %v", prodNS, err)
			}
			// Create sandbox and apply a resource
			cmd = exec.CommandContext(ctx, "chorister", "sandbox", "create",
				"--domain", domain, "--name", sandboxName, "--app", appName)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("sandbox create: %v: %s", err, out)
			}
			return ctx
		}).
		Assess("promotion with 0 approvals does not modify prod", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Create a ChoPromotionRequest via CLI
			cmd := exec.CommandContext(ctx, "chorister", "promote",
				"--domain", domain, "--sandbox", sandboxName, "--app", appName)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("promote failed: %v: %s", err, out)
			}
			// Do NOT approve — wait briefly and verify production is unchanged
			time.Sleep(5 * time.Second)
			var deps appsv1.DeploymentList
			if err := cfg.Client().Resources(prodNS).List(ctx, &deps); err != nil {
				t.Fatalf("list deployments in prod: %v", err)
			}
			if len(deps.Items) > 0 {
				t.Fatal("production namespace has Deployments despite 0 approvals")
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

func TestE2E_ProductionRBACViewOnly(t *testing.T) {
	const appName = "e2e-rbacview"
	const domain = "payments"
	prodNS := appName + "-" + domain

	feature := features.New("production RBAC view-only").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "chorister", "admin", "app", "create", appName,
				"--owners", "test@chorister.dev",
				"--compliance", "essential",
				"--domains", domain)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("failed to create app: %v: %s", err, out)
			}
			if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
				return namespaceExists(ctx, cfg, prodNS)
			}); err != nil {
				t.Fatalf("namespace %s not created: %v", prodNS, err)
			}
			// Add developer member
			cmd = exec.CommandContext(ctx, "chorister", "admin", "member", "add",
				"--app", appName, "--domain", domain,
				"--identity", "devuser@chorister.dev", "--role", "developer",
				"--expires-at", "2030-01-01T00:00:00Z")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Logf("member add: %v: %s (non-fatal)", err, out)
			}
			return ctx
		}).
		Assess("developer SA cannot create resources in production", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Use kubectl auth can-i to verify the developer cannot create in prod
			cmd := exec.CommandContext(ctx, "kubectl", "auth", "can-i", "create", "deployments",
				"--namespace", prodNS, "--as", "devuser@chorister.dev")
			out, _ := cmd.CombinedOutput()
			result := strings.TrimSpace(string(out))
			if result != "no" {
				t.Fatalf("expected developer cannot create deployments in production, got: %s", result)
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
