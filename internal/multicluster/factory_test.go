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

package multicluster

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := choristerv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("register chorister scheme: %v", err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("register core scheme: %v", err)
	}
	return s
}

func fakeLocal(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	s := testScheme(t)
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}

// fakeBuilder returns a ClientBuilder that always returns the given pre-built client.
func fakeBuilder(c client.Client) ClientBuilder {
	return func(_ []byte, _ *runtime.Scheme) (client.Client, error) {
		return c, nil
	}
}

// choClusterWithClusters creates a ChoCluster with the given cluster entries.
func choClusterWithClusters(name string, entries ...choristerv1alpha1.ClusterRegistryEntry) *choristerv1alpha1.ChoCluster {
	return &choristerv1alpha1.ChoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: choristerv1alpha1.ChoClusterSpec{
			Clusters: entries,
		},
	}
}

// kubeconfigSecret creates a Secret with a "kubeconfig" key in the given namespace.
func kubeconfigSecret(ns, name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Data:       map[string][]byte{"kubeconfig": []byte("fake-kubeconfig-data")},
	}
}

// ---------------------------------------------------------------------------
// Tests: basic local client behavior (should PASS with stub)
// ---------------------------------------------------------------------------

func TestFactory_LocalClientAlwaysAvailable(t *testing.T) {
	local := fakeLocal(t)
	f := NewFactory(FactoryConfig{Local: local})

	got := f.Local()
	if got != local {
		t.Fatal("Local() did not return the injected client")
	}
}

func TestFactory_ClientForEmptyNameReturnsLocal(t *testing.T) {
	local := fakeLocal(t)
	f := NewFactory(FactoryConfig{Local: local})

	got, err := f.ClientFor(context.Background(), "")
	if err != nil {
		t.Fatalf("ClientFor empty: %v", err)
	}
	if got != local {
		t.Fatal("ClientFor('') did not return the local client")
	}
}

func TestFactory_ClientForLocalStringReturnsLocal(t *testing.T) {
	local := fakeLocal(t)
	f := NewFactory(FactoryConfig{Local: local})

	got, err := f.ClientFor(context.Background(), "local")
	if err != nil {
		t.Fatalf("ClientFor local: %v", err)
	}
	if got != local {
		t.Fatal("ClientFor('local') did not return the local client")
	}
}

func TestFactory_ClientForUnknownClusterErrors(t *testing.T) {
	f := NewFactory(FactoryConfig{Local: fakeLocal(t)})

	_, err := f.ClientFor(context.Background(), "nonexistent-cluster")
	if err == nil {
		t.Fatal("expected error for unknown cluster, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent-cluster") {
		t.Fatalf("error should mention cluster name, got: %v", err)
	}
}

func TestFactory_ClientForRoleFallsBackToLocal(t *testing.T) {
	local := fakeLocal(t)
	f := NewFactory(FactoryConfig{Local: local})

	// No clusters registered → should fall back to local
	got, err := f.ClientForRole(context.Background(), ClusterRoleSandbox)
	if err != nil {
		t.Fatalf("ClientForRole: %v", err)
	}
	if got != local {
		t.Fatal("expected fallback to local when no clusters registered")
	}
}

func TestFactory_ClientForRoleProductionFallsBackToLocal(t *testing.T) {
	local := fakeLocal(t)
	f := NewFactory(FactoryConfig{Local: local})

	got, err := f.ClientForRole(context.Background(), ClusterRoleProduction)
	if err != nil {
		t.Fatalf("ClientForRole: %v", err)
	}
	if got != local {
		t.Fatal("expected fallback to local when no production cluster registered")
	}
}

func TestFactory_DefaultNamespace(t *testing.T) {
	f := NewFactory(FactoryConfig{Local: fakeLocal(t)}).(*factory)
	if f.namespace != "cho-system" {
		t.Fatalf("expected default namespace 'cho-system', got %q", f.namespace)
	}
}

func TestFactory_DefaultChoClusterName(t *testing.T) {
	f := NewFactory(FactoryConfig{Local: fakeLocal(t)}).(*factory)
	if f.choClusterName != "cluster-config" {
		t.Fatalf("expected default name 'cluster-config', got %q", f.choClusterName)
	}
}

func TestFactory_CustomNamespace(t *testing.T) {
	f := NewFactory(FactoryConfig{Local: fakeLocal(t), Namespace: "custom-ns"}).(*factory)
	if f.namespace != "custom-ns" {
		t.Fatalf("expected namespace 'custom-ns', got %q", f.namespace)
	}
}

// ---------------------------------------------------------------------------
// Tests: Refresh — reads ChoCluster + Secrets, builds remote clients
// These tests WILL FAIL until Refresh() is implemented.
// ---------------------------------------------------------------------------

func TestFactory_RefreshReadsChoCluster(t *testing.T) {
	choCluster := choClusterWithClusters("cluster-config",
		choristerv1alpha1.ClusterRegistryEntry{
			Name:      "sandbox-pool",
			Role:      "sandbox",
			SecretRef: "sandbox-pool-kubeconfig",
		},
	)
	secret := kubeconfigSecret("cho-system", "sandbox-pool-kubeconfig")
	local := fakeLocal(t, choCluster, secret)

	fakeRemote := fakeLocal(t)
	f := NewFactory(FactoryConfig{
		Local:         local,
		Scheme:        testScheme(t),
		ClientBuilder: fakeBuilder(fakeRemote),
	})

	err := f.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh() should succeed when ChoCluster and Secret exist, got: %v", err)
	}
}

