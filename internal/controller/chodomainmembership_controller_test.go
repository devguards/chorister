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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			Namespace: "default", // TODO(user):Modify as needed
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
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
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
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})

	// -----------------------------------------------------------------------
	// 1A.11 — RBAC & membership (envtest)
	// -----------------------------------------------------------------------

	Context("1A.11 — RBAC & membership", func() {
		It("should create edit RoleBinding for developer", func() {
			Skip("awaiting Phase 9.1: ChoDomainMembership reconciler → RoleBinding")

			membership := &choristerv1alpha1.ChoDomainMembership{
				ObjectMeta: metav1.ObjectMeta{Name: "dev-membership", Namespace: "default"},
				Spec: choristerv1alpha1.ChoDomainMembershipSpec{
					Application: "myapp",
					Domain:      "payments",
					Identity:    "alice@example.com",
					Role:        "developer",
				},
			}
			Expect(k8sClient.Create(ctx, membership)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, membership) }()

			reconciler := &ChoDomainMembershipReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert edit RoleBinding in sandbox namespace
			rbList := &rbacv1.RoleBindingList{}
			Expect(k8sClient.List(ctx, rbList, client.InNamespace("myapp-payments"))).To(Succeed())
			Expect(rbList.Items).NotTo(BeEmpty())
		})

		It("should create view RoleBinding for viewer", func() {
			Skip("awaiting Phase 9.1: ChoDomainMembership reconciler → RoleBinding")

			// viewer → view RoleBinding
		})

		It("should grant view-only in production namespace for all human roles", func() {
			Skip("awaiting Phase 9.3: Production RBAC lockdown")

			// All human roles get view-only in production namespace
		})

		It("should delete RoleBinding when membership expires", func() {
			Skip("awaiting Phase 9.2: Membership expiry enforcement")

			pastTime := metav1.NewTime(time.Now().Add(-24 * time.Hour))
			membership := &choristerv1alpha1.ChoDomainMembership{
				ObjectMeta: metav1.ObjectMeta{Name: "expired-membership", Namespace: "default"},
				Spec: choristerv1alpha1.ChoDomainMembershipSpec{
					Application: "myapp",
					Domain:      "payments",
					Identity:    "bob@example.com",
					Role:        "developer",
					ExpiresAt:   &pastTime,
				},
			}
			Expect(k8sClient.Create(ctx, membership)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, membership) }()

			reconciler := &ChoDomainMembershipReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert RoleBinding removed, status shows expired
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: membership.Name, Namespace: membership.Namespace}, membership)).To(Succeed())
			Expect(membership.Status.Phase).To(Equal("Expired"))
		})

		It("should reject restricted domain membership without expiresAt", func() {
			Skip("awaiting Phase 9.2: Membership expiry enforcement")

			// Restricted domain membership without expiresAt is rejected
		})

		It("should deprovision bindings when OIDC group removes subject", func() {
			Skip("awaiting OIDC sync implementation")

			// Subject removed from synced OIDC group → membership/RoleBinding removed
		})

		It("should create admin RoleBinding for org-admin", func() {
			Skip("awaiting Phase 9.1: ChoDomainMembership reconciler → RoleBinding")

			// org-admin → admin RoleBinding
		})
	})
})
