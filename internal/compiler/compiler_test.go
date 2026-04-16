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

package compiler

import (
	"fmt"
	"testing"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ---------------------------------------------------------------------------
// 1A.1 — Compilation unit tests
// ---------------------------------------------------------------------------

// Skipped compiler tests for compute, database, queue, cache, and storage PVC
// were removed — those resources are handled by direct reconcilers, not the compiler.

// --- ChoNetwork compilation ---

func TestCompileNetwork_IngressHTTPRoute(t *testing.T) {
	network := &choristerv1alpha1.ChoNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: "public-api", Namespace: "myapp-payments"},
		Spec: choristerv1alpha1.ChoNetworkSpec{
			Application: "myapp",
			Domain:      "payments",
			Ingress: &choristerv1alpha1.NetworkIngressSpec{
				From: "internet",
				Port: 443,
				Auth: &choristerv1alpha1.NetworkAuthSpec{
					JWT: &choristerv1alpha1.JWTAuthSpec{
						JWKSUri:  "https://auth.example.com/.well-known/jwks.json",
						Issuer:   "https://auth.example.com",
						Audience: []string{"payments-api"},
					},
				},
			},
		},
	}

	route := CompileIngressHTTPRoute(network)
	if route.GetKind() != "HTTPRoute" {
		t.Fatalf("expected HTTPRoute, got %s", route.GetKind())
	}
	if route.GetName() != "public-api-ingress" {
		t.Fatalf("expected route name public-api-ingress, got %s", route.GetName())
	}
	if route.GetAnnotations()["chorister.dev/jwt-issuer"] != "https://auth.example.com" {
		t.Fatalf("expected JWT issuer annotation, got %v", route.GetAnnotations())
	}
	spec := getSpec(t, route)
	rules := spec["rules"].([]any)
	backendRefs := rules[0].(map[string]any)["backendRefs"].([]any)
	if backendRefs[0].(map[string]any)["port"].(int) != 443 {
		t.Fatalf("expected backend port 443, got %v", backendRefs[0])
	}
}

func TestCompileNetwork_EgressCiliumPolicy(t *testing.T) {
	network := &choristerv1alpha1.ChoNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: "external", Namespace: "myapp-payments"},
		Spec: choristerv1alpha1.ChoNetworkSpec{
			Application: "myapp",
			Domain:      "payments",
			Egress: &choristerv1alpha1.NetworkEgressSpec{
				Allowlist: []string{"api.stripe.com"},
			},
		},
	}

	policy := CompileEgressCiliumPolicy(network)
	if policy.GetKind() != "CiliumNetworkPolicy" {
		t.Fatalf("expected CiliumNetworkPolicy, got %s", policy.GetKind())
	}
	spec := getSpec(t, policy)
	egress := spec["egress"].([]any)
	if len(egress) < 2 {
		t.Fatalf("expected DNS plus allowlist rules, got %d", len(egress))
	}
	allowRule := egress[1].(map[string]any)
	toFQDNs := allowRule["toFQDNs"].([]any)
	if toFQDNs[0].(map[string]any)["matchName"] != "api.stripe.com" {
		t.Fatalf("expected api.stripe.com allowlist, got %v", toFQDNs)
	}
}

