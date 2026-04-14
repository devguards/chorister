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

package validation

import (
	"testing"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ---------------------------------------------------------------------------
// 1A.2 — Validation unit tests
// ---------------------------------------------------------------------------

// --- Consumes/Supplies ---

func TestValidateConsumesSupplies_Mismatch(t *testing.T) {
	t.Skip("awaiting Phase 6.2: Supply/consume validation")

	app := &choristerv1alpha1.ChoApplication{
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Owners: []string{"owner@example.com"},
			Policy: choristerv1alpha1.ApplicationPolicy{
				Compliance: "essential",
				Promotion: choristerv1alpha1.PromotionPolicy{
					RequiredApprovers: 1,
					AllowedRoles:      []string{"developer"},
				},
			},
			Domains: []choristerv1alpha1.DomainSpec{
				{
					Name: "payments",
					Consumes: []choristerv1alpha1.ConsumeRef{
						{Domain: "auth", Services: []string{"auth-svc"}, Port: 8080},
					},
				},
				{
					Name: "auth",
					// No Supplies defined — mismatch
				},
			},
		},
	}

	_ = app
	// TODO: Call validator, assert error indicating A consumes B but B does not supply
}

func TestValidateConsumesSupplies_OK(t *testing.T) {
	t.Skip("awaiting Phase 6.2: Supply/consume validation")

	app := &choristerv1alpha1.ChoApplication{
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Owners: []string{"owner@example.com"},
			Policy: choristerv1alpha1.ApplicationPolicy{
				Compliance: "essential",
				Promotion: choristerv1alpha1.PromotionPolicy{
					RequiredApprovers: 1,
					AllowedRoles:      []string{"developer"},
				},
			},
			Domains: []choristerv1alpha1.DomainSpec{
				{
					Name: "payments",
					Consumes: []choristerv1alpha1.ConsumeRef{
						{Domain: "auth", Services: []string{"auth-svc"}, Port: 8080},
					},
				},
				{
					Name: "auth",
					Supplies: &choristerv1alpha1.SupplySpec{
						Services: []string{"auth-svc"},
						Port:     8080,
					},
				},
			},
		},
	}

	_ = app
	// TODO: Call validator, assert no error
}

// --- Cycle detection ---

func TestValidateCycleDetection(t *testing.T) {
	t.Skip("awaiting Phase 6.2: Supply/consume validation — cycle detection")

	// A→B→C→A forms a cycle
	app := &choristerv1alpha1.ChoApplication{
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Owners: []string{"owner@example.com"},
			Policy: choristerv1alpha1.ApplicationPolicy{
				Compliance: "essential",
				Promotion: choristerv1alpha1.PromotionPolicy{
					RequiredApprovers: 1,
					AllowedRoles:      []string{"developer"},
				},
			},
			Domains: []choristerv1alpha1.DomainSpec{
				{
					Name:     "a",
					Consumes: []choristerv1alpha1.ConsumeRef{{Domain: "b", Port: 8080}},
					Supplies: &choristerv1alpha1.SupplySpec{Port: 8080},
				},
				{
					Name:     "b",
					Consumes: []choristerv1alpha1.ConsumeRef{{Domain: "c", Port: 8080}},
					Supplies: &choristerv1alpha1.SupplySpec{Port: 8080},
				},
				{
					Name:     "c",
					Consumes: []choristerv1alpha1.ConsumeRef{{Domain: "a", Port: 8080}},
					Supplies: &choristerv1alpha1.SupplySpec{Port: 8080},
				},
			},
		},
	}

	_ = app
	// TODO: Call validator, assert error with cycle path (a→b→c→a)
}

func TestValidateCycleDetection_DAG(t *testing.T) {
	t.Skip("awaiting Phase 6.2: Supply/consume validation — cycle detection")

	// A→B, A→C, B→C — acyclic
	app := &choristerv1alpha1.ChoApplication{
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Owners: []string{"owner@example.com"},
			Policy: choristerv1alpha1.ApplicationPolicy{
				Compliance: "essential",
				Promotion: choristerv1alpha1.PromotionPolicy{
					RequiredApprovers: 1,
					AllowedRoles:      []string{"developer"},
				},
			},
			Domains: []choristerv1alpha1.DomainSpec{
				{
					Name: "a",
					Consumes: []choristerv1alpha1.ConsumeRef{
						{Domain: "b", Port: 8080},
						{Domain: "c", Port: 8080},
					},
				},
				{
					Name:     "b",
					Consumes: []choristerv1alpha1.ConsumeRef{{Domain: "c", Port: 8080}},
					Supplies: &choristerv1alpha1.SupplySpec{Port: 8080},
				},
				{
					Name:     "c",
					Supplies: &choristerv1alpha1.SupplySpec{Port: 8080},
				},
			},
		},
	}

	_ = app
	// TODO: Call validator, assert no error
}

