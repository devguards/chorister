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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var _ = Describe("ChoNetwork Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		chonetwork := &choristerv1alpha1.ChoNetwork{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ChoNetwork")
			err := k8sClient.Get(ctx, typeNamespacedName, chonetwork)
			if err != nil && errors.IsNotFound(err) {
				resource := &choristerv1alpha1.ChoNetwork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &choristerv1alpha1.ChoNetwork{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ChoNetwork")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ChoNetworkReconciler{
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
	// 1A.8 — Network policy reconciliation (envtest)
	// -----------------------------------------------------------------------

	Context("1A.8 — Network policy reconciliation", func() {
		It("should generate allow-rule when consumes declares a port", func() {
			Skip("awaiting Phase 6.1: Compile consumes/supplies → NetworkPolicy")

			// A consumes B:8080 → A→B:8080 allowed via NetworkPolicy
			// Create ChoApplication with consumes/supplies → reconcile → check NetworkPolicy
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "netpol-app", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"developer"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: "payments", Consumes: []choristerv1alpha1.ConsumeRef{{Domain: "auth", Port: 8080}}},
						{Name: "auth", Supplies: &choristerv1alpha1.SupplySpec{Port: 8080}},
					},
				},
			}
			_ = app
			// TODO: Reconcile and check that NetworkPolicy allows payments→auth:8080
		})

		It("should not generate allow-rule when no consumes declared", func() {
			Skip("awaiting Phase 6.1: Compile consumes/supplies → NetworkPolicy")

			// No consumes → no NetworkPolicy allow-rule beyond deny-all
		})

		It("should set error in status on supply mismatch", func() {
			Skip("awaiting Phase 6.2: Supply/consume validation")

			// A consumes B but B doesn't supply → error in status
		})

		It("should not allow access on wrong port", func() {
			Skip("awaiting Phase 6.1: Compile consumes/supplies → NetworkPolicy")

			// A consumes B:8080 but B exposes 9090 only → no allow rule for undeclared port
		})

		It("should always allow DNS egress on port 53", func() {
			Skip("awaiting Phase 2.2: Default deny NetworkPolicy per namespace")

			// Generated deny-all policy preserves kube-dns egress on port 53
			npList := &networkingv1.NetworkPolicyList{}
			Expect(k8sClient.List(ctx, npList, client.InNamespace("default"))).To(Succeed())
			// TODO: Find the deny-all policy and verify DNS egress port 53 is allowed
		})

		It("should generate CiliumNetworkPolicy for restricted domains", func() {
			Skip("awaiting Phase 6.3: CiliumNetworkPolicy for L7 filtering")

			// sensitivity=restricted → CiliumNetworkPolicy with L7 rules
		})

		It("should generate CiliumNetworkPolicy FQDN rules for egress allowlist", func() {
			Skip("awaiting Phase 13.1: Egress allowlist enforcement")

			// App egress policy → CiliumNetworkPolicy FQDN rules
		})

		It("should produce HTTPRoute + ReferenceGrant + deny policy for cross-app links", func() {
			Skip("awaiting Phase 13.3: Cross-application links via Gateway API")

			// Link produces HTTPRoute + ReferenceGrant + CiliumEnvoyConfig + direct-traffic deny
		})
	})
})