func TestCompileNetwork_CrossApplicationLink(t *testing.T) {
	app := &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "consumer-app"},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Owners: []string{"owner@example.com"},
			Policy: choristerv1alpha1.ApplicationPolicy{
				Compliance: "standard",
				Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"org-admin"}},
			},
			Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}},
		},
	}
	link := choristerv1alpha1.LinkSpec{
		Name:           "auth-api",
		Target:         "platform-app",
		TargetDomain:   "auth",
		Port:           8443,
		Consumers:      []string{"payments"},
		Auth:           &choristerv1alpha1.LinkAuth{Type: "jwt"},
		RateLimit:      &choristerv1alpha1.LinkRateLimit{RequestsPerMinute: 120},
		CircuitBreaker: &choristerv1alpha1.LinkCircuitBreaker{ConsecutiveErrors: 5},
	}

	artifacts := CompileCrossApplicationLink(app, link, "payments")
	if artifacts.HTTPRoute == nil || artifacts.ReferenceGrant == nil || artifacts.CiliumEnvoyConfig == nil || artifacts.CiliumPolicy == nil || artifacts.DirectDenyPolicy == nil {
		t.Fatal("expected all link artifacts to be generated")
	}
	if artifacts.HTTPRoute.GetNamespace() != "consumer-app-payments" {
		t.Fatalf("expected consumer namespace, got %s", artifacts.HTTPRoute.GetNamespace())
	}
	if artifacts.ReferenceGrant.GetNamespace() != "platform-app-auth" {
		t.Fatalf("expected supplier namespace, got %s", artifacts.ReferenceGrant.GetNamespace())
	}
	if artifacts.CiliumEnvoyConfig.GetAnnotations()["chorister.dev/link-auth-type"] != "jwt" {
		t.Fatalf("expected link auth annotation, got %v", artifacts.CiliumEnvoyConfig.GetAnnotations())
	}
	if artifacts.DirectDenyPolicy.Name == "" {
		t.Fatal("expected direct deny policy to be named")
	}

	// CiliumEnvoyConfig GVK
	gvk := artifacts.CiliumEnvoyConfig.GroupVersionKind()
	if gvk.Group != "cilium.io" || gvk.Version != "v2" || gvk.Kind != "CiliumEnvoyConfig" {
		t.Fatalf("expected cilium.io/v2/CiliumEnvoyConfig, got %s", gvk)
	}

	// CiliumEnvoyConfig namespace is consumer namespace
	if artifacts.CiliumEnvoyConfig.GetNamespace() != "consumer-app-payments" {
		t.Fatalf("expected consumer namespace for CiliumEnvoyConfig, got %s", artifacts.CiliumEnvoyConfig.GetNamespace())
	}

	// Rate-limit value propagated
	spec := getSpec(t, artifacts.CiliumEnvoyConfig)
	rl, ok := spec["rateLimit"].(map[string]any)
	if !ok {
		t.Fatal("expected rateLimit map in CiliumEnvoyConfig spec")
	}
	if fmt.Sprintf("%v", rl["requestsPerMinute"]) != "120" {
		t.Fatalf("expected requestsPerMinute=120, got %v", rl["requestsPerMinute"])
	}

	// Circuit-breaker value propagated
	cb, ok := spec["circuitBreaker"].(map[string]any)
	if !ok {
		t.Fatal("expected circuitBreaker map in CiliumEnvoyConfig spec")
	}
	if fmt.Sprintf("%v", cb["consecutiveErrors"]) != "5" {
		t.Fatalf("expected consecutiveErrors=5, got %v", cb["consecutiveErrors"])
	}

	// services reference supplier namespace
	svc, _ := spec["services"].([]any)
	if len(svc) == 0 {
		t.Fatal("expected at least one service entry")
	}
	first := svc[0].(map[string]any)
	if first["namespace"] != "platform-app-auth" {
		t.Fatalf("expected service namespace platform-app-auth, got %v", first["namespace"])
	}
}

func getSpec(t *testing.T, obj *unstructured.Unstructured) map[string]any {
	t.Helper()
	spec, ok := obj.Object["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec map, got %T", obj.Object["spec"])
	}
	return spec
}

// ---------------------------------------------------------------------------
// Phase 15.1 — Restricted domain L7 CiliumNetworkPolicy
// ---------------------------------------------------------------------------

