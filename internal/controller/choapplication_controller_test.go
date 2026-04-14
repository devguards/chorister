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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var _ = Describe("ChoApplication Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		choapplication := &choristerv1alpha1.ChoApplication{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ChoApplication")
			err := k8sClient.Get(ctx, typeNamespacedName, choapplication)
			if err != nil && errors.IsNotFound(err) {
				resource := &choristerv1alpha1.ChoApplication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: choristerv1alpha1.ChoApplicationSpec{
						Owners: []string{"owner@example.com"},
						Policy: choristerv1alpha1.ApplicationPolicy{
							Compliance: "essential",
							Promotion: choristerv1alpha1.PromotionPolicy{
								RequiredApprovers: 1,
								AllowedRoles:      []string{"developer"},
							},
						},
						Domains: []choristerv1alpha1.DomainSpec{{
							Name: "payments",
						}},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &choristerv1alpha1.ChoApplication{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ChoApplication")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ChoApplicationReconciler{
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
	// 1A.4 — ChoApplication lifecycle (envtest)
	// -----------------------------------------------------------------------

	Context("1A.4 — ChoApplication lifecycle", func() {
		It("should create namespaces for each domain", func() {
			Skip("awaiting Phase 2.1: ChoApplication reconciler → namespace creation")

			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ns-test-app",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"developer"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: "payments"},
						{Name: "auth"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, app) }()

			reconciler := &ChoApplicationReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert 2 namespaces created with correct labels
			for _, domainName := range []string{"payments", "auth"} {
				ns := &corev1.Namespace{}
				nsName := app.Name + "-" + domainName
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: nsName}, ns)).To(Succeed())
				Expect(ns.Labels).To(HaveKeyWithValue("chorister.dev/application", app.Name))
				Expect(ns.Labels).To(HaveKeyWithValue("chorister.dev/domain", domainName))
			}
		})

		It("should delete namespaces when application is deleted", func() {
			Skip("awaiting Phase 2.1: ChoApplication reconciler → namespace creation")

			// Create app, reconcile → namespaces exist
			// Delete app → reconcile → namespaces cascade-deleted via owner refs
		})

		It("should handle domain add and remove", func() {
			Skip("awaiting Phase 2.1: ChoApplication reconciler → namespace creation")

			// Add domain → new namespace created
			// Remove domain → namespace deleted
		})

		It("should create default deny NetworkPolicy per namespace", func() {
			Skip("awaiting Phase 2.2: Default deny NetworkPolicy per namespace")

			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "netpol-test-app",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"developer"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, app) }()

			reconciler := &ChoApplicationReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert deny-all NetworkPolicy + DNS-allow exists
			npList := &networkingv1.NetworkPolicyList{}
			nsName := app.Name + "-payments"
			Expect(k8sClient.List(ctx, npList, client.InNamespace(nsName))).To(Succeed())
			Expect(npList.Items).NotTo(BeEmpty())
		})

		It("should create ResourceQuota from application policy", func() {
			Skip("awaiting Phase 2.3: Resource quota and LimitRange")

			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "quota-test-app",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"developer"}},
						Quotas: &choristerv1alpha1.QuotaPolicy{
							DefaultPerDomain: &choristerv1alpha1.DomainQuota{
								CPU:    resource.MustParse("4"),
								Memory: resource.MustParse("8Gi"),
							},
						},
					},
					Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, app) }()

			reconciler := &ChoApplicationReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert ResourceQuota exists in domain namespace
			rqList := &corev1.ResourceQuotaList{}
			nsName := app.Name + "-payments"
			Expect(k8sClient.List(ctx, rqList, client.InNamespace(nsName))).To(Succeed())
			Expect(rqList.Items).NotTo(BeEmpty())
		})

		It("should create LimitRange from application policy", func() {
			Skip("awaiting Phase 2.3: Resource quota and LimitRange")

			// Assert LimitRange exists in domain namespace matching app policy
		})
	})
})