func TestFactory_RefreshBuildsRemoteClients(t *testing.T) {
	choCluster := choClusterWithClusters("cluster-config",
		choristerv1alpha1.ClusterRegistryEntry{
			Name:      "prod-cell-1",
			Role:      "production",
			SecretRef: "prod-cell-1-kubeconfig",
		},
	)
	secret := kubeconfigSecret("cho-system", "prod-cell-1-kubeconfig")
	local := fakeLocal(t, choCluster, secret)

	fakeRemote := fakeLocal(t)
	f := NewFactory(FactoryConfig{
		Local:         local,
		Scheme:        testScheme(t),
		ClientBuilder: fakeBuilder(fakeRemote),
	})

	if err := f.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// After refresh, the remote client should be available by name.
	got, err := f.ClientFor(context.Background(), "prod-cell-1")
	if err != nil {
		t.Fatalf("ClientFor after Refresh: %v", err)
	}
	if got != fakeRemote {
		t.Fatal("expected the injected fake remote client")
	}
}

func TestFactory_ClientForRoleAfterRefresh(t *testing.T) {
	choCluster := choClusterWithClusters("cluster-config",
		choristerv1alpha1.ClusterRegistryEntry{
			Name:      "sandbox-pool",
			Role:      "sandbox",
			SecretRef: "sandbox-kubeconfig",
		},
		choristerv1alpha1.ClusterRegistryEntry{
			Name:      "prod-cell-1",
			Role:      "production",
			SecretRef: "prod-kubeconfig",
		},
	)
	sandboxSecret := kubeconfigSecret("cho-system", "sandbox-kubeconfig")
	prodSecret := kubeconfigSecret("cho-system", "prod-kubeconfig")
	local := fakeLocal(t, choCluster, sandboxSecret, prodSecret)

	// Use different fake clients for sandbox and production to verify routing.
	sandboxClient := fakeLocal(t)
	prodClient := fakeLocal(t)
	callCount := 0
	builder := func(_ []byte, _ *runtime.Scheme) (client.Client, error) {
		callCount++
		if callCount == 1 {
			return sandboxClient, nil
		}
		return prodClient, nil
	}

	f := NewFactory(FactoryConfig{
		Local:         local,
		Scheme:        testScheme(t),
		ClientBuilder: builder,
	})

	if err := f.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	got, err := f.ClientForRole(context.Background(), ClusterRoleSandbox)
	if err != nil {
		t.Fatalf("ClientForRole(sandbox): %v", err)
	}
	if got != sandboxClient {
		t.Fatal("expected sandbox client from ClientForRole(sandbox)")
	}

	got, err = f.ClientForRole(context.Background(), ClusterRoleProduction)
	if err != nil {
		t.Fatalf("ClientForRole(production): %v", err)
	}
	if got != prodClient {
		t.Fatal("expected production client from ClientForRole(production)")
	}
}

