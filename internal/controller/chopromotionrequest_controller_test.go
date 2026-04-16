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

package controller

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var _ = Describe("ChoPromotionRequest Controller", func() {

	// -----------------------------------------------------------------------
	// 1A.10 — Promotion lifecycle (envtest)
	// -----------------------------------------------------------------------

	Context("1A.10 — Promotion lifecycle", func() {
		It("should follow status lifecycle: Pending → Approved → Executing → Completed", func() {
			ctx := context.Background()

			// Create the ChoApplication that the promotion references
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "promo-lifecycle-app",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"admin@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion: choristerv1alpha1.PromotionPolicy{
							RequiredApprovers: 1,
							AllowedRoles:      []string{"domain-admin", "org-admin"},
						},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: "payments"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())

			// Ensure production namespace exists
			prodNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "promo-lifecycle-app-payments"},
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: prodNs.Name}, &corev1.Namespace{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, prodNs)).To(Succeed())
			}

			// Ensure sandbox namespace exists
			sandboxNs := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "promo-lifecycle-app-payments-sandbox-alice"},
			}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: sandboxNs.Name}, &corev1.Namespace{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, sandboxNs)).To(Succeed())
			}

			// Create a ChoCompute in the sandbox namespace to be promoted
			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api-server",
					Namespace: sandboxNs.Name,
				},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "promo-lifecycle-app",
					Domain:      "payments",
					Image:       "nginx:1.25",
					Replicas:    int32Ptr(2),
					Port:        int32Ptr(8080),
				},
			}
			Expect(k8sClient.Create(ctx, compute)).To(Succeed())

			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "promo-lifecycle",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: "promo-lifecycle-app",
					Domain:      "payments",
					Sandbox:     "alice",
					RequestedBy: "alice@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).To(Succeed())

			reconciler := &ChoPromotionRequestReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Initial reconcile → Pending
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			Expect(pr.Status.Phase).To(Equal("Pending"))

			// Add an approval
			pr.Status.Approvals = []choristerv1alpha1.PromotionApproval{
				{
					Approver:   "admin@example.com",
					Role:       "org-admin",
					ApprovedAt: metav1.Now(),
				},
			}
			Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())

			// Reconcile → Approved → Executing → Completed
			for range 5 {
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
				})
				Expect(err).NotTo(HaveOccurred())
			}

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			Expect(pr.Status.Phase).To(Equal("Completed"))

			// Verify the compute resource was copied to production namespace
			prodCompute := &choristerv1alpha1.ChoCompute{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "api-server",
				Namespace: "promo-lifecycle-app-payments",
			}, prodCompute)).To(Succeed())
			Expect(prodCompute.Spec.Image).To(Equal("nginx:1.25"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, pr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, app)).To(Succeed())
		})

		It("should stay Pending until required approvals met", func() {
			ctx := context.Background()

			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "promo-approval-app",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"admin@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion: choristerv1alpha1.PromotionPolicy{
							RequiredApprovers: 2,
							AllowedRoles:      []string{"domain-admin", "org-admin"},
						},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: "auth"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())

			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "promo-insufficient",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: "promo-approval-app",
					Domain:      "auth",
					Sandbox:     "bob",
					RequestedBy: "bob@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).To(Succeed())

			reconciler := &ChoPromotionRequestReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Initial → Pending
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Add only 1 approval (need 2)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			pr.Status.Approvals = []choristerv1alpha1.PromotionApproval{
				{
					Approver:   "admin1@example.com",
					Role:       "org-admin",
					ApprovedAt: metav1.Now(),
				},
			}
			Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())

			// Reconcile → still Pending
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			Expect(pr.Status.Phase).To(Equal("Pending"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, pr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, app)).To(Succeed())
		})

		It("should reject approval from disallowed role", func() {
			ctx := context.Background()

			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "promo-role-app",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"admin@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion: choristerv1alpha1.PromotionPolicy{
							RequiredApprovers: 1,
							AllowedRoles:      []string{"org-admin"},
						},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: "billing"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())

			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "promo-wrong-role",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: "promo-role-app",
					Domain:      "billing",
					Sandbox:     "carol",
					RequestedBy: "carol@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).To(Succeed())

			reconciler := &ChoPromotionRequestReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Initial → Pending
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Add approval from disallowed role "developer"
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			pr.Status.Approvals = []choristerv1alpha1.PromotionApproval{
				{
					Approver:   "dev@example.com",
					Role:       "developer", // not in allowedRoles
					ApprovedAt: metav1.Now(),
				},
			}
			Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())

			// Reconcile → still Pending (developer not counted)
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			Expect(pr.Status.Phase).To(Equal("Pending"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, pr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, app)).To(Succeed())
		})

		It("should copy compiled manifests on approval", func() {
			ctx := context.Background()

			// Create app with 1 required approver
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "promo-copy-app",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"admin@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion: choristerv1alpha1.PromotionPolicy{
							RequiredApprovers: 1,
							AllowedRoles:      []string{"org-admin"},
						},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: "api"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())

			// Create namespaces
			for _, nsName := range []string{"promo-copy-app-api", "promo-copy-app-api-sandbox-dev"} {
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &corev1.Namespace{})
				if errors.IsNotFound(err) {
					Expect(k8sClient.Create(ctx, ns)).To(Succeed())
				}
			}

			// Create resources in sandbox
			sandboxCompute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web",
					Namespace: "promo-copy-app-api-sandbox-dev",
				},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "promo-copy-app",
					Domain:      "api",
					Image:       "myapp:v2",
					Replicas:    int32Ptr(3),
					Port:        int32Ptr(8080),
				},
			}
			Expect(k8sClient.Create(ctx, sandboxCompute)).To(Succeed())

			// Create promotion request with approval already in place
			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "promo-copy-test",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: "promo-copy-app",
					Domain:      "api",
					Sandbox:     "dev",
					RequestedBy: "dev@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).To(Succeed())

			reconciler := &ChoPromotionRequestReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Reconcile → Pending
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Add approval
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			pr.Status.Approvals = []choristerv1alpha1.PromotionApproval{
				{Approver: "admin@example.com", Role: "org-admin", ApprovedAt: metav1.Now()},
			}
			Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())

			// Reconcile through full lifecycle
			for range 5 {
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Verify prod has the resource
			prodCompute := &choristerv1alpha1.ChoCompute{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "web",
				Namespace: "promo-copy-app-api",
			}, prodCompute)).To(Succeed())
			Expect(prodCompute.Spec.Image).To(Equal("myapp:v2"))
			Expect(*prodCompute.Spec.Replicas).To(Equal(int32(3)))

			// Cleanup
			Expect(k8sClient.Delete(ctx, pr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, app)).To(Succeed())
		})

		It("should store diff and compiled revision in request status", func() {
			ctx := context.Background()

			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "promo-rev-app",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"admin@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion: choristerv1alpha1.PromotionPolicy{
							RequiredApprovers: 1,
							AllowedRoles:      []string{"org-admin"},
						},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: "data"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())

			// Create namespaces
			for _, nsName := range []string{"promo-rev-app-data", "promo-rev-app-data-sandbox-rev"} {
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &corev1.Namespace{})
				if errors.IsNotFound(err) {
					Expect(k8sClient.Create(ctx, ns)).To(Succeed())
				}
			}

			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "promo-rev-test",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application:          "promo-rev-app",
					Domain:               "data",
					Sandbox:              "rev",
					RequestedBy:          "dev@example.com",
					CompiledWithRevision: "v1.2.3",
					Diff:                 "Added: Deployment/worker",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).To(Succeed())

			reconciler := &ChoPromotionRequestReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Reconcile → Pending
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Approve and complete
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			pr.Status.Approvals = []choristerv1alpha1.PromotionApproval{
				{Approver: "admin@example.com", Role: "org-admin", ApprovedAt: metav1.Now()},
			}
			Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())

			for range 5 {
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
				})
				Expect(err).NotTo(HaveOccurred())
			}

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			Expect(pr.Status.CompiledWithRevision).To(Equal("v1.2.3"))
			Expect(pr.Spec.Diff).To(Equal("Added: Deployment/worker"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, pr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, app)).To(Succeed())
		})

		It("should block promotion when domain is degraded", func() {
			ctx := context.Background()

			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "promo-iso-app",
					Namespace: "default",
					Annotations: map[string]string{
						"chorister.dev/isolate-payments": "true",
					},
				},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"admin@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion: choristerv1alpha1.PromotionPolicy{
							RequiredApprovers: 1,
							AllowedRoles:      []string{"org-admin"},
						},
					},
					Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, app) }()

			for _, nsName := range []string{"promo-iso-app-payments", "promo-iso-app-payments-sandbox-dev"} {
				Expect(ensureNamespaceExists(ctx, nsName)).To(Succeed())
			}

			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "promo-iso-test", Namespace: "default"},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: "promo-iso-app",
					Domain:      "payments",
					Sandbox:     "dev",
					RequestedBy: "dev@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pr) }()

			reconciler := &ChoPromotionRequestReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Reconcile → Pending
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Add approval
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			pr.Status.Approvals = []choristerv1alpha1.PromotionApproval{
				{Approver: "admin@example.com", Role: "org-admin", ApprovedAt: metav1.Now()},
			}
			Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())

			// Reconcile → Approved
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Reconcile → should fail with DomainIsolated
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			Expect(pr.Status.Phase).To(Equal("Failed"))

			// Verify condition reason is DomainIsolated
			var found bool
			for _, c := range pr.Status.Conditions {
				if c.Reason == "DomainIsolated" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected DomainIsolated condition reason")
		})

		It("should block promotion on critical image CVE", func() {
			ctx := context.Background()

			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "promo-scan-app", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"admin@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "standard",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"org-admin"}, RequireSecurityScan: true},
					},
					Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, app) }()

			for _, nsName := range []string{"promo-scan-app-payments", "promo-scan-app-payments-sandbox-dev"} {
				ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, &corev1.Namespace{})
				if errors.IsNotFound(err) {
					Expect(k8sClient.Create(ctx, ns)).To(Succeed())
				}
			}

			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "promo-scan-app-payments-sandbox-dev"},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "promo-scan-app",
					Domain:      "payments",
					Image:       "registry.example.com/vuln-critical:1.0",
					Replicas:    int32Ptr(1),
				},
			}
			Expect(k8sClient.Create(ctx, compute)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, compute) }()

			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "promo-scan", Namespace: "default"},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: "promo-scan-app",
					Domain:      "payments",
					Sandbox:     "dev",
					RequestedBy: "dev@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pr) }()

			reconciler := &ChoPromotionRequestReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			pr.Status.Approvals = []choristerv1alpha1.PromotionApproval{{Approver: "admin@example.com", Role: "org-admin", ApprovedAt: metav1.Now()}}
			Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			Expect(pr.Status.Phase).To(Equal("Rejected"))

			report := &choristerv1alpha1.ChoVulnerabilityReport{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "promo-scan-scan", Namespace: "default"}, report)).To(Succeed())
			Expect(report.Status.CriticalCount).To(BeNumerically(">", 0))
		})
	})

	// -----------------------------------------------------------------------
	// Gap 1 — Promotion copies ChoNetwork and ChoStorage resources
	// -----------------------------------------------------------------------
	Context("Gap 1 — Promotion copies all 6 resource types", func() {
		It("should copy ChoNetwork resources from sandbox to production", func() {
			ctx := context.Background()

			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "promo-net-app", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"admin@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"org-admin"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{{Name: "api"}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, app) }()

			for _, nsName := range []string{"promo-net-app-api", "promo-net-app-api-sandbox-dev"} {
				Expect(ensureNamespaceExists(ctx, nsName)).To(Succeed())
			}

			// Create ChoNetwork in sandbox
			network := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "api-boundary", Namespace: "promo-net-app-api-sandbox-dev"},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: "promo-net-app",
					Domain:      "api",
					Ingress: &choristerv1alpha1.NetworkIngressSpec{
						From: "internet",
						Port: 443,
					},
				},
			}
			Expect(k8sClient.Create(ctx, network)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, network) }()

			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "promo-net-test", Namespace: "default"},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: "promo-net-app",
					Domain:      "api",
					Sandbox:     "dev",
					RequestedBy: "dev@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pr) }()

			reconciler := &ChoPromotionRequestReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Reconcile → Pending
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			// Add approval
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			pr.Status.Approvals = []choristerv1alpha1.PromotionApproval{
				{Approver: "admin@example.com", Role: "org-admin", ApprovedAt: metav1.Now()},
			}
			Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())

			// Reconcile through full lifecycle
			for range 5 {
				_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}})
				Expect(err).NotTo(HaveOccurred())
			}

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			Expect(pr.Status.Phase).To(Equal("Completed"))

			// Verify ChoNetwork was copied to production
			prodNetwork := &choristerv1alpha1.ChoNetwork{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "api-boundary", Namespace: "promo-net-app-api"}, prodNetwork)).To(Succeed())
			Expect(prodNetwork.Spec.Ingress.Port).To(Equal(443))
		})

		It("should copy ChoStorage resources from sandbox to production", func() {
			ctx := context.Background()

			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "promo-stor-app", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"admin@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"org-admin"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{{Name: "data"}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, app) }()

			for _, nsName := range []string{"promo-stor-app-data", "promo-stor-app-data-sandbox-dev"} {
				Expect(ensureNamespaceExists(ctx, nsName)).To(Succeed())
			}

			// Create ChoStorage in sandbox
			storageSize := resource.MustParse("10Gi")
			storage := &choristerv1alpha1.ChoStorage{
				ObjectMeta: metav1.ObjectMeta{Name: "uploads", Namespace: "promo-stor-app-data-sandbox-dev"},
				Spec: choristerv1alpha1.ChoStorageSpec{
					Application: "promo-stor-app",
					Domain:      "data",
					Variant:     "object",
					Size:        &storageSize,
				},
			}
			Expect(k8sClient.Create(ctx, storage)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, storage) }()

			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "promo-stor-test", Namespace: "default"},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: "promo-stor-app",
					Domain:      "data",
					Sandbox:     "dev",
					RequestedBy: "dev@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pr) }()

			reconciler := &ChoPromotionRequestReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			pr.Status.Approvals = []choristerv1alpha1.PromotionApproval{
				{Approver: "admin@example.com", Role: "org-admin", ApprovedAt: metav1.Now()},
			}
			Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())

			for range 5 {
				_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}})
				Expect(err).NotTo(HaveOccurred())
			}

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			Expect(pr.Status.Phase).To(Equal("Completed"))

			// Verify ChoStorage was copied to production
			prodStorage := &choristerv1alpha1.ChoStorage{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "uploads", Namespace: "promo-stor-app-data"}, prodStorage)).To(Succeed())
			Expect(prodStorage.Spec.Variant).To(Equal("object"))
		})
	})

	// -----------------------------------------------------------------------
	// Gap 4 — ChoCache archiving during promotion
	// -----------------------------------------------------------------------
	// -----------------------------------------------------------------------
	// Gap 2 — Archived resource blocks dependent promotions
	// -----------------------------------------------------------------------
	Context("Gap 2 — Archived resource blocks dependent promotions", func() {
		It("should block promotion when production has archived dependencies", func() {
			ctx := context.Background()

			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "promo-arch-dep-app", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"admin@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"org-admin"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, app) }()

			prodNs := "promo-arch-dep-app-payments"
			sandboxNs := "promo-arch-dep-app-payments-sandbox-dev"
			for _, nsName := range []string{prodNs, sandboxNs} {
				Expect(ensureNamespaceExists(ctx, nsName)).To(Succeed())
			}

			// Create an Archived ChoDatabase in the production namespace
			archivedDB := &choristerv1alpha1.ChoDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "archive-db", Namespace: prodNs},
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "promo-arch-dep-app",
					Domain:      "payments",
					Engine:      "postgres",
				},
			}
			Expect(k8sClient.Create(ctx, archivedDB)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, archivedDB) }()

			// Set the database status to Archived
			now := metav1.Now()
			archivedDB.Status.Lifecycle = "Archived"
			archivedDB.Status.ArchivedAt = &now
			archivedDB.Status.Ready = false
			Expect(k8sClient.Status().Update(ctx, archivedDB)).To(Succeed())

			// Create a ChoCompute in sandbox referencing the archived database
			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{Name: "api-server", Namespace: sandboxNs},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "promo-arch-dep-app",
					Domain:      "payments",
					Image:       "nginx:1.25",
					Replicas:    int32Ptr(1),
				},
			}
			Expect(k8sClient.Create(ctx, compute)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, compute) }()

			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "promo-arch-dep-test", Namespace: "default"},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: "promo-arch-dep-app",
					Domain:      "payments",
					Sandbox:     "dev",
					RequestedBy: "dev@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pr) }()

			reconciler := &ChoPromotionRequestReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Reconcile → Pending
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Add approval
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			pr.Status.Approvals = []choristerv1alpha1.PromotionApproval{
				{Approver: "admin@example.com", Role: "org-admin", ApprovedAt: metav1.Now()},
			}
			Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())

			// Reconcile — should fail with ArchivedDependency
			for range 5 {
				_, err = reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
				})
				Expect(err).NotTo(HaveOccurred())
			}

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			Expect(pr.Status.Phase).To(Equal("Failed"))

			// Verify condition reason is ArchivedDependency
			var found bool
			for _, c := range pr.Status.Conditions {
				if c.Reason == "ArchivedDependency" {
					found = true
					Expect(c.Message).To(ContainSubstring("archive-db"))
					Expect(c.Message).To(ContainSubstring("archived"))
					break
				}
			}
			Expect(found).To(BeTrue(), "expected ArchivedDependency condition reason")
		})
	})

	Context("Gap 4 — ChoCache archive lifecycle during promotion", func() {
		It("should archive orphaned ChoCache resources in production", func() {
			ctx := context.Background()

			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "promo-cache-arch-app", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"admin@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"org-admin"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{{Name: "api"}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, app) }()

			prodNs := "promo-cache-arch-app-api"
			sandboxNs := "promo-cache-arch-app-api-sandbox-dev"
			for _, nsName := range []string{prodNs, sandboxNs} {
				Expect(ensureNamespaceExists(ctx, nsName)).To(Succeed())
			}

			// Create a ChoCache in production (simulating a previously promoted resource)
			prodCache := &choristerv1alpha1.ChoCache{
				ObjectMeta: metav1.ObjectMeta{Name: "sessions-cache", Namespace: prodNs},
				Spec: choristerv1alpha1.ChoCacheSpec{
					Application: "promo-cache-arch-app",
					Domain:      "api",
					Size:        "small",
				},
			}
			Expect(k8sClient.Create(ctx, prodCache)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, prodCache) }()

			// The sandbox does NOT have this cache (it was removed from DSL)
			// Create a different compute in sandbox so promotion has something to do
			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{Name: "api-server", Namespace: sandboxNs},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "promo-cache-arch-app",
					Domain:      "api",
					Image:       "nginx:1.25",
					Replicas:    int32Ptr(1),
				},
			}
			Expect(k8sClient.Create(ctx, compute)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, compute) }()

			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "promo-cache-arch-test", Namespace: "default"},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: "promo-cache-arch-app",
					Domain:      "api",
					Sandbox:     "dev",
					RequestedBy: "dev@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pr) }()

			reconciler := &ChoPromotionRequestReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			pr.Status.Approvals = []choristerv1alpha1.PromotionApproval{
				{Approver: "admin@example.com", Role: "org-admin", ApprovedAt: metav1.Now()},
			}
			Expect(k8sClient.Status().Update(ctx, pr)).To(Succeed())

			for range 5 {
				_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}})
				Expect(err).NotTo(HaveOccurred())
			}

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			Expect(pr.Status.Phase).To(Equal("Completed"))

			// Verify the orphaned ChoCache in production was archived
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "sessions-cache", Namespace: prodNs}, prodCache)).To(Succeed())
			Expect(prodCache.Status.Lifecycle).To(Equal("Archived"))
			Expect(prodCache.Status.ArchivedAt).NotTo(BeNil())
			Expect(prodCache.Status.DeletableAfter).NotTo(BeNil())
			Expect(prodCache.Status.Ready).To(BeFalse())
		})
	})
})