func TestCompileRestrictedDomainL7Policy(t *testing.T) {
	app := &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "myapp"},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Domains: []choristerv1alpha1.DomainSpec{
				{
					Name:        "secrets",
					Sensitivity: "restricted",
					Supplies:    &choristerv1alpha1.SupplySpec{Port: 8443, Services: []string{"grpc"}},
				},
			},
		},
	}

	policy := CompileRestrictedDomainL7Policy(app, app.Spec.Domains[0])
	if policy == nil {
		t.Fatal("expected non-nil policy")
	}

	if policy.GetKind() != "CiliumNetworkPolicy" {
		t.Errorf("expected CiliumNetworkPolicy, got %s", policy.GetKind())
	}
	if policy.GetNamespace() != "myapp-secrets" {
		t.Errorf("expected namespace myapp-secrets, got %s", policy.GetNamespace())
	}
	if policy.GetName() != "secrets-l7-restricted" {
		t.Errorf("expected name secrets-l7-restricted, got %s", policy.GetName())
	}

	spec := getSpec(t, policy)
	ingress, ok := spec["ingress"].([]any)
	if !ok || len(ingress) != 1 {
		t.Fatalf("expected 1 ingress rule, got %v", spec["ingress"])
	}

	rule, ok := ingress[0].(map[string]any)
	if !ok {
		t.Fatal("expected ingress rule to be a map")
	}
	if _, ok := rule["toPorts"]; !ok {
		t.Fatal("expected toPorts in L7 rule")
	}
}

func TestCompileRestrictedDomainL7Policy_DefaultPort(t *testing.T) {
	app := &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "myapp"},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Domains: []choristerv1alpha1.DomainSpec{
				{Name: "data", Sensitivity: "restricted"},
			},
		},
	}

	policy := CompileRestrictedDomainL7Policy(app, app.Spec.Domains[0])
	if policy == nil {
		t.Fatal("expected non-nil policy")
	}
	if policy.GetName() != "data-l7-restricted" {
		t.Errorf("expected data-l7-restricted, got %s", policy.GetName())
	}
}

// ---------------------------------------------------------------------------
// Phase 15.2 — Tetragon TracingPolicy
// ---------------------------------------------------------------------------

func TestCompileTetragonTracingPolicy(t *testing.T) {
	app := &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "secure-app"},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Policy: choristerv1alpha1.ApplicationPolicy{
				Compliance: "regulated",
			},
			Domains: []choristerv1alpha1.DomainSpec{
				{Name: "vault", Sensitivity: "restricted"},
			},
		},
	}

	policy := CompileTetragonTracingPolicy(app, app.Spec.Domains[0])
	if policy == nil {
		t.Fatal("expected non-nil policy")
	}

	if policy.GetKind() != "TracingPolicy" {
		t.Errorf("expected TracingPolicy, got %s", policy.GetKind())
	}
	if policy.GetNamespace() != "secure-app-vault" {
		t.Errorf("expected namespace secure-app-vault, got %s", policy.GetNamespace())
	}
	if policy.GetName() != "vault-runtime-tracing" {
		t.Errorf("expected name vault-runtime-tracing, got %s", policy.GetName())
	}

	labels := policy.GetLabels()
	if labels["chorister.dev/component"] != "runtime-security" {
		t.Errorf("expected runtime-security label, got %s", labels["chorister.dev/component"])
	}

	spec := getSpec(t, policy)
	kprobes, ok := spec["kprobes"].([]any)
	if !ok || len(kprobes) < 2 {
		t.Fatalf("expected at least 2 kprobes, got %v", spec["kprobes"])
	}
}

// ---------------------------------------------------------------------------
// Phase 16.1 — cert-manager Certificate
// ---------------------------------------------------------------------------