// --- Ingress auth ---

func TestValidateIngressRequiresAuth(t *testing.T) {
	t.Skip("awaiting Phase 10.3: Compile-time guardrails")

	network := &choristerv1alpha1.ChoNetwork{
		Spec: choristerv1alpha1.ChoNetworkSpec{
			Application: "myapp",
			Domain:      "payments",
			Ingress: &choristerv1alpha1.NetworkIngressSpec{
				From: "internet",
				Port: 443,
				// No Auth — should fail
			},
		},
	}

	_ = network
	// TODO: Assert compile error: internet ingress without auth block
}

func TestValidateIngressAllowedIdP(t *testing.T) {
	t.Skip("awaiting Phase 10.3: Compile-time guardrails")

	appPolicy := choristerv1alpha1.ApplicationPolicy{
		Compliance: "standard",
		Promotion: choristerv1alpha1.PromotionPolicy{
			RequiredApprovers: 1,
			AllowedRoles:      []string{"developer"},
		},
		Network: &choristerv1alpha1.AppNetworkPolicy{
			Ingress: &choristerv1alpha1.IngressPolicy{
				AllowedIdPs: []choristerv1alpha1.IdPReference{
					{Issuer: "https://approved.idp.com", JWKSUri: "https://approved.idp.com/.well-known/jwks.json"},
				},
			},
		},
	}

	network := &choristerv1alpha1.ChoNetwork{
		Spec: choristerv1alpha1.ChoNetworkSpec{
			Application: "myapp",
			Domain:      "payments",
			Ingress: &choristerv1alpha1.NetworkIngressSpec{
				From: "internet",
				Port: 443,
				Auth: &choristerv1alpha1.NetworkAuthSpec{
					JWT: &choristerv1alpha1.JWTAuthSpec{
						Issuer:  "https://unapproved.idp.com",
						JWKSUri: "https://unapproved.idp.com/.well-known/jwks.json",
					},
				},
			},
		},
	}

	_, _ = appPolicy, network
	// TODO: Assert compile error referencing unapproved IdP, message includes allowed IdPs
}

// --- Egress ---

func TestValidateEgressWildcard(t *testing.T) {
	t.Skip("awaiting Phase 10.3: Compile-time guardrails")

	network := &choristerv1alpha1.ChoNetwork{
		Spec: choristerv1alpha1.ChoNetworkSpec{
			Application: "myapp",
			Domain:      "payments",
			Egress: &choristerv1alpha1.NetworkEgressSpec{
				Allowlist: []string{"*"},
			},
		},
	}

	_ = network
	// TODO: Assert compile error: wildcard egress not permitted
}

func TestValidateEgressUnapprovedDestination(t *testing.T) {
	t.Skip("awaiting Phase 13.1: Egress allowlist enforcement")

	appPolicy := choristerv1alpha1.ApplicationPolicy{
		Compliance: "essential",
		Promotion: choristerv1alpha1.PromotionPolicy{
			RequiredApprovers: 1,
			AllowedRoles:      []string{"developer"},
		},
		Network: &choristerv1alpha1.AppNetworkPolicy{
			Egress: &choristerv1alpha1.EgressPolicy{
				Allowlist: []choristerv1alpha1.EgressTarget{
					{Host: "api.stripe.com", Port: 443},
				},
			},
		},
	}

	network := &choristerv1alpha1.ChoNetwork{
		Spec: choristerv1alpha1.ChoNetworkSpec{
			Application: "myapp",
			Domain:      "payments",
			Egress: &choristerv1alpha1.NetworkEgressSpec{
				Allowlist: []string{"api.stripe.com", "evil.example.com"},
			},
		},
	}

	_, _ = appPolicy, network
	// TODO: Assert compile error: evil.example.com not in application allowlist
}

// --- Compliance ---

