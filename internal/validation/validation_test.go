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
	"strings"
	"testing"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// ---------------------------------------------------------------------------
// 1A.2 — Validation unit tests
// ---------------------------------------------------------------------------

// --- Consumes/Supplies ---

func TestValidateConsumesSupplies_Mismatch(t *testing.T) {
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

	errs := ValidateConsumesSupplies(app)
	if len(errs) == 0 {
		t.Fatal("expected validation error for consumes/supplies mismatch, got none")
	}
	found := false
	for _, e := range errs {
		if contains(e, "does not declare supplies") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected error about missing supplies, got: %v", errs)
	}
}

func TestValidateConsumesSupplies_OK(t *testing.T) {
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

	errs := ValidateConsumesSupplies(app)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

// --- Cycle detection ---

func TestValidateCycleDetection(t *testing.T) {
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

	err := ValidateCycleDetection(app)
	if err == nil {
		t.Fatal("expected cycle detection error, got nil")
	}
	if !contains(err.Error(), "cycle") {
		t.Fatalf("expected error mentioning cycle, got: %s", err.Error())
	}
}

func TestValidateCycleDetection_DAG(t *testing.T) {
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

	err := ValidateCycleDetection(app)
	if err != nil {
		t.Fatalf("expected no cycle error, got: %s", err.Error())
	}
}

// --- Ingress auth ---

func TestValidateIngressRequiresAuth(t *testing.T) {
	network := &choristerv1alpha1.ChoNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: "no-auth-ingress"},
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

	errs := ValidateIngressAuth(network)
	if len(errs) == 0 {
		t.Fatal("expected validation error for internet ingress without auth, got none")
	}
	if !contains(errs[0], "requires an auth block") {
		t.Fatalf("expected error about auth block, got: %v", errs)
	}
}

func TestValidateIngressRequiresAuth_NonInternet(t *testing.T) {
	network := &choristerv1alpha1.ChoNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: "internal-ingress"},
		Spec: choristerv1alpha1.ChoNetworkSpec{
			Application: "myapp",
			Domain:      "payments",
			Ingress: &choristerv1alpha1.NetworkIngressSpec{
				From: "internal",
				Port: 8080,
				// No Auth — OK for non-internet
			},
		},
	}

	errs := ValidateIngressAuth(network)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for non-internet ingress, got: %v", errs)
	}
}

func TestValidateIngressAllowedIdP(t *testing.T) {
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
		ObjectMeta: metav1.ObjectMeta{Name: "unapproved-idp"},
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

	errs := ValidateIngressAllowedIdP(network, appPolicy)
	if len(errs) == 0 {
		t.Fatal("expected validation error for unapproved IdP, got none")
	}
	if !contains(errs[0], "not in the application's allowed IdP list") {
		t.Fatalf("expected error about unapproved IdP, got: %v", errs)
	}
	if !contains(errs[0], "https://approved.idp.com") {
		t.Fatalf("expected error to include allowed IdPs, got: %v", errs)
	}
}

func TestValidateIngressAllowedIdP_Approved(t *testing.T) {
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
		ObjectMeta: metav1.ObjectMeta{Name: "approved-idp"},
		Spec: choristerv1alpha1.ChoNetworkSpec{
			Application: "myapp",
			Domain:      "payments",
			Ingress: &choristerv1alpha1.NetworkIngressSpec{
				From: "internet",
				Port: 443,
				Auth: &choristerv1alpha1.NetworkAuthSpec{
					JWT: &choristerv1alpha1.JWTAuthSpec{
						Issuer:  "https://approved.idp.com",
						JWKSUri: "https://approved.idp.com/.well-known/jwks.json",
					},
				},
			},
		},
	}

	errs := ValidateIngressAllowedIdP(network, appPolicy)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for approved IdP, got: %v", errs)
	}
}

// --- Egress ---

func TestValidateEgressWildcard(t *testing.T) {
	network := &choristerv1alpha1.ChoNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: "wildcard-egress"},
		Spec: choristerv1alpha1.ChoNetworkSpec{
			Application: "myapp",
			Domain:      "payments",
			Egress: &choristerv1alpha1.NetworkEgressSpec{
				Allowlist: []string{"*"},
			},
		},
	}

	errs := ValidateEgressWildcard(network)
	if len(errs) == 0 {
		t.Fatal("expected validation error for wildcard egress, got none")
	}
	if !contains(errs[0], "wildcard egress") {
		t.Fatalf("expected error about wildcard egress, got: %v", errs)
	}
}