func TestCompileCertManagerCertificate(t *testing.T) {
	app := &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "secure-app"},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Domains: []choristerv1alpha1.DomainSpec{
				{Name: "api", Sensitivity: "confidential"},
			},
		},
	}

	cert := CompileCertManagerCertificate(app, app.Spec.Domains[0])
	if cert == nil {
		t.Fatal("expected non-nil certificate")
	}

	if cert.GetKind() != "Certificate" {
		t.Errorf("expected Certificate, got %s", cert.GetKind())
	}
	if cert.GetNamespace() != "secure-app-api" {
		t.Errorf("expected namespace secure-app-api, got %s", cert.GetNamespace())
	}
	if cert.GetName() != "api-tls" {
		t.Errorf("expected name api-tls, got %s", cert.GetName())
	}

	spec := getSpec(t, cert)
	if spec["secretName"] != "api-tls-secret" {
		t.Errorf("expected secretName api-tls-secret, got %v", spec["secretName"])
	}

	issuerRef, ok := spec["issuerRef"].(map[string]any)
	if !ok {
		t.Fatal("expected issuerRef map")
	}
	if issuerRef["name"] != "chorister-cluster-issuer" {
		t.Errorf("expected issuer name chorister-cluster-issuer, got %v", issuerRef["name"])
	}
	if issuerRef["kind"] != "ClusterIssuer" {
		t.Errorf("expected issuer kind ClusterIssuer, got %v", issuerRef["kind"])
	}

	dnsNames, ok := spec["dnsNames"].([]any)
	if !ok || len(dnsNames) != 2 {
		t.Fatalf("expected 2 DNS names, got %v", spec["dnsNames"])
	}
}

// ---------------------------------------------------------------------------
// Phase 16.2 — Cilium encryption policy
// ---------------------------------------------------------------------------

func TestCompileCiliumEncryptionPolicy(t *testing.T) {
	app := &choristerv1alpha1.ChoApplication{
		ObjectMeta: metav1.ObjectMeta{Name: "secure-app"},
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Domains: []choristerv1alpha1.DomainSpec{
				{
					Name:        "api",
					Sensitivity: "restricted",
					Supplies:    &choristerv1alpha1.SupplySpec{Port: 8080, Services: []string{"http"}},
				},
			},
		},
	}

	policy := CompileCiliumEncryptionPolicy(app, app.Spec.Domains[0])
	if policy == nil {
		t.Fatal("expected non-nil policy")
	}

	if policy.GetKind() != "CiliumNetworkPolicy" {
		t.Errorf("expected CiliumNetworkPolicy, got %s", policy.GetKind())
	}
	if policy.GetName() != "api-encryption-policy" {
		t.Errorf("expected api-encryption-policy, got %s", policy.GetName())
	}

	labels := policy.GetLabels()
	if labels["chorister.dev/tls-enforced"] != "true" {
		t.Error("expected tls-enforced label")
	}

	annotations := policy.GetAnnotations()
	if annotations["chorister.dev/encryption"] != "wireguard" {
		t.Error("expected wireguard encryption annotation")
	}

	spec := getSpec(t, policy)
	ingress, ok := spec["ingress"].([]any)
	if !ok || len(ingress) != 1 {
		t.Fatalf("expected 1 ingress rule, got %v", spec["ingress"])
	}

	rule := ingress[0].(map[string]any)
	auth, ok := rule["authentication"].(map[string]any)
	if !ok || auth["mode"] != "required" {
		t.Error("expected authentication mode=required in ingress")
	}

	egress, ok := spec["egress"].([]any)
	if !ok || len(egress) != 1 {
		t.Fatalf("expected 1 egress rule, got %v", spec["egress"])
	}

	egressRule := egress[0].(map[string]any)
	egressAuth, ok := egressRule["authentication"].(map[string]any)
	if !ok || egressAuth["mode"] != "required" {
		t.Error("expected authentication mode=required in egress")
	}
}

// ---------------------------------------------------------------------------
// H.2 / H.3 — Object storage kro RGD compilation
// ---------------------------------------------------------------------------