func TestFactory_RefreshMultipleClusters(t *testing.T) {
	choCluster := choClusterWithClusters("cluster-config",
		choristerv1alpha1.ClusterRegistryEntry{Name: "a", Role: "sandbox", SecretRef: "a-kc"},
		choristerv1alpha1.ClusterRegistryEntry{Name: "b", Role: "production", SecretRef: "b-kc"},
		choristerv1alpha1.ClusterRegistryEntry{Name: "c", Role: "production", SecretRef: "c-kc"},
	)
	local := fakeLocal(t, choCluster,
		kubeconfigSecret("cho-system", "a-kc"),
		kubeconfigSecret("cho-system", "b-kc"),
		kubeconfigSecret("cho-system", "c-kc"),
	)

	f := NewFactory(FactoryConfig{
		Local:         local,
		Scheme:        testScheme(t),
		ClientBuilder: fakeBuilder(fakeLocal(t)),
	})

	if err := f.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// All three clusters should be reachable.
	for _, name := range []string{"a", "b", "c"} {
		if _, err := f.ClientFor(context.Background(), name); err != nil {
			t.Fatalf("ClientFor(%q) after Refresh: %v", name, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests: Refresh error cases
// These tests WILL FAIL until Refresh() is implemented.
// ---------------------------------------------------------------------------

func TestFactory_RefreshMissingChoClusterErrors(t *testing.T) {
	// No ChoCluster object in the fake client.
	local := fakeLocal(t)

	f := NewFactory(FactoryConfig{
		Local:  local,
		Scheme: testScheme(t),
	})

	err := f.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected error when ChoCluster is missing")
	}
	if !strings.Contains(err.Error(), "cluster-config") {
		t.Fatalf("error should mention ChoCluster name, got: %v", err)
	}
}

func TestFactory_RefreshMissingSecretErrors(t *testing.T) {
	choCluster := choClusterWithClusters("cluster-config",
		choristerv1alpha1.ClusterRegistryEntry{
			Name:      "sandbox-pool",
			Role:      "sandbox",
			SecretRef: "nonexistent-secret",
		},
	)
	local := fakeLocal(t, choCluster)

	f := NewFactory(FactoryConfig{
		Local:         local,
		Scheme:        testScheme(t),
		ClientBuilder: fakeBuilder(fakeLocal(t)),
	})

	err := f.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected error when kubeconfig Secret is missing")
	}
	if !strings.Contains(err.Error(), "nonexistent-secret") {
		t.Fatalf("error should mention Secret name, got: %v", err)
	}
}

func TestFactory_RefreshMissingKubeconfigKeyErrors(t *testing.T) {
	choCluster := choClusterWithClusters("cluster-config",
		choristerv1alpha1.ClusterRegistryEntry{
			Name:      "sandbox-pool",
			Role:      "sandbox",
			SecretRef: "bad-secret",
		},
	)
	// Secret exists but has no "kubeconfig" key.
	badSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "cho-system", Name: "bad-secret"},
		Data:       map[string][]byte{"wrong-key": []byte("data")},
	}
	local := fakeLocal(t, choCluster, badSecret)

	f := NewFactory(FactoryConfig{
		Local:         local,
		Scheme:        testScheme(t),
		ClientBuilder: fakeBuilder(fakeLocal(t)),
	})

	err := f.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected error when Secret is missing 'kubeconfig' key")
	}
	if !strings.Contains(err.Error(), "kubeconfig") {
		t.Fatalf("error should mention missing key, got: %v", err)
	}
}