func TestValidateEgressWildcard_Specific(t *testing.T) {
	network := &choristerv1alpha1.ChoNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: "specific-egress"},
		Spec: choristerv1alpha1.ChoNetworkSpec{
			Application: "myapp",
			Domain:      "payments",
			Egress: &choristerv1alpha1.NetworkEgressSpec{
				Allowlist: []string{"api.stripe.com"},
			},
		},
	}

	errs := ValidateEgressWildcard(network)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for specific egress, got: %v", errs)
	}
}

func TestValidateEgressUnapprovedDestination(t *testing.T) {
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

	errs := ValidateEgressAllowedDestinations(network, appPolicy)
	if len(errs) == 0 {
		t.Fatal("expected validation error for unapproved egress destination, got none")
	}
	if !contains(errs[0], "evil.example.com") {
		t.Fatalf("expected offending host in error, got: %v", errs)
	}
	if !contains(errs[0], "api.stripe.com") {
		t.Fatalf("expected approved host list in error, got: %v", errs)
	}
}

// --- Compliance ---

func TestValidateComplianceEscalation(t *testing.T) {
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

	errs := ValidateComplianceEscalation(app)
	if len(errs) == 0 {
		t.Fatal("expected validation error for compliance escalation, got none")
	}
	if !contains(errs[0], "weaker than application compliance") {
		t.Fatalf("expected error about weakened compliance, got: %v", errs)
	}
}

func TestValidateComplianceEscalation_OK(t *testing.T) {
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
					Name:        "payments",
					Sensitivity: "confidential", // stronger than essential — OK
				},
			},
		},
	}

	errs := ValidateComplianceEscalation(app)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

// --- Sizing templates ---

func TestValidateSizingTemplate_Undefined(t *testing.T) {
	// Use actual sizing templates from ChoCluster
	clusterTemplates := map[string]choristerv1alpha1.SizingTemplateSet{
		"database": {
			Templates: map[string]choristerv1alpha1.SizingTemplate{
				"small":  {CPU: resource.MustParse("250m"), Memory: resource.MustParse("512Mi")},
				"medium": {CPU: resource.MustParse("1"), Memory: resource.MustParse("2Gi")},
			},
		},
	}

	errs := ValidateSizingTemplate("nonexistent-size", "database", clusterTemplates)
	if len(errs) == 0 {
		t.Fatal("expected validation error for undefined size template, got none")
	}
	if !contains(errs[0], "undefined size") {
		t.Fatalf("expected error about undefined size, got: %v", errs)
	}
	if !contains(errs[0], "nonexistent-size") {
		t.Fatalf("expected error to include template name, got: %v", errs)
	}
}

func TestValidateSizingTemplate_ErrorMessage(t *testing.T) {
	clusterTemplates := map[string]choristerv1alpha1.SizingTemplateSet{
		"database": {
			Templates: map[string]choristerv1alpha1.SizingTemplate{
				"small":  {CPU: resource.MustParse("250m"), Memory: resource.MustParse("512Mi")},
				"medium": {CPU: resource.MustParse("1"), Memory: resource.MustParse("2Gi")},
				"large":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("8Gi")},
			},
		},
	}

	errs := ValidateSizingTemplate("jumbo", "database", clusterTemplates)
	if len(errs) == 0 {
		t.Fatal("expected validation error, got none")
	}
	// Error must include the template name and available options
	if !contains(errs[0], "jumbo") {
		t.Fatalf("expected error to include template name 'jumbo', got: %v", errs)
	}
	if !contains(errs[0], "small") || !contains(errs[0], "medium") || !contains(errs[0], "large") {
		t.Fatalf("expected error to include available options, got: %v", errs)
	}

	// Valid size should pass
	errs = ValidateSizingTemplate("medium", "database", clusterTemplates)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for valid size, got: %v", errs)
	}

	// Missing resource type should error
	errs = ValidateSizingTemplate("small", "nosuchtype", clusterTemplates)
	if len(errs) == 0 {
		t.Fatal("expected error for missing resource type, got none")
	}

	// Empty size should pass (no template reference)
	errs = ValidateSizingTemplate("", "database", clusterTemplates)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty size, got: %v", errs)
	}
}