func TestCompileObjectStorageRGD_S3(t *testing.T) {
	size := resource.MustParse("50Gi")
	storage := &choristerv1alpha1.ChoStorage{
		ObjectMeta: metav1.ObjectMeta{Name: "media-bucket", Namespace: "myapp-media"},
		Spec: choristerv1alpha1.ChoStorageSpec{
			Application:   "myapp",
			Domain:        "media",
			Variant:       "object",
			ObjectBackend: "s3",
			Size:          &size,
		},
	}

	rgd := CompileObjectStorageRGD(storage)
	if rgd == nil {
		t.Fatal("expected non-nil RGD")
	}

	if rgd.GetKind() != "ResourceGraphDefinition" {
		t.Errorf("expected kind ResourceGraphDefinition, got %s", rgd.GetKind())
	}
	if rgd.GroupVersionKind().Group != "kro.run" {
		t.Errorf("expected group kro.run, got %s", rgd.GroupVersionKind().Group)
	}
	if rgd.GetName() != "media-bucket-object-storage" {
		t.Errorf("expected name media-bucket-object-storage, got %s", rgd.GetName())
	}
	if rgd.GetNamespace() != "myapp-media" {
		t.Errorf("expected namespace myapp-media, got %s", rgd.GetNamespace())
	}

	labels := rgd.GetLabels()
	if labels["chorister.dev/application"] != "myapp" {
		t.Errorf("expected application label myapp, got %s", labels["chorister.dev/application"])
	}
	if labels["chorister.dev/variant"] != "object" {
		t.Errorf("expected variant label object, got %s", labels["chorister.dev/variant"])
	}

	spec := getSpec(t, rgd)
	schemaField, ok := spec["schema"].(map[string]any)
	if !ok {
		t.Fatal("expected schema field in spec")
	}
	if schemaField["kind"] != "ObjectStorageClaim" {
		t.Errorf("expected schema kind ObjectStorageClaim, got %v", schemaField["kind"])
	}

	schemaSpec, ok := schemaField["spec"].(map[string]any)
	if !ok {
		t.Fatal("expected spec in schema")
	}
	if schemaSpec["backend"] != "s3" {
		t.Errorf("expected backend s3, got %v", schemaSpec["backend"])
	}
	if schemaSpec["size"] != "50Gi" {
		t.Errorf("expected size 50Gi, got %v", schemaSpec["size"])
	}

	resources, ok := spec["resources"].([]any)
	if !ok || len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %v", spec["resources"])
	}
	res := resources[0].(map[string]any)
	if res["id"] != "bucket" {
		t.Errorf("expected resource id bucket, got %v", res["id"])
	}
	tmpl := res["template"].(map[string]any)
	if tmpl["apiVersion"] != "s3.aws.upbound.io/v1beta1" {
		t.Errorf("expected s3 apiVersion, got %v", tmpl["apiVersion"])
	}
	if tmpl["kind"] != "Bucket" {
		t.Errorf("expected Bucket kind, got %v", tmpl["kind"])
	}
}

func TestCompileObjectStorageRGD_GCS(t *testing.T) {
	size := resource.MustParse("100Gi")
	storage := &choristerv1alpha1.ChoStorage{
		ObjectMeta: metav1.ObjectMeta{Name: "assets-bucket", Namespace: "myapp-assets"},
		Spec: choristerv1alpha1.ChoStorageSpec{
			Application:   "myapp",
			Domain:        "assets",
			Variant:       "object",
			ObjectBackend: "gcs",
			Size:          &size,
		},
	}

	rgd := CompileObjectStorageRGD(storage)
	spec := getSpec(t, rgd)
	resources := spec["resources"].([]any)
	tmpl := resources[0].(map[string]any)["template"].(map[string]any)
	if tmpl["apiVersion"] != "storage.gcp.upbound.io/v1beta1" {
		t.Errorf("expected gcs apiVersion, got %v", tmpl["apiVersion"])
	}
}

func TestCompileObjectStorageRGD_DefaultSize(t *testing.T) {
	storage := &choristerv1alpha1.ChoStorage{
		ObjectMeta: metav1.ObjectMeta{Name: "no-size", Namespace: "myapp-data"},
		Spec: choristerv1alpha1.ChoStorageSpec{
			Application:   "myapp",
			Domain:        "data",
			Variant:       "object",
			ObjectBackend: "s3",
			// Size is nil — should default to 10Gi
		},
	}

	rgd := CompileObjectStorageRGD(storage)
	spec := getSpec(t, rgd)
	schemaSpec := spec["schema"].(map[string]any)["spec"].(map[string]any)
	if schemaSpec["size"] != "10Gi" {
		t.Errorf("expected default size 10Gi, got %v", schemaSpec["size"])
	}
}