func TestValidateComplianceEscalation(t *testing.T) {
	t.Skip("awaiting Phase 10.2: Compliance-profile-driven constraints")

	// Domain sensitivity cannot weaken app compliance
	app := &choristerv1alpha1.ChoApplication{
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Owners: []string{"owner@example.com"},
			Policy: choristerv1alpha1.ApplicationPolicy{
				Compliance: "regulated",
				Promotion: choristerv1alpha1.PromotionPolicy{
					RequiredApprovers: 2,
					AllowedRoles:      []string{"domain-admin"},
				},
			},
			Domains: []choristerv1alpha1.DomainSpec{
				{
					Name:        "payments",
					Sensitivity: "public", // weaker than regulated — should error
				},
			},
		},
	}

	_ = app
	// TODO: Assert validation error: domain sensitivity cannot weaken app compliance
}

// --- Sizing templates ---

func TestValidateSizingTemplate_Undefined(t *testing.T) {
	t.Skip("awaiting Phase 21.1: Sizing template definitions")

	db := &choristerv1alpha1.ChoDatabase{
		Spec: choristerv1alpha1.ChoDatabaseSpec{
			Application: "myapp",
			Domain:      "payments",
			Engine:      "postgres",
			Size:        "nonexistent-size",
		},
	}

	_ = db
	// TODO: Assert compile error: undefined size template
}

func TestValidateSizingTemplate_ErrorMessage(t *testing.T) {
	t.Skip("awaiting Phase 21.1: Sizing template definitions")

	// TODO: Assert error includes template name and available options
}

func TestValidateQuotaExceeded(t *testing.T) {
	t.Skip("awaiting Phase 2.3: Resource quota and LimitRange")

	appPolicy := choristerv1alpha1.ApplicationPolicy{
		Compliance: "essential",
		Promotion: choristerv1alpha1.PromotionPolicy{
			RequiredApprovers: 1,
			AllowedRoles:      []string{"developer"},
		},
		Quotas: &choristerv1alpha1.QuotaPolicy{
			DefaultPerDomain: &choristerv1alpha1.DomainQuota{
				CPU:    resource.MustParse("2"),
				Memory: resource.MustParse("4Gi"),
			},
		},
	}

	// Resource requests exceed namespace quota
	compute := &choristerv1alpha1.ChoCompute{
		Spec: choristerv1alpha1.ChoComputeSpec{
			Application: "myapp",
			Domain:      "payments",
			Image:       "myregistry/api:v1",
			Replicas:    int32Ptr(1),
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("8"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
			},
		},
	}

	_, _ = appPolicy, compute
	// TODO: Assert error: resources exceed namespace quota
}

func TestValidateExplicitResourcesVsQuota(t *testing.T) {
	t.Skip("awaiting Phase 21.2: Explicit resource override")

	// Explicit override bypasses template but still fails quota validation
	// TODO: Assert quota details in error message
}

// --- Archive lifecycle ---

func TestValidateArchivedResourceDependencies(t *testing.T) {
	t.Skip("awaiting Phase 18: Stateful resource deletion safety")

	// Compute referencing archived database should produce compile error
	// TODO: Assert compile error when referencing archived resources
}

func TestValidateArchiveRetentionMinimum(t *testing.T) {
	t.Skip("awaiting Phase 18: Stateful resource deletion safety")

	app := &choristerv1alpha1.ChoApplication{
		Spec: choristerv1alpha1.ChoApplicationSpec{
			Owners: []string{"owner@example.com"},
			Policy: choristerv1alpha1.ApplicationPolicy{
				Compliance: "essential",
				Promotion: choristerv1alpha1.PromotionPolicy{
					RequiredApprovers: 1,
					AllowedRoles:      []string{"developer"},
				},
				ArchiveRetention: "7d", // below 30d minimum
			},
			Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}},
		},
	}

	_ = app
	// TODO: Assert validation error: archive retention below 30 days
}

// --- Membership ---

func TestValidateRestrictedMembershipExpiryRequired(t *testing.T) {
	t.Skip("awaiting Phase 9.2: Membership expiry enforcement")

	membership := &choristerv1alpha1.ChoDomainMembership{
		Spec: choristerv1alpha1.ChoDomainMembershipSpec{
			Application: "myapp",
			Domain:      "payments",
			Identity:    "alice@example.com",
			Role:        "developer",
			// No ExpiresAt — should fail for restricted domain
		},
	}

	domainSensitivity := "restricted"

	_, _ = membership, domainSensitivity
	// TODO: Assert validation error: restricted domain membership without expiresAt
}

// helper
func int32Ptr(v int32) *int32 { return &v }