func TestValidateQuotaExceeded(t *testing.T) {
	quota := &choristerv1alpha1.DomainQuota{
		CPU:    resource.MustParse("2"),
		Memory: resource.MustParse("4Gi"),
	}

	// Resource requests exceed namespace quota
	resources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("8"),
			corev1.ResourceMemory: resource.MustParse("16Gi"),
		},
	}

	errs := ValidateResourcesVsQuota(resources, quota, "ChoCompute/api")
	if len(errs) == 0 {
		t.Fatal("expected validation errors for quota exceeded, got none")
	}
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors (CPU + memory), got %d: %v", len(errs), errs)
	}
	if !contains(errs[0], "CPU") {
		t.Fatalf("expected CPU error, got: %s", errs[0])
	}
	if !contains(errs[1], "memory") {
		t.Fatalf("expected memory error, got: %s", errs[1])
	}

	// Within quota should pass
	withinQuota := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
	errs = ValidateResourcesVsQuota(withinQuota, quota, "ChoCompute/api")
	if len(errs) != 0 {
		t.Fatalf("expected no errors for within-quota resources, got: %v", errs)
	}

	// Nil resources or nil quota should pass
	errs = ValidateResourcesVsQuota(nil, quota, "ChoCompute/api")
	if len(errs) != 0 {
		t.Fatalf("expected no errors for nil resources, got: %v", errs)
	}
	errs = ValidateResourcesVsQuota(resources, nil, "ChoCompute/api")
	if len(errs) != 0 {
		t.Fatalf("expected no errors for nil quota, got: %v", errs)
	}
}

func TestValidateExplicitResourcesVsQuota(t *testing.T) {
	quota := &choristerv1alpha1.DomainQuota{
		CPU:     resource.MustParse("2"),
		Memory:  resource.MustParse("4Gi"),
		Storage: resource.MustParse("100Gi"),
	}

	// Explicit override bypasses template but still fails quota validation
	explicitResources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("8"),
			corev1.ResourceMemory: resource.MustParse("16Gi"),
		},
	}

	errs := ValidateExplicitResourcesVsQuota(explicitResources, quota, "ChoDatabase/main")
	if len(errs) == 0 {
		t.Fatal("expected errors for explicit resources exceeding quota, got none")
	}
	// Should include quota details
	foundQuotaInfo := false
	for _, e := range errs {
		if contains(e, "domain quota") {
			foundQuotaInfo = true
			break
		}
	}
	if !foundQuotaInfo {
		t.Fatalf("expected quota details in error message, got: %v", errs)
	}

	// Within quota should pass
	withinQuota := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
	errs = ValidateExplicitResourcesVsQuota(withinQuota, quota, "ChoDatabase/main")
	if len(errs) != 0 {
		t.Fatalf("expected no errors for within-quota resources, got: %v", errs)
	}

	// Nil explicit resources should pass
	errs = ValidateExplicitResourcesVsQuota(nil, quota, "ChoDatabase/main")
	if len(errs) != 0 {
		t.Fatalf("expected no errors for nil resources, got: %v", errs)
	}
}

// --- Archive lifecycle ---

func TestValidateArchivedResourceDependencies(t *testing.T) {
	// Archived databases and queues should be flagged
	databases := []choristerv1alpha1.ChoDatabase{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "prod-db"},
			Spec:       choristerv1alpha1.ChoDatabaseSpec{Application: "myapp", Domain: "payments", Engine: "postgres"},
			Status:     choristerv1alpha1.ChoDatabaseStatus{Lifecycle: "Archived"},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "active-db"},
			Spec:       choristerv1alpha1.ChoDatabaseSpec{Application: "myapp", Domain: "payments", Engine: "postgres"},
			Status:     choristerv1alpha1.ChoDatabaseStatus{Lifecycle: "Active"},
		},
	}
	queues := []choristerv1alpha1.ChoQueue{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "prod-queue"},
			Spec:       choristerv1alpha1.ChoQueueSpec{Application: "myapp", Domain: "payments"},
			Status:     choristerv1alpha1.ChoQueueStatus{Lifecycle: "Archived"},
		},
	}
	storages := []choristerv1alpha1.ChoStorage{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "active-storage"},
			Spec:       choristerv1alpha1.ChoStorageSpec{Application: "myapp", Domain: "payments", Variant: "block"},
			Status:     choristerv1alpha1.ChoStorageStatus{Lifecycle: "Active"},
		},
	}

	errs := ValidateArchivedResourceDependencies(databases, queues, storages)
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors (one archived db, one archived queue), got %d: %v", len(errs), errs)
	}
	if !contains(errs[0], "prod-db") {
		t.Fatalf("expected error to mention prod-db, got: %s", errs[0])
	}
	if !contains(errs[1], "prod-queue") {
		t.Fatalf("expected error to mention prod-queue, got: %s", errs[1])
	}

	// No archived resources → no errors
	activeDbs := []choristerv1alpha1.ChoDatabase{databases[1]}
	activeQueues := []choristerv1alpha1.ChoQueue{}
	activeStorages := []choristerv1alpha1.ChoStorage{storages[0]}
	errs = ValidateArchivedResourceDependencies(activeDbs, activeQueues, activeStorages)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for all-active resources, got: %v", errs)
	}
}

