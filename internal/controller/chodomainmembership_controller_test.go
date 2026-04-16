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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var _ = Describe("ChoDomainMembership Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		chodomainmembership := &choristerv1alpha1.ChoDomainMembership{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ChoDomainMembership")
			err := k8sClient.Get(ctx, typeNamespacedName, chodomainmembership)
			if err != nil && errors.IsNotFound(err) {
				resource := &choristerv1alpha1.ChoDomainMembership{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: choristerv1alpha1.ChoDomainMembershipSpec{
						Application: "test-app",
						Domain:      "payments",
						Identity:    "alice@example.com",
						Role:        "developer",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &choristerv1alpha1.ChoDomainMembership{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ChoDomainMembership")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ChoDomainMembershipReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// -----------------------------------------------------------------------
	// 1A.11 — RBAC & membership (envtest)
	// -----------------------------------------------------------------------

	Context("1A.11 — RBAC & membership", func() {
		var (
			ctx        = context.Background()
			reconciler *ChoDomainMembershipReconciler
		)

		// Helper to create the test application and its domain namespace
		setupAppAndNamespace := func(appName, domainName string, sensitivity string) func() {
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: appName, Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"developer"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: domainName, Sensitivity: sensitivity},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())

			// Create domain namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: fmt.Sprintf("%s-%s", appName, domainName),
					Labels: map[string]string{
						labelApplication: appName,
						labelDomain:      domainName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())

			return func() {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, app)
				controllerutil.RemoveFinalizer(app, applicationFinalizerName)
				_ = k8sClient.Update(ctx, app)
				_ = k8sClient.Delete(ctx, app)
				_ = k8sClient.Delete(ctx, ns)
			}
		}

		BeforeEach(func() {
			reconciler = &ChoDomainMembershipReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		})

		It("should create edit RoleBinding for developer", func() {
			cleanup := setupAppAndNamespace("rbac-dev-app", "payments", "internal")
			defer cleanup()

			membership := &choristerv1alpha1.ChoDomainMembership{
				ObjectMeta: metav1.ObjectMeta{Name: "dev-membership", Namespace: "default"},
				Spec: choristerv1alpha1.ChoDomainMembershipSpec{
					Application: "rbac-dev-app",
					Domain:      "payments",
					Identity:    "alice@example.com",
					Role:        "developer",
				},
			}
			Expect(k8sClient.Create(ctx, membership)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, membership) }()

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Check that the prod-view RoleBinding was created in the domain namespace
			prodRb := &rbacv1.RoleBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "membership-dev-membership-prod", Namespace: "rbac-dev-app-payments",
			}, prodRb)).To(Succeed())
			Expect(prodRb.RoleRef.Name).To(Equal("view"))

			// Status should be Active
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace}, membership)).To(Succeed())
			Expect(membership.Status.Phase).To(Equal("Active"))
		})

		It("should create view RoleBinding for viewer", func() {
			cleanup := setupAppAndNamespace("rbac-viewer-app", "auth", "internal")
			defer cleanup()

			membership := &choristerv1alpha1.ChoDomainMembership{
				ObjectMeta: metav1.ObjectMeta{Name: "viewer-membership", Namespace: "default"},
				Spec: choristerv1alpha1.ChoDomainMembershipSpec{
					Application: "rbac-viewer-app",
					Domain:      "auth",
					Identity:    "bob@example.com",
					Role:        "viewer",
				},
			}
			Expect(k8sClient.Create(ctx, membership)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, membership) }()

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Prod binding should be view
			prodRb := &rbacv1.RoleBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "membership-viewer-membership-prod", Namespace: "rbac-viewer-app-auth",
			}, prodRb)).To(Succeed())
			Expect(prodRb.RoleRef.Name).To(Equal("view"))
			Expect(prodRb.Subjects[0].Name).To(Equal("bob@example.com"))
		})

		It("should grant view-only in production namespace for all human roles", func() {
			cleanup := setupAppAndNamespace("rbac-prod-app", "data", "internal")
			defer cleanup()

			// Create an org-admin membership
			membership := &choristerv1alpha1.ChoDomainMembership{
				ObjectMeta: metav1.ObjectMeta{Name: "admin-prod-membership", Namespace: "default"},
				Spec: choristerv1alpha1.ChoDomainMembershipSpec{
					Application: "rbac-prod-app",
					Domain:      "data",
					Identity:    "admin@example.com",
					Role:        "org-admin",
				},
			}
			Expect(k8sClient.Create(ctx, membership)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, membership) }()

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Production namespace should have a view-only RoleBinding
			prodRb := &rbacv1.RoleBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "membership-admin-prod-membership-prod", Namespace: "rbac-prod-app-data",
			}, prodRb)).To(Succeed())
			Expect(prodRb.RoleRef.Name).To(Equal("view"))
		})

		It("should delete RoleBinding when membership expires", func() {
			cleanup := setupAppAndNamespace("rbac-expiry-app", "billing", "internal")
			defer cleanup()

			pastTime := metav1.NewTime(time.Now().Add(-24 * time.Hour))
			membership := &choristerv1alpha1.ChoDomainMembership{
				ObjectMeta: metav1.ObjectMeta{Name: "expired-membership", Namespace: "default"},
				Spec: choristerv1alpha1.ChoDomainMembershipSpec{
					Application: "rbac-expiry-app",
					Domain:      "billing",
					Identity:    "bob@example.com",
					Role:        "developer",
					ExpiresAt:   &pastTime,
				},
			}
			Expect(k8sClient.Create(ctx, membership)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, membership) }()

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Re-fetch and check status
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace}, membership)).To(Succeed())
			Expect(membership.Status.Phase).To(Equal("Expired"))

			// No RoleBindings should exist for this membership
			rbList := &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(ctx, rbList, client.MatchingLabels{labelMembership: membership.Name})).To(Succeed())
			Expect(rbList.Items).To(BeEmpty())
		})

		It("should reject restricted domain membership without expiresAt", func() {
			cleanup := setupAppAndNamespace("rbac-restricted-app", "secrets", "restricted")
			defer cleanup()

			membership := &choristerv1alpha1.ChoDomainMembership{
				ObjectMeta: metav1.ObjectMeta{Name: "restricted-no-expiry", Namespace: "default"},
				Spec: choristerv1alpha1.ChoDomainMembershipSpec{
					Application: "rbac-restricted-app",
					Domain:      "secrets",
					Identity:    "charlie@example.com",
					Role:        "developer",
					// No ExpiresAt — should be rejected
				},
			}
			Expect(k8sClient.Create(ctx, membership)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, membership) }()

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Status should show error
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace}, membership)).To(Succeed())
			Expect(membership.Status.Phase).To(BeEmpty())

			// Check conditions for ExpiryRequired
			var found bool
			for _, c := range membership.Status.Conditions {
				if c.Type == "Ready" && c.Reason == "ExpiryRequired" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected ExpiryRequired condition")
		})

		It("should deprovision bindings when OIDC group removes subject", func() {
			cleanup := setupAppAndNamespace("rbac-oidc-app", "sync", "internal")
			defer cleanup()

			// Create a membership sourced from an OIDC group
			membership := &choristerv1alpha1.ChoDomainMembership{
				ObjectMeta: metav1.ObjectMeta{Name: "oidc-membership", Namespace: "default"},
				Spec: choristerv1alpha1.ChoDomainMembershipSpec{
					Application: "rbac-oidc-app",
					Domain:      "sync",
					Identity:    "alice@example.com",
					Role:        "developer",
					Source:      "oidc-group",
					OIDCGroup:   "team-payments",
				},
			}
			Expect(k8sClient.Create(ctx, membership)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, membership) }()

			// Phase 1: identity IS in the OIDC group → RoleBindings should be created
			mockChecker := &mockOIDCGroupChecker{members: map[string]map[string]bool{
				"team-payments": {"alice@example.com": true},
			}}
			reconciler.OIDCGroupChecker = mockChecker

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5*time.Minute), "OIDC memberships should requeue for periodic sync")

			// Verify RoleBindings were created
			rbList := &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(ctx, rbList, client.MatchingLabels{labelMembership: membership.Name})).To(Succeed())
			Expect(rbList.Items).NotTo(BeEmpty())

			// Verify status is Active
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace}, membership)).To(Succeed())
			Expect(membership.Status.Phase).To(Equal("Active"))

			// Phase 2: identity REMOVED from the OIDC group → RoleBindings should be deleted
			mockChecker.members["team-payments"]["alice@example.com"] = false

			result, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(5*time.Minute), "Deprovisioned OIDC memberships should still requeue")

			// Verify RoleBindings were deleted
			rbList = &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(ctx, rbList, client.MatchingLabels{labelMembership: membership.Name})).To(Succeed())
			Expect(rbList.Items).To(BeEmpty())

			// Verify status is Deprovisioned with correct condition
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace}, membership)).To(Succeed())
			Expect(membership.Status.Phase).To(Equal("Deprovisioned"))

			var found bool
			for _, c := range membership.Status.Conditions {
				if c.Type == "Ready" && c.Reason == "OIDCGroupRemoved" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected OIDCGroupRemoved condition")

			// Phase 3: identity RE-ADDED to the OIDC group → RoleBindings should be re-created
			mockChecker.members["team-payments"]["alice@example.com"] = true

			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify RoleBindings were re-created
			rbList = &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(ctx, rbList, client.MatchingLabels{labelMembership: membership.Name})).To(Succeed())
			Expect(rbList.Items).NotTo(BeEmpty())

			// Verify status is Active again
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace}, membership)).To(Succeed())
			Expect(membership.Status.Phase).To(Equal("Active"))
		})

		It("should create admin RoleBinding for org-admin", func() {
			cleanup := setupAppAndNamespace("rbac-admin-app", "core", "internal")
			defer cleanup()

			membership := &choristerv1alpha1.ChoDomainMembership{
				ObjectMeta: metav1.ObjectMeta{Name: "orgadmin-membership", Namespace: "default"},
				Spec: choristerv1alpha1.ChoDomainMembershipSpec{
					Application: "rbac-admin-app",
					Domain:      "core",
					Identity:    "admin@example.com",
					Role:        "org-admin",
				},
			}
			Expect(k8sClient.Create(ctx, membership)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, membership) }()

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// The domain-level binding should use admin ClusterRole
			rb := &rbacv1.RoleBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "membership-orgadmin-membership", Namespace: "rbac-admin-app-core",
			}, rb)).To(Succeed())
			Expect(rb.RoleRef.Name).To(Equal("admin"))

			// But the prod binding is still view
			prodRb := &rbacv1.RoleBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "membership-orgadmin-membership-prod", Namespace: "rbac-admin-app-core",
			}, prodRb)).To(Succeed())
			Expect(prodRb.RoleRef.Name).To(Equal("view"))
		})

		// -----------------------------------------------------------------------
		// Gap 21 — Sandbox ownership enforcement
		// -----------------------------------------------------------------------

		It("should only grant RoleBinding in sandbox owned by the member (Gap 21)", func() {
			cleanup := setupAppAndNamespace("gap21-app", "eng", "internal")
			defer cleanup()

			// Create two sandboxes: alice's and bob's
			aliceSbNs := "gap21-app-eng-sandbox-alice"
			bobSbNs := "gap21-app-eng-sandbox-bob"
			for _, ns := range []string{aliceSbNs, bobSbNs} {
				Expect(k8sClient.Create(ctx, &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: ns},
				})).To(Succeed())
			}
			defer func() {
				for _, ns := range []string{aliceSbNs, bobSbNs} {
					_ = k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}})
				}
			}()

			aliceSandbox := &choristerv1alpha1.ChoSandbox{
				ObjectMeta: metav1.ObjectMeta{Name: "gap21-alice-sb", Namespace: "default"},
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "gap21-app",
					Domain:      "eng",
					Name:        "alice",
					Owner:       "alice@example.com",
				},
			}
			bobSandbox := &choristerv1alpha1.ChoSandbox{
				ObjectMeta: metav1.ObjectMeta{Name: "gap21-bob-sb", Namespace: "default"},
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "gap21-app",
					Domain:      "eng",
					Name:        "bob",
					Owner:       "bob@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, aliceSandbox)).To(Succeed())
			Expect(k8sClient.Create(ctx, bobSandbox)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, aliceSandbox)
				_ = k8sClient.Delete(ctx, bobSandbox)
			}()

			// Alice's membership
			membership := &choristerv1alpha1.ChoDomainMembership{
				ObjectMeta: metav1.ObjectMeta{Name: "gap21-alice-membership", Namespace: "default"},
				Spec: choristerv1alpha1.ChoDomainMembershipSpec{
					Application: "gap21-app",
					Domain:      "eng",
					Identity:    "alice@example.com",
					Role:        "developer",
				},
			}
			Expect(k8sClient.Create(ctx, membership)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, membership) }()

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Alice should have a RoleBinding in her own sandbox
			aliceRb := &rbacv1.RoleBinding{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "membership-gap21-alice-membership", Namespace: aliceSbNs,
			}, aliceRb)).To(Succeed(), "alice should have RoleBinding in her own sandbox")

			// Alice should NOT have a RoleBinding in Bob's sandbox
			bobRb := &rbacv1.RoleBinding{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name: "membership-gap21-alice-membership", Namespace: bobSbNs,
			}, bobRb)
			Expect(errors.IsNotFound(err)).To(BeTrue(),
				"alice should NOT have a RoleBinding in bob's sandbox")
		})
	})
})

// mockOIDCGroupChecker is a test double for OIDCGroupChecker.
type mockOIDCGroupChecker struct {
	members map[string]map[string]bool // group → identity → isMember
}

func (m *mockOIDCGroupChecker) IsMember(_ context.Context, group, identity string) (bool, error) {
	if identities, ok := m.members[group]; ok {
		return identities[identity], nil
	}
	return false, nil
}
