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

	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestE2ENamespaceLifecycle(t *testing.T) {
	namespaceName := envconf.RandomName("chorister-e2e", 24)

	feature := features.New("namespace lifecycle").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			return createNamespace(ctx, t, cfg, namespaceName)
		}).
		Assess("namespace exists", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			exists, err := namespaceExists(ctx, cfg, namespaceName)
			if err != nil {
				t.Fatalf("check namespace existence: %v", err)
			}
			if !exists {
				t.Fatalf("namespace %q was not created", namespaceName)
			}
			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			return cleanupNamespace(ctx, t, cfg, namespaceName)
		}).
		Feature()

	testEnv.Test(t, feature)
}
