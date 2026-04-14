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

// ---------------------------------------------------------------------------
// 1A.13 — Developer daily workflow (e2e, Kind+Cilium)
// ---------------------------------------------------------------------------

func TestE2E_DeveloperWorkflow(t *testing.T) {
	feature := features.New("developer daily workflow").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Skip("awaiting Phase 2-8: full developer workflow end-to-end")
			return ctx
		}).
		Assess("create ChoApplication with 2 domains", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Create ChoApplication with domains: payments, auth
			return ctx
		}).
		Assess("sandbox create via CLI", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// chorister sandbox create --domain payments --name alice
			return ctx
		}).
		Assess("apply compute + database to sandbox", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// chorister apply --domain payments --sandbox alice (compute + database)
			return ctx
		}).
		Assess("resources running in sandbox namespace", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// Assert Deployment, Service, SGCluster/Secret running
			return ctx
		}).
		Assess("diff shows differences from empty prod", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// chorister diff --domain payments --sandbox alice → Added resources
			return ctx
		}).
		Assess("promote creates ChoPromotionRequest", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// chorister promote --domain payments --sandbox alice
			return ctx
		}).
		Assess("approve promotion updates production", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// chorister approve <promotion-id> → prod namespace updated
			return ctx
		}).
		Assess("diff shows no differences after promotion", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// chorister diff → NoDifferences
			return ctx
		}).
		Assess("sandbox destroy cleans up namespace", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// chorister sandbox destroy --domain payments --name alice
			return ctx
		}).
		Assess("compilation revision drift surfaced on controller upgrade", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Skip("awaiting Phase 21: compilation revision tracking")
			// diff surfaces drift when controller revision changes even with same DSL
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}