func TestValidateArchiveRetentionMinimum(t *testing.T) {
	// Archive retention below 30 days should error
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

	errs := ValidateArchiveRetentionMinimum(app)
	if len(errs) == 0 {
		t.Fatal("expected validation error for archive retention below 30 days, got none")
	}
	if !contains(errs[0], "below minimum 30 days") {
		t.Fatalf("expected error about minimum retention, got: %v", errs)
	}

	// 30 days should pass
	app.Spec.Policy.ArchiveRetention = "30d"
	errs = ValidateArchiveRetentionMinimum(app)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for 30d retention, got: %v", errs)
	}

	// 1 year should pass
	app.Spec.Policy.ArchiveRetention = "1y"
	errs = ValidateArchiveRetentionMinimum(app)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for 1y retention, got: %v", errs)
	}

	// Empty (default) should pass
	app.Spec.Policy.ArchiveRetention = ""
	errs = ValidateArchiveRetentionMinimum(app)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty retention (30d default), got: %v", errs)
	}
}

// --- Membership ---

func TestValidateRestrictedMembershipExpiryRequired(t *testing.T) {
	membership := &choristerv1alpha1.ChoDomainMembership{
		ObjectMeta: metav1.ObjectMeta{Name: "no-expiry-membership"},
		Spec: choristerv1alpha1.ChoDomainMembershipSpec{
			Application: "myapp",
			Domain:      "payments",
			Identity:    "alice@example.com",
			Role:        "developer",
			// No ExpiresAt — should fail for restricted domain
		},
	}

	domainSensitivity := "restricted"

	errs := ValidateRestrictedMembershipExpiry(membership, domainSensitivity)
	if len(errs) == 0 {
		t.Fatal("expected validation error for restricted membership without expiry, got none")
	}
	if !contains(errs[0], "requires expiresAt") {
		t.Fatalf("expected error about expiresAt, got: %v", errs)
	}
}

func TestValidateRestrictedMembershipExpiryRequired_NonRestricted(t *testing.T) {
	membership := &choristerv1alpha1.ChoDomainMembership{
		ObjectMeta: metav1.ObjectMeta{Name: "internal-membership"},
		Spec: choristerv1alpha1.ChoDomainMembershipSpec{
			Application: "myapp",
			Domain:      "payments",
			Identity:    "alice@example.com",
			Role:        "developer",
		},
	}

	errs := ValidateRestrictedMembershipExpiry(membership, "internal")
	if len(errs) != 0 {
		t.Fatalf("expected no errors for non-restricted domain, got: %v", errs)
	}
}

// helper
func int32Ptr(v int32) *int32 { return &v }

// --- Auth=none on all routes ---

func TestValidateIngressAuthNoneAllRoutes_Rejected(t *testing.T) {
	network := &choristerv1alpha1.ChoNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: "all-routes-none"},
		Spec: choristerv1alpha1.ChoNetworkSpec{
			Application: "myapp",
			Domain:      "payments",
			Ingress: &choristerv1alpha1.NetworkIngressSpec{
				From: "internet",
				Port: 443,
				Auth: &choristerv1alpha1.NetworkAuthSpec{
					JWT: &choristerv1alpha1.JWTAuthSpec{
						Issuer:  "https://idp.example.com",
						JWKSUri: "https://idp.example.com/.well-known/jwks.json",
					},
				},
				Routes: []choristerv1alpha1.NetworkRouteSpec{
					{Path: "/api/*", Auth: "none"},
					{Path: "/healthz", Auth: "none"},
				},
			},
		},
	}

	errs := ValidateIngressAuthNoneAllRoutes(network)
	if len(errs) == 0 {
		t.Fatal("expected validation error when all routes have auth=none, got none")
	}
	if !contains(errs[0], "all routes override auth") {
		t.Fatalf("expected error about all routes auth=none, got: %v", errs)
	}
}

