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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "netpol-consumes", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"developer"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{
							Name:     "payments",
							Consumes: []choristerv1alpha1.ConsumeRef{{Domain: "auth", Services: []string{"auth-svc"}, Port: 8080}},
						},
						{
							Name:     "auth",
							Supplies: &choristerv1alpha1.SupplySpec{Services: []string{"auth-svc"}, Port: 8080},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, app)
				controllerutil.RemoveFinalizer(app, "chorister.dev/application-cleanup")
				_ = k8sClient.Update(ctx, app)
				_ = k8sClient.Delete(ctx, app)
			}()

			reconciler := &ChoApplicationReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			// Two reconciles: add finalizer + create resources
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert egress allow policy in payments namespace
			egressPolicy := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "allow-egress-to-auth-8080", Namespace: "netpol-consumes-payments",
			}, egressPolicy)).To(Succeed())
			Expect(egressPolicy.Spec.Egress).To(HaveLen(1))
			Expect(egressPolicy.Spec.Egress[0].Ports).To(HaveLen(1))

			// Assert ingress allow policy in auth namespace
			ingressPolicy := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "allow-ingress-from-consumers-8080", Namespace: "netpol-consumes-auth",
			}, ingressPolicy)).To(Succeed())
			Expect(ingressPolicy.Spec.Ingress).To(HaveLen(1))
			Expect(ingressPolicy.Spec.Ingress[0].From).To(HaveLen(1))
		})

		It("should not generate allow-rule when no consumes declared", func() {
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "netpol-no-consumes", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"developer"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: "api"},
						{Name: "worker"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, app)
				controllerutil.RemoveFinalizer(app, "chorister.dev/application-cleanup")
				_ = k8sClient.Update(ctx, app)
				_ = k8sClient.Delete(ctx, app)
			}()

			reconciler := &ChoApplicationReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Only default-deny should exist, no consumes-egress policies
			npList := &networkingv1.NetworkPolicyList{}
			Expect(k8sClient.List(ctx, npList, client.InNamespace("netpol-no-consumes-api"),
				client.MatchingLabels{"chorister.dev/netpol-type": "consumes-egress"})).To(Succeed())
			Expect(npList.Items).To(BeEmpty())
		})

		It("should set error in status on supply mismatch", func() {
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "netpol-mismatch", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"developer"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{
							Name:     "payments",
							Consumes: []choristerv1alpha1.ConsumeRef{{Domain: "auth", Services: []string{"auth-svc"}, Port: 8080}},
						},
						{
							Name: "auth",
							// No Supplies - mismatch
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, app)
				controllerutil.RemoveFinalizer(app, "chorister.dev/application-cleanup")
				_ = k8sClient.Update(ctx, app)
				_ = k8sClient.Delete(ctx, app)
			}()

			reconciler := &ChoApplicationReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Check status has validation error
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, app)).To(Succeed())
			var validCondition *metav1.Condition
			for i := range app.Status.Conditions {
				if app.Status.Conditions[i].Type == "Valid" {
					validCondition = &app.Status.Conditions[i]
					break
				}
			}
			Expect(validCondition).NotTo(BeNil())
			Expect(validCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(validCondition.Message).To(ContainSubstring("does not declare supplies"))
		})

		It("should not allow access on wrong port", func() {
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "netpol-wrong-port", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"developer"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{
							Name:     "payments",
							Consumes: []choristerv1alpha1.ConsumeRef{{Domain: "auth", Services: []string{"auth-svc"}, Port: 8080}},
						},
						{
							Name:     "auth",
							Supplies: &choristerv1alpha1.SupplySpec{Services: []string{"auth-svc"}, Port: 9090}, // different port
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, app)
				controllerutil.RemoveFinalizer(app, "chorister.dev/application-cleanup")
				_ = k8sClient.Update(ctx, app)
				_ = k8sClient.Delete(ctx, app)
			}()

			reconciler := &ChoApplicationReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Port mismatch: no egress allow policy should be created
			egressPolicy := &networkingv1.NetworkPolicy{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name: "allow-egress-to-auth-8080", Namespace: "netpol-wrong-port-payments",
			}, egressPolicy)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			// Check validation error in status
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, app)).To(Succeed())
			var validCondition *metav1.Condition
			for i := range app.Status.Conditions {
				if app.Status.Conditions[i].Type == "Valid" {
					validCondition = &app.Status.Conditions[i]
					break
				}
			}
			Expect(validCondition).NotTo(BeNil())
			Expect(validCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(validCondition.Message).To(ContainSubstring("port"))
		})

		It("should always allow DNS egress on port 53", func() {
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "netpol-dns-test", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"developer"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: "web"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, app)
				controllerutil.RemoveFinalizer(app, "chorister.dev/application-cleanup")
				_ = k8sClient.Update(ctx, app)
				_ = k8sClient.Delete(ctx, app)
			}()

			reconciler := &ChoApplicationReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify default-deny policy allows DNS on port 53
			denyPolicy := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "default-deny", Namespace: "netpol-dns-test-web",
			}, denyPolicy)).To(Succeed())
			Expect(denyPolicy.Spec.Egress).To(HaveLen(1))
			Expect(denyPolicy.Spec.Egress[0].Ports).To(HaveLen(2)) // UDP + TCP for DNS
		})

		It("should generate CiliumNetworkPolicy for restricted domains", func() {
			Skip("awaiting Phase 6.3: CiliumNetworkPolicy for L7 filtering requires Cilium CRDs in envtest")
		})

		It("should generate CiliumNetworkPolicy FQDN rules for egress allowlist", func() {
			Skip("awaiting Phase 13.1: Egress allowlist enforcement")
		})

		It("should produce HTTPRoute + ReferenceGrant + deny policy for cross-app links", func() {
			Skip("awaiting Phase 13.3: Cross-application links via Gateway API")

			// Link produces HTTPRoute + ReferenceGrant + CiliumEnvoyConfig + direct-traffic deny
		})
	})
})
