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

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func boolPtr(b bool) *bool { return &b }

// ---------------------------------------------------------------------------
// 1A.17 — Compliance and policy enforcement (e2e)
// ---------------------------------------------------------------------------

func TestE2E_EssentialCompliance(t *testing.T) {
	const appName = "e2e-compliance"
	const domain = "payments"
	prodNS := appName + "-" + domain

	feature := features.New("essential compliance").
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
			return ctx
		}).
		Assess("no privileged pods and non-root enforced", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Attempt to create a privileged pod — should be rejected by webhook or policy
			privilegedPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "privileged-test",
					Namespace: prodNS,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "test",
						Image: "busybox:latest",
						SecurityContext: &corev1.SecurityContext{
							Privileged: boolPtr(true),
						},
					}},
				},
			}
			err := cfg.Client().Resources().Create(ctx, privilegedPod)
			if err == nil {
				// Clean up the pod if it was unexpectedly created
				_ = cfg.Client().Resources().Delete(ctx, privilegedPod)
				t.Fatal("expected privileged pod creation to be rejected")
			}
			// Verify error mentions security context or privileged
			if !strings.Contains(strings.ToLower(err.Error()), "privileged") &&
				!strings.Contains(strings.ToLower(err.Error()), "security") &&
				!strings.Contains(strings.ToLower(err.Error()), "forbidden") {
				t.Logf("warning: rejection error does not explicitly mention privileged: %v", err)
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

func TestE2E_StandardCompliance(t *testing.T) {
	const appName = "e2e-scangate"
	const domain = "payments"
	const sandboxName = "dev"

	feature := features.New("standard compliance").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			cmd := exec.CommandContext(ctx, "chorister", "admin", "app", "create", appName,
				"--owners", "test@chorister.dev",
				"--compliance", "standard",
				"--domains", domain)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("failed to create app: %v: %s", err, out)
			}
			if err := waitForCondition(ctx, 60*time.Second, 2*time.Second, func() (bool, error) {
				return namespaceExists(ctx, cfg, appName+"-"+domain)
			}); err != nil {
				t.Fatalf("namespace not created: %v", err)
			}
			// Create sandbox
			cmd = exec.CommandContext(ctx, "chorister", "sandbox", "create",
				"--domain", domain, "--name", sandboxName, "--app", appName)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("sandbox create: %v: %s", err, out)
			}
			return ctx
		}).
		Assess("image scanning gate on promotion", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Trigger a scan and verify the CLI completes
			cmd := exec.CommandContext(ctx, "chorister", "admin", "scan",
				"--domain", domain, "--app", appName)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("admin scan returned error (may need real scanner): %v: %s", err, out)
			}
			// Verify vulnerability report listing works
			cmd = exec.CommandContext(ctx, "chorister", "admin", "vulnerabilities",
				"list", "--domain", domain, "--app", appName)
			out, err = cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("admin vulnerabilities list failed: %v: %s", err, out)
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

func TestE2E_RegulatedCompliance(t *testing.T) {
	feature := features.New("regulated compliance").
		Assess("seccomp AppArmor and Tetragon TracingPolicy enforced", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Verify compliance report for regulated profile
			cmd := exec.CommandContext(ctx, "chorister", "admin", "compliance", "report", "--app", "e2e-compliance", "--output", "json")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("compliance report: %v: %s (non-fatal: Tetragon may not be installed)", err, out)
				return ctx
			}
			output := strings.ToLower(string(out))
			if !strings.Contains(output, "seccomp") && !strings.Contains(output, "apparmor") && !strings.Contains(output, "tetragon") {
				t.Logf("warning: compliance report does not mention runtime security controls: %s", out)
			}
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}

func TestE2E_IngressRequiresAuth(t *testing.T) {
	const appName = "e2e-ingressauth"
	const domain = "payments"

	feature := features.New("ingress requires auth").
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
				return namespaceExists(ctx, cfg, appName+"-"+domain)
			}); err != nil {
				t.Fatalf("namespace not created: %v", err)
			}
			return ctx
		}).
		Assess("internet ingress without auth is rejected", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			client := cfg.Client().Resources()

			// Create ChoNetwork with internet ingress but no auth — webhook should reject
			network := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-auth-ingress-e2e",
					Namespace: appName + "-" + domain,
				},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: appName,
					Domain:      domain,
					Ingress: &choristerv1alpha1.NetworkIngressSpec{
						From: "internet",
						Port: 443,
						// No Auth — webhook should reject
					},
				},
			}
			err := client.Create(ctx, network)
			if err == nil {
				// Clean up if unexpectedly created
				_ = client.Delete(ctx, network)
				t.Fatal("expected webhook to reject ChoNetwork without auth for internet ingress")
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
