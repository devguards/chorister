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
// 1A.16 — Production safety (e2e)
// ---------------------------------------------------------------------------

func TestE2E_CannotApplyToProd(t *testing.T) {
	feature := features.New("cannot apply to production").
		Assess("chorister apply targeting production is rejected", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Skip("awaiting Phase 2: apply command enforcement + sandbox targeting")

			// Run: chorister apply --domain payments --sandbox production
			// Expect: exit code != 0, error contains "cannot apply to production"
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}

func TestE2E_PromotionRequiresApproval(t *testing.T) {
	feature := features.New("promotion requires approval").
		Assess("promotion with 0 approvals does not modify prod", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Skip("awaiting Phase 8: promotion lifecycle reconciler")

			// Create ChoPromotionRequest, do not approve
			// Assert production namespace is unchanged
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}

func TestE2E_ProductionRBACViewOnly(t *testing.T) {
	feature := features.New("production RBAC view-only").
		Assess("developer SA cannot create resources in production", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			t.Skip("awaiting Phase 9: RBAC reconciler + production lockdown")

			// Use a developer ServiceAccount to attempt creating a Deployment in prod namespace
			// Assert: forbidden
			return ctx
		}).
		Feature()

	testEnv.Test(t, feature)
}
