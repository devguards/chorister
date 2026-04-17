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
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// ---------------------------------------------------------------------------
// Multi-cluster e2e tests
//
// These tests verify the multi-cluster workflow:
//   - Cluster registration via ChoCluster CRD
//   - Controller processes registered clusters and reports connectivity
//   - Sandboxes target the sandbox-role cluster
//   - Promotion copies resources from sandbox cluster to production cluster
//   - CLI uses its own config, independent of kubectl context
//
// Expected TDD state: ALL FAIL (multi-cluster not implemented yet).
// ---------------------------------------------------------------------------

// TestE2E_MultiCluster_ChoClusterAcceptsClusters verifies that the ChoCluster
// CRD accepts the clusters field. This validates the CRD schema change.
func TestE2E_MultiCluster_ChoClusterAcceptsClusters(t *testing.T) {
	const clusterName = "e2e-mc-crdtest"

	feature := features.New("ChoCluster accepts clusters field").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Create a kubeconfig Secret (placeholder — content doesn't matter for CRD acceptance).
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sandbox-pool-kubeconfig",
					Namespace: "cho-system",
				},
				Data: map[string][]byte{
					"kubeconfig": []byte("placeholder-kubeconfig"),
				},
			}
			if err := cfg.Client().Resources().Create(ctx, secret); err != nil {
				t.Logf("secret create (may already exist): %v", err)
			}
			return ctx
		}).
		Assess("create ChoCluster with clusters field", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Apply a ChoCluster with clusters via kubectl.
			manifest := `
apiVersion: chorister.dev/v1alpha1
kind: ChoCluster
metadata:
  name: ` + clusterName + `
spec:
  clusters:
    - name: sandbox-pool
      role: sandbox
      secretRef: sandbox-pool-kubeconfig
    - name: prod-cell-1
      role: production
      secretRef: prod-cell-1-kubeconfig
`
			cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(manifest)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("kubectl apply ChoCluster with clusters: %v: %s", err, out)
			}
			return ctx
		}).
		Assess("clusters field persisted in CRD", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "kubectl", "get", "chocluster", clusterName, "-o", "json")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("kubectl get ChoCluster: %v: %s", err, out)
			}

			var result map[string]interface{}
			if err := json.Unmarshal(out, &result); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			spec, ok := result["spec"].(map[string]interface{})
			if !ok {
				t.Fatal("missing spec in ChoCluster")
			}

			clusters, ok := spec["clusters"].([]interface{})
			if !ok || len(clusters) != 2 {
				t.Fatalf("expected 2 clusters in spec.clusters, got: %v", spec["clusters"])
			}
			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "kubectl", "delete", "chocluster", clusterName, "--ignore-not-found")
			_, _ = cmd.CombinedOutput()
			cmd = exec.CommandContext(ctx, "kubectl", "delete", "secret", "sandbox-pool-kubeconfig", "-n", "cho-system", "--ignore-not-found")
			_, _ = cmd.CombinedOutput()
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}

// TestE2E_MultiCluster_ControllerReportsClusterConnectivity verifies that the
// controller processes registered clusters and reports their connectivity
// in ChoCluster.status.clusterConnectivity.
func TestE2E_MultiCluster_ControllerReportsClusterConnectivity(t *testing.T) {
	const clusterName = "e2e-mc-connectivity"

	feature := features.New("controller reports cluster connectivity").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Create kubeconfig Secret (placeholder).
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-sandbox-kubeconfig",
					Namespace: "cho-system",
				},
				Data: map[string][]byte{
					"kubeconfig": []byte("placeholder-kubeconfig"),
				},
			}
			if err := cfg.Client().Resources().Create(ctx, secret); err != nil {
				t.Logf("secret create: %v", err)
			}

			// Create ChoCluster with one registered cluster.
			manifest := `
apiVersion: chorister.dev/v1alpha1
kind: ChoCluster
metadata:
  name: ` + clusterName + `
spec:
  clusters:
    - name: test-sandbox
      role: sandbox
      secretRef: test-sandbox-kubeconfig
`
			cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-")
			cmd.Stdin = strings.NewReader(manifest)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("apply ChoCluster: %v: %s", err, out)
			}
			return ctx
		}).
		Assess("status.clusterConnectivity populated", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			var lastOutput string
			err := waitForCondition(ctx, 60*time.Second, 3*time.Second, func() (bool, error) {
				cmd := exec.CommandContext(ctx, "kubectl", "get", "chocluster", clusterName, "-o", "json")
				out, err := cmd.CombinedOutput()
				if err != nil {
					return false, err
				}
				lastOutput = string(out)

				var result map[string]interface{}
				if err := json.Unmarshal(out, &result); err != nil {
					return false, err
				}

				status, ok := result["status"].(map[string]interface{})
				if !ok {
					return false, nil
				}

				connectivity, ok := status["clusterConnectivity"].(map[string]interface{})
				if !ok || len(connectivity) == 0 {
					return false, nil
				}

				// At least "test-sandbox" should have a status.
				_, exists := connectivity["test-sandbox"]
				return exists, nil
			})
			if err != nil {
				t.Fatalf("controller did not report clusterConnectivity for test-sandbox within timeout.\nLast output: %s", lastOutput)
			}
			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "kubectl", "delete", "chocluster", clusterName, "--ignore-not-found")
			_, _ = cmd.CombinedOutput()
			cmd = exec.CommandContext(ctx, "kubectl", "delete", "secret", "test-sandbox-kubeconfig", "-n", "cho-system", "--ignore-not-found")
			_, _ = cmd.CombinedOutput()
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}