func int32Ptr(i int32) *int32 { return &i }

// cleanupNamespaces is a helper to clean up namespaces created during tests.
func cleanupNamespaces(ctx context.Context, names ...string) {
	for _, name := range names {
		ns := &corev1.Namespace{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, ns)
		if err == nil {
			_ = k8sClient.Delete(ctx, ns)
		}
	}
}

// ensureNamespaceExists creates a namespace if it doesn't exist.
func ensureNamespaceExists(ctx context.Context, name string) error {
	ns := &corev1.Namespace{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, ns)
	if errors.IsNotFound(err) {
		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
		return k8sClient.Create(ctx, ns)
	}
	return err
}

// listChoComputes lists all ChoCompute resources in a namespace.
func listChoComputes(ctx context.Context, namespace string) (*choristerv1alpha1.ChoComputeList, error) {
	list := &choristerv1alpha1.ChoComputeList{}
	err := k8sClient.List(ctx, list, client.InNamespace(namespace))
	return list, err
}

// printResources is a debug helper to dump resources in a namespace.
func printResources(ctx context.Context, namespace string) {
	list, err := listChoComputes(ctx, namespace)
	if err != nil {
		fmt.Printf("  error listing computes in %s: %v\n", namespace, err)
		return
	}
	for _, c := range list.Items {
		fmt.Printf("  ChoCompute: %s/%s image=%s\n", c.Namespace, c.Name, c.Spec.Image)
	}
}
