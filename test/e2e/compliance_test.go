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
	"testing"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// ---------------------------------------------------------------------------
// 1A.17 — Compliance and policy enforcement (e2e)
// ---------------------------------------------------------------------------

func TestE2E_EssentialCompliance(t *testing.T) {
	feature := features.New("essential compliance").
		Assess("no privileged pods and non-root enforced", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Skip("awaiting Phase 10: Gatekeeper constraints + compliance profiles")

			// Attempt to create privileged pod → rejected
			// Attempt to create pod running as root → rejected
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}

func TestE2E_StandardCompliance(t *testing.T) {
	feature := features.New("standard compliance").
		Assess("image scanning gate on promotion", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Skip("awaiting Phase 14: image scanning before promotion")

			// Image with known CVE → promotion blocked
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}

func TestE2E_RegulatedCompliance(t *testing.T) {
	feature := features.New("regulated compliance").
		Assess("seccomp AppArmor and Tetragon TracingPolicy enforced", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Skip("e2e: requires full Tetragon + Cilium in Kind cluster — covered by envtest unit tests")

			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}

func TestE2E_IngressRequiresAuth(t *testing.T) {
	feature := features.New("ingress requires auth").
		Assess("internet ingress without auth is rejected", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			client := cfg.Client().Resources()

			// Create ChoNetwork with internet ingress but no auth — webhook should reject
			network := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-auth-ingress-e2e",
					Namespace: cfg.Namespace(),
				},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: "myapp",
					Domain:      "payments",
					Ingress: &choristerv1alpha1.NetworkIngressSpec{
						From: "internet",
						Port: 443,
						// No Auth — webhook should reject
					},
				},
			}
			err := client.Create(ctx, network)
			if err == nil {
				t.Fatal("expected webhook to reject ChoNetwork without auth for internet ingress")
			}ould reject
			network := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-auth-ingress-e2e",
					Namespace: cfg.Namespace(),
				},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: "myapp",
					Domain:      "payments",
					Ingress: &choristerv1alpha1.NetworkIngressSpec{
						From: "internet",
						Port: 443,
						// No Auth — webhook should reject
					},
				},
			}
			err := client.Create(ctx, network)
			if err == nil {
				t.Fatal("expected webhook to reject ChoNetwork without auth for internet ingress")
			}
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}
