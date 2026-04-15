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
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/compiler"
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
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "restricted-cnp-app"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"sec@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "regulated",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 2, AllowedRoles: []string{"sre"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{
							Name:        "secrets",
							Sensitivity: "restricted",
							Supplies:    &choristerv1alpha1.SupplySpec{Port: 8443, Services: []string{"grpc"}},
						},
					},
				},
			}

			policy := compiler.CompileRestrictedDomainL7Policy(app, app.Spec.Domains[0])
			Expect(policy).NotTo(BeNil())
			Expect(policy.GetKind()).To(Equal("CiliumNetworkPolicy"))
			Expect(policy.GetNamespace()).To(Equal("restricted-cnp-app-secrets"))
			Expect(policy.GetName()).To(Equal("secrets-l7-restricted"))

			spec, ok := policy.Object["spec"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(spec).To(HaveKey("endpointSelector"))

			ingress, ok := spec["ingress"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(ingress).To(HaveLen(1))
		})

		It("should generate CiliumNetworkPolicy FQDN rules for egress allowlist", func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "egress-allow-payments"}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, ns) }()

			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "egress-app", Namespace: ns.Name},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "standard",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"developer"}},
						Network: &choristerv1alpha1.AppNetworkPolicy{
							Egress: &choristerv1alpha1.EgressPolicy{Allowlist: []choristerv1alpha1.EgressTarget{{Host: "api.stripe.com", Port: 443}}},
						},
					},
					Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, app) }()

			network := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "external", Namespace: ns.Name},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: app.Name,
					Domain:      "payments",
					Egress:      &choristerv1alpha1.NetworkEgressSpec{Allowlist: []string{"api.stripe.com"}},
				},
			}
			Expect(k8sClient.Create(ctx, network)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, network) }()

			reconciler := &ChoNetworkReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: network.Name, Namespace: network.Namespace}})
			Expect(err).NotTo(HaveOccurred())

			cnp := &unstructured.Unstructured{}
			cnp.SetAPIVersion("cilium.io/v2")
			cnp.SetKind("CiliumNetworkPolicy")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "external-egress", Namespace: ns.Name}, cnp)).To(Succeed())
			egress := cnp.Object["spec"].(map[string]interface{})["egress"].([]interface{})
			Expect(egress).To(HaveLen(2))
			fqdnRule := egress[1].(map[string]interface{})["toFQDNs"].([]interface{})
			Expect(fqdnRule[0].(map[string]interface{})["matchName"]).To(Equal("api.stripe.com"))
		})

		It("should produce HTTPRoute + ReferenceGrant + deny policy for cross-app links", func() {
			target := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "supplier-app", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners:  []string{"owner@example.com"},
					Policy:  choristerv1alpha1.ApplicationPolicy{Compliance: "standard", Promotion: choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"org-admin"}}},
					Domains: []choristerv1alpha1.DomainSpec{{Name: "auth"}},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())
			defer func() {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, target)
				controllerutil.RemoveFinalizer(target, applicationFinalizerName)
				_ = k8sClient.Update(ctx, target)
				_ = k8sClient.Delete(ctx, target)
			}()

			source := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "consumer-app", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners:  []string{"owner@example.com"},
					Policy:  choristerv1alpha1.ApplicationPolicy{Compliance: "standard", Promotion: choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"org-admin"}}},
					Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}},
					Links: []choristerv1alpha1.LinkSpec{{
						Name:         "auth-api",
						Target:       "supplier-app",
						TargetDomain: "auth",
						Port:         8443,
						Consumers:    []string{"payments"},
						Auth:         &choristerv1alpha1.LinkAuth{Type: "jwt"},
						RateLimit:    &choristerv1alpha1.LinkRateLimit{RequestsPerMinute: 100},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, source)).To(Succeed())
			defer func() {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: source.Name, Namespace: source.Namespace}, source)
				controllerutil.RemoveFinalizer(source, applicationFinalizerName)
				_ = k8sClient.Update(ctx, source)
				_ = k8sClient.Delete(ctx, source)
			}()

			reconciler := &ChoApplicationReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			for _, app := range []*choristerv1alpha1.ChoApplication{target, source} {
				_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace}})
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace}})
				Expect(err).NotTo(HaveOccurred())
			}

			httpRoute := &unstructured.Unstructured{}
			httpRoute.SetAPIVersion("gateway.networking.k8s.io/v1")
			httpRoute.SetKind("HTTPRoute")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "link-auth-api-payments", Namespace: "consumer-app-payments"}, httpRoute)).To(Succeed())

			referenceGrant := &unstructured.Unstructured{}
			referenceGrant.SetAPIVersion("gateway.networking.k8s.io/v1beta1")
			referenceGrant.SetKind("ReferenceGrant")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "link-auth-api-payments", Namespace: "supplier-app-auth"}, referenceGrant)).To(Succeed())

			envoyConfig := &unstructured.Unstructured{}
			envoyConfig.SetAPIVersion("cilium.io/v2")
			envoyConfig.SetKind("CiliumEnvoyConfig")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "link-auth-api-payments", Namespace: "consumer-app-payments"}, envoyConfig)).To(Succeed())

			directPolicy := &networkingv1.NetworkPolicy{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "link-auth-api-payments-deny-direct", Namespace: "consumer-app-payments"}, directPolicy)).To(Succeed())
		})
	})

	// -----------------------------------------------------------------------
	// 10.3 — Compile-time guardrails (envtest integration)
	// -----------------------------------------------------------------------

	Context("10.3 — Compile-time guardrails", func() {
		It("should reject internet ingress without auth block", func() {
			// Create namespace for the test
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "guardrail-noauth-payments"}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, ns) }()

			network := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "noauth-ingress", Namespace: "guardrail-noauth-payments"},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: "guardrail-noauth",
					Domain:      "payments",
					Ingress: &choristerv1alpha1.NetworkIngressSpec{
						From: "internet",
						Port: 443,
						// No Auth — should fail
					},
				},
			}
			Expect(k8sClient.Create(ctx, network)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, network) }()

			reconciler := &ChoNetworkReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: network.Name, Namespace: network.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Status should show validation failure
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: network.Name, Namespace: network.Namespace}, network)).To(Succeed())
			Expect(network.Status.Ready).To(BeFalse())

			var readyCondition *metav1.Condition
			for i := range network.Status.Conditions {
				if network.Status.Conditions[i].Type == "Ready" {
					readyCondition = &network.Status.Conditions[i]
					break
				}
			}
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal("ValidationFailed"))
			Expect(readyCondition.Message).To(ContainSubstring("requires an auth block"))
		})

		It("should accept internet ingress with auth block", func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "guardrail-auth-payments"}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, ns) }()

			network := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "auth-ingress", Namespace: "guardrail-auth-payments"},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: "guardrail-auth",
					Domain:      "payments",
					Ingress: &choristerv1alpha1.NetworkIngressSpec{
						From: "internet",
						Port: 443,
						Auth: &choristerv1alpha1.NetworkAuthSpec{
							JWT: &choristerv1alpha1.JWTAuthSpec{
								Issuer:  "https://auth.example.com",
								JWKSUri: "https://auth.example.com/.well-known/jwks.json",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, network)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, network) }()

			reconciler := &ChoNetworkReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: network.Name, Namespace: network.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: network.Name, Namespace: network.Namespace}, network)).To(Succeed())
			Expect(network.Status.Ready).To(BeTrue())
		})

		It("should reject wildcard egress", func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "guardrail-egress-payments"}}
			Expect(k8sClient.Create(ctx, ns)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, ns) }()

			network := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "wildcard-egress", Namespace: "guardrail-egress-payments"},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: "guardrail-egress",
					Domain:      "payments",
					Egress: &choristerv1alpha1.NetworkEgressSpec{
						Allowlist: []string{"*"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, network)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, network) }()

			reconciler := &ChoNetworkReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: network.Name, Namespace: network.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: network.Name, Namespace: network.Namespace}, network)).To(Succeed())
			Expect(network.Status.Ready).To(BeFalse())
			var readyCondition *metav1.Condition
			for i := range network.Status.Conditions {
				if network.Status.Conditions[i].Type == "Ready" {
					readyCondition = &network.Status.Conditions[i]
					break
				}
			}
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Message).To(ContainSubstring("wildcard egress"))
		})
	})

	// -----------------------------------------------------------------------
	// Gap 2 — Revision filtering on ChoNetwork controller
	// -----------------------------------------------------------------------
	Context("Gap 2 — Controller revision filtering", func() {
		It("should skip ChoNetwork in non-matching-revision namespace", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "network-rev-mismatch",
					Labels: map[string]string{LabelRevision: "2-0"},
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ns))).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "network-rev-mismatch"}, ns)).To(Succeed())
			if ns.Labels == nil {
				ns.Labels = map[string]string{}
			}
			ns.Labels[LabelRevision] = "2-0"
			Expect(k8sClient.Update(ctx, ns)).To(Succeed())

			network := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "rev-skip-net", Namespace: "network-rev-mismatch"},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: "myapp",
					Domain:      "api",
					Ingress: &choristerv1alpha1.NetworkIngressSpec{
						From: "internet",
						Port: 443,
					},
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, network))).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, network) }()

			reconciler := &ChoNetworkReconciler{
				Client:             k8sClient,
				Scheme:             k8sClient.Scheme(),
				ControllerRevision: "1-0",
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: network.Name, Namespace: network.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Should have skipped — status untouched
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: network.Name, Namespace: network.Namespace}, network)).To(Succeed())
			Expect(network.Status.Ready).To(BeFalse())
			Expect(network.Status.Conditions).To(BeEmpty())
		})
	})
})
