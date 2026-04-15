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
// 1A.18 — Incident response and archive safety (e2e)
// ---------------------------------------------------------------------------

func TestE2E_AdminIsolateDomain(t *testing.T) {
	feature := features.New("admin isolate domain").
		Assess("isolate tightens NetworkPolicy and freezes promotions", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Skip("e2e: requires full Cilium in Kind cluster — covered by CLI and envtest unit tests")

			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}

func TestE2E_ArchivedResourceBlocksPromotion(t *testing.T) {
	feature := features.New("archived resource blocks promotion").
		Assess("removing production database archives it and blocks dependent promotions", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Skip("awaiting Phase 18: archive lifecycle and retention policies")

			// Remove a production database → archived
			// Promote compute that depends on the database → rejected
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}

func TestE2E_AdminDeleteArchivedResource(t *testing.T) {
	feature := features.New("admin delete archived resource").
		Assess("archived stateful resource requires explicit admin delete after retention", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Skip("awaiting Phase 18: archive lifecycle and retention policies")

			// chorister admin resource delete --archived <name>
			// Assert: resource deleted only after retention window
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}
