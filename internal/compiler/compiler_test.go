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
	"testing"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ---------------------------------------------------------------------------
// 1A.1 — Compilation unit tests
// ---------------------------------------------------------------------------

// --- ChoCompute compilation ---

func TestCompileCompute_DeploymentShape(t *testing.T) {
	t.Skip("awaiting Phase 3: ChoCompute reconciler → Deployment + Service")

	replicas := int32(3)
	port := int32(8080)
	compute := &choristerv1alpha1.ChoCompute{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "myapp-payments"},
		Spec: choristerv1alpha1.ChoComputeSpec{
			Application: "myapp",
			Domain:      "payments",
			Image:       "myregistry/api:v1.2.3",
			Variant:     "long-running",
			Replicas:    &replicas,
			Port:        &port,
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
			},
		},
	}

	_ = compute
	// TODO: Call compiler and assert:
	// - Deployment name, namespace, labels (chorister.dev/application, chorister.dev/domain)
	// - Container image, ports, resource requests
	// - Service name, port, selector
}

func TestCompileCompute_JobVariant(t *testing.T) {
	t.Skip("awaiting Phase 3.3: Compute variants — Job and CronJob")

	replicas := int32(1)
	compute := &choristerv1alpha1.ChoCompute{
		ObjectMeta: metav1.ObjectMeta{Name: "migrate", Namespace: "myapp-payments"},
		Spec: choristerv1alpha1.ChoComputeSpec{
			Application: "myapp",
			Domain:      "payments",
			Image:       "myregistry/migrate:v1",
			Variant:     "job",
			Replicas:    &replicas,
		},
	}

	_ = compute
	// TODO: Assert Job manifest (not Deployment)
}

func TestCompileCompute_CronJobVariant(t *testing.T) {
	t.Skip("awaiting Phase 3.3: Compute variants — Job and CronJob")

	replicas := int32(1)
	compute := &choristerv1alpha1.ChoCompute{
		ObjectMeta: metav1.ObjectMeta{Name: "cleanup", Namespace: "myapp-payments"},
		Spec: choristerv1alpha1.ChoComputeSpec{
			Application: "myapp",
			Domain:      "payments",
			Image:       "myregistry/cleanup:v1",
			Variant:     "cronjob",
			Schedule:    "0 2 * * *",
			Replicas:    &replicas,
		},
	}

	_ = compute
	// TODO: Assert CronJob with correct schedule field
}

func TestCompileCompute_GPUVariant(t *testing.T) {
	t.Skip("awaiting Phase 3: ChoCompute reconciler with GPU support")

	replicas := int32(1)
	compute := &choristerv1alpha1.ChoCompute{
		ObjectMeta: metav1.ObjectMeta{Name: "training", Namespace: "myapp-ml"},
		Spec: choristerv1alpha1.ChoComputeSpec{
			Application: "myapp",
			Domain:      "ml",
			Image:       "myregistry/training:v1",
			Variant:     "gpu",
			Replicas:    &replicas,
			GPU: &choristerv1alpha1.GPUSpec{
				Count: 2,
				Type:  "nvidia.com/gpu",
			},
		},
	}

	_ = compute
	// TODO: Assert Deployment/Job with nvidia.com/gpu limits and expected labels
}

func TestCompileCompute_ScaleToZeroVariant(t *testing.T) {
	t.Skip("awaiting scale-to-zero engine selection (future phase)")

	// TODO: Assert scale-to-zero variant compiles to the selected engine contract
}

func TestCompileCompute_HPA(t *testing.T) {
	t.Skip("awaiting Phase 3.2: HPA and PDB for compute")

	replicas := int32(2)
	targetCPU := int32(80)
	compute := &choristerv1alpha1.ChoCompute{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "myapp-payments"},
		Spec: choristerv1alpha1.ChoComputeSpec{
			Application: "myapp",
			Domain:      "payments",
			Image:       "myregistry/api:v1",
			Replicas:    &replicas,
			Autoscaling: &choristerv1alpha1.AutoscalingSpec{
				MinReplicas:      2,
				MaxReplicas:      10,
				TargetCPUPercent: &targetCPU,
			},
		},
	}

	_ = compute
	// TODO: Assert HPA manifest with correct min/max/target
}

