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

	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// ---------------------------------------------------------------------------
// 1A.14 — Network isolation (e2e, Kind+Cilium)
// ---------------------------------------------------------------------------

func TestE2E_NetworkIsolation(t *testing.T) {
	const appName = "e2e-netiso"
	paymentsNS := appName + "-payments"
	authNS := appName + "-auth"

	feature := features.New("network isolation").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Verify Cilium is installed
			cmd := exec.CommandContext(ctx, "kubectl", "get", "daemonset", "-n", "kube-system", "cilium")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Skipf("Cilium not installed, skipping network isolation tests: %v: %s", err, out)
			}

			// Create app with payments (consumes auth:8080) and auth (supplies :8080)
			cmd = exec.CommandContext(ctx, "kubectl", "apply", "-f", "testdata/netiso-app.yaml")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("failed to create app: %v: %s", err, out)
			}
			if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
				return namespaceExists(ctx, cfg, paymentsNS)
			}); err != nil {
				t.Fatalf("namespace %s not created: %v", paymentsNS, err)
			}
			if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
				return namespaceExists(ctx, cfg, authNS)
			}); err != nil {
				t.Fatalf("namespace %s not created: %v", authNS, err)
			}
			return ctx
		}).
		Assess("payments can reach auth on declared port", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Deploy test pods using the apply manifest helper
			applyManifest(ctx, t, cfg, "testdata/netiso-payments-pod.yaml")
			applyManifest(ctx, t, cfg, "testdata/netiso-auth-pod.yaml")

			// Wait for both pods to be ready
			for _, ns := range []string{paymentsNS, authNS} {
				if err := waitForCondition(ctx, 90*time.Second, 3*time.Second, func() (bool, error) {
					cmd := exec.CommandContext(ctx, "kubectl", "wait", "pod/echo-api",
						"-n", ns, "--for=condition=Ready", "--timeout=5s")
					return cmd.Run() == nil, nil
				}); err != nil {
					t.Fatalf("pod echo-api in %s not ready: %v", ns, err)
				}
			}

			// Test connectivity from payments to auth on port 8080
			cmd := exec.CommandContext(ctx, "kubectl", "exec", "echo-api", "-n", paymentsNS,
				"--",
				"timeout", "5", "wget", "-qO-", "--timeout=5",
				"http://echo-api."+authNS+".svc.cluster.local:8080/healthz")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("payments cannot reach auth on port 8080: %v: %s", err, out)
			}
			return ctx
		}).
		Assess("payments cannot reach auth on wrong port", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "kubectl", "exec", "echo-api", "-n", paymentsNS,
				"--",
				"timeout", "3", "wget", "-qO-", "--timeout=3",
				"http://echo-api."+authNS+".svc.cluster.local:9090/healthz")
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("expected port 9090 to be blocked, but got response: %s", out)
			}
			return ctx
		}).
		Assess("unrelated namespace cannot reach auth", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Create an unrelated namespace with a test pod
			unrelatedNS := appName + "-unrelated"
			createNamespace(ctx, t, cfg, unrelatedNS)
			applyManifest(ctx, t, cfg, "testdata/netiso-unrelated-pod.yaml")

			if err := waitForCondition(ctx, 60*time.Second, 3*time.Second, func() (bool, error) {
				cmd := exec.CommandContext(ctx, "kubectl", "wait", "pod/echo-api",
					"-n", unrelatedNS, "--for=condition=Ready", "--timeout=5s")
				return cmd.Run() == nil, nil
			}); err != nil {
				t.Fatalf("unrelated pod not ready: %v", err)
			}

			cmd := exec.CommandContext(ctx, "kubectl", "exec", "echo-api", "-n", unrelatedNS,
				"--",
				"timeout", "3", "wget", "-qO-", "--timeout=3",
				"http://echo-api."+authNS+".svc.cluster.local:8080/healthz")
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("expected unrelated namespace to be blocked, but got: %s", out)
			}
			// Best-effort async cleanup of unrelated namespace
			exec.CommandContext(ctx, "kubectl", "delete", "namespace", unrelatedNS, "--ignore-not-found", "--wait=false").Run() //nolint:errcheck
			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cleanupApp(ctx, t, appName)
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}

