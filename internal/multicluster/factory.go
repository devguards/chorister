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

// Package multicluster provides multi-cluster client management for the chorister controller.
// It reads cluster registrations from ChoCluster.spec.clusters and builds K8s clients
// for remote clusters from kubeconfig Secrets.
package multicluster

import (
	"context"
	"fmt"
	"sync"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterRole defines the purpose of a registered cluster.
type ClusterRole string

const (
	// ClusterRoleSandbox designates a cluster for sandbox workloads.
	ClusterRoleSandbox ClusterRole = "sandbox"
	// ClusterRoleProduction designates a cluster for production workloads.
	ClusterRoleProduction ClusterRole = "production"
)

// ClientBuilder builds a controller-runtime client from raw kubeconfig bytes.
// Override in tests to return fake clients without needing a real cluster.
type ClientBuilder func(kubeconfigData []byte, scheme *runtime.Scheme) (client.Client, error)

// ClientFactory provides K8s clients for the home cluster and registered remote clusters.
//
// The home cluster is where chorister CRDs live. Remote clusters are registered
// via ChoCluster.spec.clusters and their kubeconfig Secrets in cho-system.
//
// In single-cluster mode (no clusters registered), all methods return the local client.
type ClientFactory interface {
	// Local returns the home cluster client (where CRDs live).
	Local() client.Client

	// ClientFor returns a client for the named cluster.
	// Returns the local client if clusterName is empty or "local".
	// Returns an error if the cluster is not registered.
	ClientFor(ctx context.Context, clusterName string) (client.Client, error)

	// ClientForRole returns a client for the first cluster with the given role.
	// Falls back to the local client if no clusters are registered for that role.
	ClientForRole(ctx context.Context, role ClusterRole) (client.Client, error)

	// Refresh reloads cluster registrations from the ChoCluster resource
	// and rebuilds remote clients from kubeconfig Secrets.
	Refresh(ctx context.Context) error
}

// FactoryConfig configures a new ClientFactory.
type FactoryConfig struct {
	// Local is the home cluster client (where chorister CRDs live).
	Local client.Client

	// Scheme is the runtime scheme for building remote clients.
	Scheme *runtime.Scheme

	// Namespace is the namespace where kubeconfig Secrets are stored.
	// Defaults to "cho-system".
	Namespace string

	// ChoClusterName is the name of the ChoCluster resource to read.
	// Defaults to "cluster-config".
	ChoClusterName string

	// ClientBuilder overrides the default function that builds a client from kubeconfig bytes.
	// Used in tests to inject fake clients.
	ClientBuilder ClientBuilder
}

type clusterEntry struct {
	name string
	role ClusterRole
}

type factory struct {
	local          client.Client
	scheme         *runtime.Scheme
	namespace      string
	choClusterName string
	clientBuilder  ClientBuilder

	mu       sync.RWMutex
	clusters []clusterEntry
	remotes  map[string]client.Client
}

// NewFactory creates a ClientFactory with the given configuration.
// The factory starts with no remote clusters; call Refresh() to load them.
func NewFactory(cfg FactoryConfig) ClientFactory {
	ns := cfg.Namespace
	if ns == "" {
		ns = "cho-system"
	}
	name := cfg.ChoClusterName
	if name == "" {
		name = "cluster-config"
	}
	builder := cfg.ClientBuilder
	if builder == nil {
		builder = defaultClientBuilder
	}
	return &factory{
		local:          cfg.Local,
		scheme:         cfg.Scheme,
		namespace:      ns,
		choClusterName: name,
		clientBuilder:  builder,
		remotes:        make(map[string]client.Client),
	}
}

func (f *factory) Local() client.Client {
	return f.local
}

func (f *factory) ClientFor(ctx context.Context, clusterName string) (client.Client, error) {
	if clusterName == "" || clusterName == "local" {
		return f.local, nil
	}

	f.mu.RLock()
	c, ok := f.remotes[clusterName]
	f.mu.RUnlock()

	if ok {
		return c, nil
	}

	return nil, fmt.Errorf("cluster %q not registered or client not built; call Refresh() after updating ChoCluster.spec.clusters", clusterName)
}

func (f *factory) ClientForRole(ctx context.Context, role ClusterRole) (client.Client, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	for _, entry := range f.clusters {
		if entry.role == role {
			if c, ok := f.remotes[entry.name]; ok {
				return c, nil
			}
		}
	}

	// Fallback: no cluster registered for this role → use local (single-cluster mode)
	return f.local, nil
}

func (f *factory) Refresh(ctx context.Context) error {
	// 1. Read ChoCluster resource.
	choCluster := &choristerv1alpha1.ChoCluster{}
	if err := f.local.Get(ctx, types.NamespacedName{Name: f.choClusterName}, choCluster); err != nil {
		return fmt.Errorf("reading ChoCluster %q: %w", f.choClusterName, err)
	}

	newClusters := make([]clusterEntry, 0, len(choCluster.Spec.Clusters))
	newRemotes := make(map[string]client.Client, len(choCluster.Spec.Clusters))

	for _, entry := range choCluster.Spec.Clusters {
		// 2. Read the kubeconfig Secret.
		secret := &corev1.Secret{}
		secretKey := types.NamespacedName{Namespace: f.namespace, Name: entry.SecretRef}
		if err := f.local.Get(ctx, secretKey, secret); err != nil {
			return fmt.Errorf("reading kubeconfig Secret %q for cluster %q: %w", entry.SecretRef, entry.Name, err)
		}

		// 3. Extract the "kubeconfig" key.
		kubeconfigData, ok := secret.Data["kubeconfig"]
		if !ok {
			return fmt.Errorf("secret %q for cluster %q missing \"kubeconfig\" key", entry.SecretRef, entry.Name)
		}

		// 4. Build a client from the kubeconfig.
		remoteClient, err := f.clientBuilder(kubeconfigData, f.scheme)
		if err != nil {
			return fmt.Errorf("building client for cluster %q: %w", entry.Name, err)
		}

		newClusters = append(newClusters, clusterEntry{
			name: entry.Name,
			role: ClusterRole(entry.Role),
		})
		newRemotes[entry.Name] = remoteClient
	}

	// 5. Atomically swap.
	f.mu.Lock()
	f.clusters = newClusters
	f.remotes = newRemotes
	f.mu.Unlock()

	return nil
}

// defaultClientBuilder builds a real K8s client from kubeconfig bytes.
func defaultClientBuilder(kubeconfigData []byte, s *runtime.Scheme) (client.Client, error) {
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
	if err != nil {
		return nil, fmt.Errorf("parsing kubeconfig: %w", err)
	}
	c, err := client.New(restConfig, client.Options{Scheme: s})
	if err != nil {
		return nil, fmt.Errorf("creating client: %w", err)
	}
	return c, nil
}