func TestValidateIngressAuthNoneAllRoutes_Accepted_MixedAuth(t *testing.T) {
	network := &choristerv1alpha1.ChoNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: "mixed-auth"},
		Spec: choristerv1alpha1.ChoNetworkSpec{
			Application: "myapp",
			Domain:      "payments",
			Ingress: &choristerv1alpha1.NetworkIngressSpec{
				From: "internet",
				Port: 443,
				Auth: &choristerv1alpha1.NetworkAuthSpec{
					JWT: &choristerv1alpha1.JWTAuthSpec{
						Issuer:  "https://idp.example.com",
						JWKSUri: "https://idp.example.com/.well-known/jwks.json",
					},
				},
				Routes: []choristerv1alpha1.NetworkRouteSpec{
					{Path: "/api/*"},                 // inherits default auth
					{Path: "/healthz", Auth: "none"}, // explicit anonymous
				},
			},
		},
	}

	errs := ValidateIngressAuthNoneAllRoutes(network)
	if len(errs) != 0 {
		t.Fatalf("expected no errors when some routes use default auth, got: %v", errs)
	}
}

func TestValidateIngressAuthNoneAllRoutes_Accepted_NoRoutes(t *testing.T) {
	network := &choristerv1alpha1.ChoNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: "no-routes"},
		Spec: choristerv1alpha1.ChoNetworkSpec{
			Application: "myapp",
			Domain:      "payments",
			Ingress: &choristerv1alpha1.NetworkIngressSpec{
				From: "internet",
				Port: 443,
				Auth: &choristerv1alpha1.NetworkAuthSpec{
					JWT: &choristerv1alpha1.JWTAuthSpec{
						Issuer:  "https://idp.example.com",
						JWKSUri: "https://idp.example.com/.well-known/jwks.json",
					},
				},
			},
		},
	}

	errs := ValidateIngressAuthNoneAllRoutes(network)
	if len(errs) != 0 {
		t.Fatalf("expected no errors when no routes are declared, got: %v", errs)
	}
}

func TestValidateIngressAuthNoneAllRoutes_Accepted_NonInternet(t *testing.T) {
	network := &choristerv1alpha1.ChoNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: "internal-all-none"},
		Spec: choristerv1alpha1.ChoNetworkSpec{
			Application: "myapp",
			Domain:      "payments",
			Ingress: &choristerv1alpha1.NetworkIngressSpec{
				From: "internal",
				Port: 8080,
				Routes: []choristerv1alpha1.NetworkRouteSpec{
					{Path: "/api/*", Auth: "none"},
					{Path: "/healthz", Auth: "none"},
				},
			},
		},
	}

	errs := ValidateIngressAuthNoneAllRoutes(network)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for non-internet ingress, got: %v", errs)
	}
}

func TestValidateIngressAuthNoneAllRoutes_Rejected_SingleRoute(t *testing.T) {
	network := &choristerv1alpha1.ChoNetwork{
		ObjectMeta: metav1.ObjectMeta{Name: "single-route-none"},
		Spec: choristerv1alpha1.ChoNetworkSpec{
			Application: "myapp",
			Domain:      "payments",
			Ingress: &choristerv1alpha1.NetworkIngressSpec{
				From: "internet",
				Port: 443,
				Auth: &choristerv1alpha1.NetworkAuthSpec{
					JWT: &choristerv1alpha1.JWTAuthSpec{
						Issuer:  "https://idp.example.com",
						JWKSUri: "https://idp.example.com/.well-known/jwks.json",
					},
				},
				Routes: []choristerv1alpha1.NetworkRouteSpec{
					{Path: "/api/*", Auth: "none"},
				},
			},
		},
	}

	errs := ValidateIngressAuthNoneAllRoutes(network)
	if len(errs) == 0 {
		t.Fatal("expected validation error when single route has auth=none, got none")
	}
}