// TestE2E_MultiCluster_SandboxOnSandboxCluster verifies that when a sandbox-role
// cluster is registered, sandbox namespaces are created on that cluster (not the home cluster).
func TestE2E_MultiCluster_SandboxOnSandboxCluster(t *testing.T) {
	const appName = "e2e-mc-sandbox"
	const domain = "payments"
	const sandboxName = "alice"
	sandboxNS := appName + "-" + domain + "-sandbox-" + sandboxName

	feature := features.New("sandbox targets sandbox cluster").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Create app.
			cmd := exec.CommandContext(ctx, "chorister", "admin", "app", "create", appName,
				"--owners", "test@chorister.dev",
				"--compliance", "essential",
				"--domains", domain)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("create app: %v: %s", err, out)
			}

			prodNS := appName + "-" + domain
			if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
				return namespaceExists(ctx, cfg, prodNS)
			}); err != nil {
				t.Fatalf("domain namespace not created: %v", err)
			}
			return ctx
		}).
		Assess("sandbox create targets sandbox cluster", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "chorister", "sandbox", "create",
				"--domain", domain, "--name", sandboxName, "--app", appName)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("sandbox create: %v: %s", err, out)
			}

			// In multi-cluster mode, the sandbox namespace should be created on the
			// SANDBOX cluster, not the home cluster. We verify this by checking that
			// the namespace does NOT exist on the home cluster.
			//
			// NOTE: With single-cluster fallback, the namespace IS on the home cluster.
			// This test fails until multi-cluster routing is implemented.
			if err := waitForCondition(ctx, 30*time.Second, 2*time.Second, func() (bool, error) {
				return namespaceExists(ctx, cfg, sandboxNS)
			}); err == nil {
				// Namespace exists on home cluster — multi-cluster routing not active.
				t.Fatal("sandbox namespace created on home cluster; expected it on the registered sandbox cluster")
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

// TestE2E_MultiCluster_PromotionToProductionCluster verifies that promoting a
// sandbox creates resources on the production-role cluster.
func TestE2E_MultiCluster_PromotionToProductionCluster(t *testing.T) {
	const appName = "e2e-mc-promote"
	const domain = "payments"
	const sandboxName = "dev"
	prodNS := appName + "-" + domain

	feature := features.New("promotion targets production cluster").
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
				t.Fatalf("domain namespace not created: %v", err)
			}

			// Create sandbox and apply resources.
			cmd = exec.CommandContext(ctx, "chorister", "sandbox", "create",
				"--domain", domain, "--name", sandboxName, "--app", appName)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("sandbox create: %v: %s", err, out)
			}

			sandboxNS := appName + "-" + domain + "-sandbox-" + sandboxName
			if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
				return namespaceExists(ctx, cfg, sandboxNS)
			}); err != nil {
				t.Fatalf("sandbox namespace not created: %v", err)
			}

			cmd = exec.CommandContext(ctx, "chorister", "apply",
				"--domain", domain, "--sandbox", sandboxName, "--app", appName,
				"--file", "testdata/devflow-resources.yaml")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("apply: %v: %s", err, out)
			}
			return ctx
		}).
		Assess("promote and approve routes to production cluster", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Promote.
			cmd := exec.CommandContext(ctx, "chorister", "promote",
				"--domain", domain, "--sandbox", sandboxName, "--app", appName)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("promote: %v: %s", err, out)
			}

			// Find and approve the promotion request.
			cmd = exec.CommandContext(ctx, "chorister", "requests",
				"--domain", domain, "--app", appName, "--status", "pending", "--output", "json")
			out, err = cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("requests: %v: %s", err, out)
			}
			reqID := extractPromotionID(string(out))
			if reqID == "" {
				t.Fatalf("no promotion request found in: %s", out)
			}

			cmd = exec.CommandContext(ctx, "chorister", "approve", reqID)
			if out, err = cmd.CombinedOutput(); err != nil {
				t.Fatalf("approve: %v: %s", err, out)
			}

			// In multi-cluster mode, resources should appear on the PRODUCTION cluster,
			// not the home cluster. We check the home cluster — if resources appear here,
			// multi-cluster routing is not active.
			//
			// Wait for Deployment to appear (or not) in production namespace.
			if err := waitForCondition(ctx, 120*time.Second, 3*time.Second, func() (bool, error) {
				var deps appsv1.DeploymentList
				if err := cfg.Client().Resources(prodNS).List(ctx, &deps); err != nil {
					return false, err
				}
				return len(deps.Items) > 0, nil
			}); err == nil {
				// Resources on home cluster → single-cluster mode, not multi-cluster.
				t.Fatal("promoted resources appeared on home cluster; expected them on the registered production cluster")
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

// TestE2E_MultiCluster_CLIUsesHomeConfig verifies that the chorister CLI reads
// its endpoint from ~/.config/chorister/config rather than from kubectl context.
func TestE2E_MultiCluster_CLIUsesHomeConfig(t *testing.T) {
	feature := features.New("CLI uses chorister config, not kubectl context").
		Assess("chorister commands ignore kubectl context", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// The CLI should read from its own config file (~/.config/chorister/config),
			// not from kubectl's current-context. This test verifies that the CLI
			// has a --home-cluster or config file mechanism.

			// Try using chorister with an explicit home-cluster flag.
			cmd := exec.CommandContext(ctx, "chorister", "status", "--home-cluster", "https://localhost:6443")
			out, err := cmd.CombinedOutput()
			output := string(out)

			// If --home-cluster flag doesn't exist, the CLI hasn't been updated for multi-cluster.
			if err != nil && strings.Contains(output, "unknown flag") {
				t.Fatalf("CLI does not support --home-cluster flag: multi-cluster CLI not implemented. Output: %s", output)
			}

			// If we get here, the flag exists. The command might fail for other reasons
			// (auth, connectivity) but the flag itself is recognized.
			_ = output
			return ctx
		}).
		Assess("chorister login creates config file", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// chorister login should support storing the home cluster endpoint.
			cmd := exec.CommandContext(ctx, "chorister", "login", "--help")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("chorister login --help: %v: %s", err, out)
			}
			output := string(out)

			// The login command should mention server/endpoint configuration.
			if !strings.Contains(output, "server") && !strings.Contains(output, "endpoint") && !strings.Contains(output, "cluster") {
				t.Fatalf("chorister login help does not mention server/endpoint/cluster config. Output: %s", output)
			}
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}

// TestE2E_MultiCluster_StatusAcrossClusters verifies that `chorister status`
// shows health from all registered clusters, not just the home cluster.
func TestE2E_MultiCluster_StatusAcrossClusters(t *testing.T) {
	const appName = "e2e-mc-status"
	const domain = "payments"

	feature := features.New("status shows cross-cluster view").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "chorister", "admin", "app", "create", appName,
				"--owners", "test@chorister.dev",
				"--compliance", "essential",
				"--domains", domain)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("create app: %v: %s", err, out)
			}
			prodNS := appName + "-" + domain
			if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
				return namespaceExists(ctx, cfg, prodNS)
			}); err != nil {
				t.Fatalf("namespace not created: %v", err)
			}
			return ctx
		}).
		Assess("status shows cluster assignments", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "chorister", "status", "--domain", domain, "--app", appName)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("chorister status: %v: %s", err, out)
			}
			output := string(out)

			// In multi-cluster mode, status should show which cluster each environment
			// is running on. Look for cluster name references.
			if !strings.Contains(output, "cluster") && !strings.Contains(output, "Cluster") {
				t.Fatalf("chorister status does not show cluster assignments. Multi-cluster status not implemented. Output: %s", output)
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