func TestFactory_RefreshClientBuilderFailure(t *testing.T) {
	choCluster := choClusterWithClusters("cluster-config",
		choristerv1alpha1.ClusterRegistryEntry{
			Name:      "bad-cluster",
			Role:      "sandbox",
			SecretRef: "bad-cluster-kc",
		},
	)
	secret := kubeconfigSecret("cho-system", "bad-cluster-kc")
	local := fakeLocal(t, choCluster, secret)

	failingBuilder := func(_ []byte, _ *runtime.Scheme) (client.Client, error) {
		return nil, fmt.Errorf("simulated connection failure")
	}

	f := NewFactory(FactoryConfig{
		Local:         local,
		Scheme:        testScheme(t),
		ClientBuilder: failingBuilder,
	})

	err := f.Refresh(context.Background())
	if err == nil {
		t.Fatal("expected error when ClientBuilder fails")
	}
	if !strings.Contains(err.Error(), "bad-cluster") {
		t.Fatalf("error should mention cluster name, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests: Refresh replaces stale clients
// These tests WILL FAIL until Refresh() is implemented.
// ---------------------------------------------------------------------------

func TestFactory_RefreshReplacesStaleClients(t *testing.T) {
	choCluster := choClusterWithClusters("cluster-config",
		choristerv1alpha1.ClusterRegistryEntry{
			Name:      "sandbox-pool",
			Role:      "sandbox",
			SecretRef: "sandbox-kc",
		},
	)
	secret := kubeconfigSecret("cho-system", "sandbox-kc")
	local := fakeLocal(t, choCluster, secret)

	firstClient := fakeLocal(t)
	secondClient := fakeLocal(t)
	callCount := 0
	builder := func(_ []byte, _ *runtime.Scheme) (client.Client, error) {
		callCount++
		if callCount == 1 {
			return firstClient, nil
		}
		return secondClient, nil
	}

	f := NewFactory(FactoryConfig{
		Local:         local,
		Scheme:        testScheme(t),
		ClientBuilder: builder,
	})

	// First refresh.
	if err := f.Refresh(context.Background()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}
	got, _ := f.ClientFor(context.Background(), "sandbox-pool")
	if got != firstClient {
		t.Fatal("expected first client after first refresh")
	}

	// Second refresh should replace with new client.
	if err := f.Refresh(context.Background()); err != nil {
		t.Fatalf("second Refresh: %v", err)
	}
	got, _ = f.ClientFor(context.Background(), "sandbox-pool")
	if got != secondClient {
		t.Fatal("expected second client after second refresh")
	}
}

func TestFactory_RefreshRemovesDeregisteredClusters(t *testing.T) {
	// Start with one cluster registered.
	choCluster := choClusterWithClusters("cluster-config",
		choristerv1alpha1.ClusterRegistryEntry{
			Name:      "temp-cluster",
			Role:      "sandbox",
			SecretRef: "temp-kc",
		},
	)
	secret := kubeconfigSecret("cho-system", "temp-kc")
	s := testScheme(t)
	local := fake.NewClientBuilder().WithScheme(s).WithObjects(choCluster, secret).Build()

	f := NewFactory(FactoryConfig{
		Local:         local,
		Scheme:        s,
		ClientBuilder: fakeBuilder(fakeLocal(t)),
	})

	if err := f.Refresh(context.Background()); err != nil {
		t.Fatalf("first Refresh: %v", err)
	}
	if _, err := f.ClientFor(context.Background(), "temp-cluster"); err != nil {
		t.Fatalf("temp-cluster should be available: %v", err)
	}

	// Remove the cluster from ChoCluster by updating it.
	// Re-fetch the object first to get the current resource version.
	current := &choristerv1alpha1.ChoCluster{}
	if err := local.Get(context.Background(), types.NamespacedName{Name: "cluster-config"}, current); err != nil {
		t.Fatalf("get ChoCluster: %v", err)
	}
	current.Spec.Clusters = nil
	if err := local.Update(context.Background(), current); err != nil {
		t.Fatalf("update ChoCluster: %v", err)
	}

	if err := f.Refresh(context.Background()); err != nil {
		t.Fatalf("second Refresh: %v", err)
	}

	// temp-cluster should no longer be available.
	_, err := f.ClientFor(context.Background(), "temp-cluster")
	if err == nil {
		t.Fatal("expected error for deregistered cluster after Refresh")
	}
}

// ---------------------------------------------------------------------------
// Tests: Concurrency safety
// These tests WILL FAIL until Refresh() is implemented.
// ---------------------------------------------------------------------------

func TestFactory_ConcurrentRefreshAndClientFor(t *testing.T) {
	choCluster := choClusterWithClusters("cluster-config",
		choristerv1alpha1.ClusterRegistryEntry{
			Name:      "sandbox-pool",
			Role:      "sandbox",
			SecretRef: "sandbox-kc",
		},
	)
	secret := kubeconfigSecret("cho-system", "sandbox-kc")
	local := fakeLocal(t, choCluster, secret)

	f := NewFactory(FactoryConfig{
		Local:         local,
		Scheme:        testScheme(t),
		ClientBuilder: fakeBuilder(fakeLocal(t)),
	})

	var wg sync.WaitGroup
	ctx := context.Background()

	// Concurrent refreshes.
	for range 10 {
		wg.Go(func() {
			_ = f.Refresh(ctx)
		})
	}

	// Concurrent reads.
	for range 20 {
		wg.Go(func() {
			_, _ = f.ClientFor(ctx, "sandbox-pool")
			_, _ = f.ClientForRole(ctx, ClusterRoleSandbox)
			_ = f.Local()
		})
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Tests: default client builder (exercises the TODO code path)
// ---------------------------------------------------------------------------

func TestFactory_DefaultClientBuilderRejectsInvalidKubeconfig(t *testing.T) {
	_, err := defaultClientBuilder([]byte("not-valid-kubeconfig"), nil)
	if err == nil {
		t.Fatal("expected defaultClientBuilder to return error for invalid kubeconfig")
	}
}
