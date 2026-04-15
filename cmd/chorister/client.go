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

package main

import (
	"fmt"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type contextKey string

const (
	clientContextKey        contextKey = "chorister-client"
	kubeClientsetContextKey contextKey = "chorister-kube-clientset"
)

// getClient retrieves the controller-runtime client from command context,
// or builds one from kubeconfig if not injected (production path).
func getClient(cmd *cobra.Command) (client.Client, error) {
	if c, ok := cmd.Context().Value(clientContextKey).(client.Client); ok {
		return c, nil
	}

	scheme := runtime.NewScheme()
	if err := choristerv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register chorister scheme: %w", err)
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}

	c, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes client: %w", err)
	}
	return c, nil
}

// getKubeClientset returns a native Kubernetes clientset built from kubeconfig.
// This is needed for operations the controller-runtime client does not support,
// such as streaming pod logs.
// In tests, a fake kubernetes.Interface may be injected via the command context.
func getKubeClientset(cmd *cobra.Command) (kubernetes.Interface, error) {
	if cs, ok := cmd.Context().Value(kubeClientsetContextKey).(kubernetes.Interface); ok {
		return cs, nil
	}
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}

	cs, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes clientset: %w", err)
	}
	return cs, nil
}
