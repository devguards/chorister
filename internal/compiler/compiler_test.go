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
	t.Skip("awaiting Phase 13.2: Ingress with JWT auth requirement")

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

	_ = network
	// TODO: Assert Gateway API HTTPRoute manifest
}

func TestCompileNetwork_EgressCiliumPolicy(t *testing.T) {
	t.Skip("awaiting Phase 13.1: Egress allowlist enforcement")

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

	_ = network
	// TODO: Assert CiliumNetworkPolicy with FQDN rules
}

func TestCompileNetwork_CrossApplicationLink(t *testing.T) {
	t.Skip("awaiting Phase 13.3: Cross-application links via Gateway API")

	// TODO: Assert link resource → HTTPRoute + ReferenceGrant + CiliumEnvoyConfig + blocking NetworkPolicy
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