func TestCompileCompute_PDB(t *testing.T) {
	t.Skip("awaiting Phase 3.2: HPA and PDB for compute")

	replicas := int32(3)
	compute := &choristerv1alpha1.ChoCompute{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "myapp-payments"},
		Spec: choristerv1alpha1.ChoComputeSpec{
			Application: "myapp",
			Domain:      "payments",
			Image:       "myregistry/api:v1",
			Replicas:    &replicas,
		},
	}

	_ = compute
	// TODO: Assert PDB with minAvailable = replicas - 1 = 2
}

// --- ChoDatabase compilation ---

func TestCompileDatabase_SGCluster(t *testing.T) {
	t.Skip("awaiting Phase 4.2: ChoDatabase reconciler → SGCluster")

	db := &choristerv1alpha1.ChoDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "main", Namespace: "myapp-payments"},
		Spec: choristerv1alpha1.ChoDatabaseSpec{
			Application: "myapp",
			Domain:      "payments",
			Engine:      "postgres",
			Size:        "medium",
			HA:          true,
		},
	}

	_ = db
	// TODO: Assert SGCluster fields, instance count >= 2 for ha=true
}

func TestCompileDatabase_Credentials(t *testing.T) {
	t.Skip("awaiting Phase 4.3: Database secret wiring")

	db := &choristerv1alpha1.ChoDatabase{
		ObjectMeta: metav1.ObjectMeta{Name: "main", Namespace: "myapp-payments"},
		Spec: choristerv1alpha1.ChoDatabaseSpec{
			Application: "myapp",
			Domain:      "payments",
			Engine:      "postgres",
			Size:        "small",
		},
	}

	_ = db
	// TODO: Assert credential Secret with expected keys: host, port, username, password, uri
	// Secret name follows: {domain}--database--{name}-credentials
}

// --- ChoQueue compilation ---

func TestCompileQueue_NATSResources(t *testing.T) {
	t.Skip("awaiting Phase 5.2: ChoQueue reconciler → NATS JetStream")

	queue := &choristerv1alpha1.ChoQueue{
		ObjectMeta: metav1.ObjectMeta{Name: "events", Namespace: "myapp-payments"},
		Spec: choristerv1alpha1.ChoQueueSpec{
			Application: "myapp",
			Domain:      "payments",
			Type:        "nats",
			Size:        "small",
		},
	}

	_ = queue
	// TODO: Assert NATS JetStream manifests
}

// --- ChoCache compilation ---

func TestCompileCache_Dragonfly(t *testing.T) {
	t.Skip("awaiting Phase 5.3: ChoCache reconciler → Dragonfly")

	cache := &choristerv1alpha1.ChoCache{
		ObjectMeta: metav1.ObjectMeta{Name: "sessions", Namespace: "myapp-auth"},
		Spec: choristerv1alpha1.ChoCacheSpec{
			Application: "myapp",
			Domain:      "auth",
			Size:        "medium",
		},
	}

	_ = cache
	// TODO: Assert Dragonfly Deployment+Service, size→resource mapping
}

// --- ChoStorage compilation ---

func TestCompileStorage_ObjectBackend(t *testing.T) {
	t.Skip("awaiting Phase 3+: ChoStorage compilation")

	size := resource.MustParse("50Gi")
	storage := &choristerv1alpha1.ChoStorage{
		ObjectMeta: metav1.ObjectMeta{Name: "uploads", Namespace: "myapp-media"},
		Spec: choristerv1alpha1.ChoStorageSpec{
			Application:   "myapp",
			Domain:        "media",
			Variant:       "object",
			Size:          &size,
			ObjectBackend: "s3",
		},
	}

	_ = storage
	// TODO: Assert provider binding/manifests for S3 backend
}

func TestCompileStorage_BlockPVC(t *testing.T) {
	t.Skip("awaiting Phase 3+: ChoStorage compilation")

	size := resource.MustParse("20Gi")
	storage := &choristerv1alpha1.ChoStorage{
		ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "myapp-payments"},
		Spec: choristerv1alpha1.ChoStorageSpec{
			Application:  "myapp",
			Domain:       "payments",
			Variant:      "block",
			Size:         &size,
			AccessMode:   "ReadWriteOnce",
			StorageClass: "encrypted-ssd",
		},
	}

	_ = storage
	// TODO: Assert PVC with expected class, size, and access mode
}