// ---------------------------------------------------------------------------
// 1A.15 — Cross-application link flow (e2e, Kind+Cilium)
// ---------------------------------------------------------------------------

func TestE2E_CrossApplicationLink(t *testing.T) {
	const retailApp = "e2e-retail"
	const capitalApp = "e2e-capital"
	retailPaymentsNS := retailApp + "-payments"
	capitalPricingNS := capitalApp + "-pricing"

	feature := features.New("cross-application link").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Verify Cilium and Gateway API are available
			cmd := exec.CommandContext(ctx, "kubectl", "get", "daemonset", "-n", "kube-system", "cilium")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Skipf("Cilium not installed: %v: %s", err, out)
			}
			cmd = exec.CommandContext(ctx, "kubectl", "get", "crd", "gateways.gateway.networking.k8s.io")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Skipf("Gateway API CRDs not installed: %v: %s", err, out)
			}

			// Create two applications
			cmd = exec.CommandContext(ctx, "chorister", "admin", "app", "create", retailApp,
				"--owners", "test@chorister.dev",
				"--compliance", "essential",
				"--domains", "payments")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("create retail app: %v: %s", err, out)
			}
			cmd = exec.CommandContext(ctx, "chorister", "admin", "app", "create", capitalApp,
				"--owners", "test@chorister.dev",
				"--compliance", "essential",
				"--domains", "pricing")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("create capital app: %v: %s", err, out)
			}

			for _, ns := range []string{retailPaymentsNS, capitalPricingNS} {
				if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
					return namespaceExists(ctx, cfg, ns)
				}); err != nil {
					t.Fatalf("namespace %s not created: %v", ns, err)
				}
			}
			return ctx
		}).
		Assess("direct pod-to-pod cross-application traffic is blocked", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Deploy test pods
			applyManifest(ctx, t, cfg, "testdata/crossapp-retail-pod.yaml")
			applyManifest(ctx, t, cfg, "testdata/crossapp-capital-pod.yaml")

			// Wait for pods
			for _, ns := range []string{retailPaymentsNS, capitalPricingNS} {
				if err := waitForCondition(ctx, 90*time.Second, 3*time.Second, func() (bool, error) {
					cmd := exec.CommandContext(ctx, "kubectl", "wait", "pod/echo-api",
						"-n", ns, "--for=condition=Ready", "--timeout=5s")
					return cmd.Run() == nil, nil
				}); err != nil {
					t.Fatalf("pod echo-api in %s not ready: %v", ns, err)
				}
			}

			// Get pricing pod IP
			cmd := exec.CommandContext(ctx, "kubectl", "get", "pods", "-n", capitalPricingNS,
				"-l", "app=echo-api", "-o", "jsonpath={.items[0].status.podIP}")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("get pricing pod IP: %v: %s", err, out)
			}
			podIP := strings.TrimSpace(string(out))

			// Direct pod-to-pod should be blocked
			cmd = exec.CommandContext(ctx, "kubectl", "exec", "echo-api", "-n", retailPaymentsNS,
				"--",
				"timeout", "3", "wget", "-qO-", "--timeout=3",
				"http://"+podIP+":8080/healthz")
			if _, err := cmd.CombinedOutput(); err == nil {
				t.Fatal("expected direct pod-to-pod cross-app traffic to be blocked")
			}
			return ctx
		}).
		Assess("HTTPRoute and ReferenceGrant are present", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Check for HTTPRoute in retail-payments
			cmd := exec.CommandContext(ctx, "kubectl", "get", "httproutes", "-n", retailPaymentsNS, "--no-headers")
			out, err := cmd.CombinedOutput()
			if err != nil || len(strings.TrimSpace(string(out))) == 0 {
				t.Fatalf("no HTTPRoute found in %s: %v: %s", retailPaymentsNS, err, out)
			}
			// Check for ReferenceGrant in capital-pricing
			cmd = exec.CommandContext(ctx, "kubectl", "get", "referencegrants", "-n", capitalPricingNS, "--no-headers")
			out, err = cmd.CombinedOutput()
			if err != nil || len(strings.TrimSpace(string(out))) == 0 {
				t.Fatalf("no ReferenceGrant found in %s: %v: %s", capitalPricingNS, err, out)
			}
			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cleanupApp(ctx, t, retailApp)
			cleanupApp(ctx, t, capitalApp)
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}