func TestCompileStorage_FilePVC(t *testing.T) {
	t.Skip("awaiting Phase 3+: ChoStorage compilation")

	size := resource.MustParse("100Gi")
	storage := &choristerv1alpha1.ChoStorage{
		ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "myapp-media"},
		Spec: choristerv1alpha1.ChoStorageSpec{
			Application: "myapp",
			Domain:      "media",
			Variant:     "file",
			Size:        &size,
			AccessMode:  "ReadWriteMany",
		},
	}

	_ = storage
	// TODO: Assert RWX-capable PVC or storage-class specific manifest
}

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
	rules := spec["rules"].([]interface{})
	backendRefs := rules[0].(map[string]interface{})["backendRefs"].([]interface{})
	if backendRefs[0].(map[string]interface{})["port"].(int) != 443 {
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
	egress := spec["egress"].([]interface{})
	if len(egress) < 2 {
		t.Fatalf("expected DNS plus allowlist rules, got %d", len(egress))
	}
	allowRule := egress[1].(map[string]interface{})
	toFQDNs := allowRule["toFQDNs"].([]interface{})
	if toFQDNs[0].(map[string]interface{})["matchName"] != "api.stripe.com" {
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
		Name:         "auth-api",
		Target:       "platform-app",
		TargetDomain: "auth",
		Port:         8443,
		Consumers:    []string{"payments"},
		Auth:         &choristerv1alpha1.LinkAuth{Type: "jwt"},
		RateLimit:    &choristerv1alpha1.LinkRateLimit{RequestsPerMinute: 120},
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
}

// --- Table-driven edge cases ---

func TestCompileCompute_EdgeCases(t *testing.T) {
	t.Skip("awaiting Phase 3: ChoCompute reconciler")

	tests := []struct {
		name    string
		spec    choristerv1alpha1.ChoComputeSpec
		wantErr bool
	}{
		{
			name: "zero replicas",
			spec: choristerv1alpha1.ChoComputeSpec{
				Application: "myapp",
				Domain:      "payments",
				Image:       "myregistry/api:v1",
				Replicas:    int32Ptr(0),
			},
			wantErr: true,
		},
		{
			name: "empty image",
			spec: choristerv1alpha1.ChoComputeSpec{
				Application: "myapp",
				Domain:      "payments",
				Image:       "",
				Replicas:    int32Ptr(1),
			},
			wantErr: true,
		},
		{
			name: "missing required fields",
			spec: choristerv1alpha1.ChoComputeSpec{
				Image: "myregistry/api:v1",
			},
			wantErr: true,
		},
		{
			name: "cronjob without schedule",
			spec: choristerv1alpha1.ChoComputeSpec{
				Application: "myapp",
				Domain:      "payments",
				Image:       "myregistry/api:v1",
				Variant:     "cronjob",
				Replicas:    int32Ptr(1),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = tt // TODO: Call compiler, check for expected error
		})
	}
}

func int32Ptr(v int32) *int32 { return &v }

func getSpec(t *testing.T, obj *unstructured.Unstructured) map[string]interface{} {
	t.Helper()
	spec, ok := obj.Object["spec"].(map[string]interface{})
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
	ingress, ok := spec["ingress"].([]interface{})
	if !ok || len(ingress) != 1 {
		t.Fatalf("expected 1 ingress rule, got %v", spec["ingress"])
	}

	rule, ok := ingress[0].(map[string]interface{})
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
	kprobes, ok := spec["kprobes"].([]interface{})
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

	issuerRef, ok := spec["issuerRef"].(map[string]interface{})
	if !ok {
		t.Fatal("expected issuerRef map")
	}
	if issuerRef["name"] != "chorister-cluster-issuer" {
		t.Errorf("expected issuer name chorister-cluster-issuer, got %v", issuerRef["name"])
	}
	if issuerRef["kind"] != "ClusterIssuer" {
		t.Errorf("expected issuer kind ClusterIssuer, got %v", issuerRef["kind"])
	}

	dnsNames, ok := spec["dnsNames"].([]interface{})
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
	ingress, ok := spec["ingress"].([]interface{})
	if !ok || len(ingress) != 1 {
		t.Fatalf("expected 1 ingress rule, got %v", spec["ingress"])
	}

	rule := ingress[0].(map[string]interface{})
	auth, ok := rule["authentication"].(map[string]interface{})
	if !ok || auth["mode"] != "required" {
		t.Error("expected authentication mode=required in ingress")
	}

	egress, ok := spec["egress"].([]interface{})
	if !ok || len(egress) != 1 {
		t.Fatalf("expected 1 egress rule, got %v", spec["egress"])
	}

	egressRule := egress[0].(map[string]interface{})
	egressAuth, ok := egressRule["authentication"].(map[string]interface{})
	if !ok || egressAuth["mode"] != "required" {
		t.Error("expected authentication mode=required in egress")
	}
}
