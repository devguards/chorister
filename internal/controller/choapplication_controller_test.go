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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
			defer func() {
				// Clean up: remove finalizer and delete
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, app)
				controllerutil.RemoveFinalizer(app, applicationFinalizerName)
				_ = k8sClient.Update(ctx, app)
				_ = k8sClient.Delete(ctx, app)
			}()

			reconciler := &ChoApplicationReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Re-fetch after finalizer add, then reconcile again for the actual logic
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
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

			// Assert status updated
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, app)).To(Succeed())
			Expect(app.Status.DomainNamespaces).To(HaveLen(2))
			Expect(app.Status.DomainNamespaces).To(HaveKeyWithValue("payments", "ns-test-app-payments"))
			Expect(app.Status.DomainNamespaces).To(HaveKeyWithValue("auth", "ns-test-app-auth"))
			Expect(app.Status.Phase).To(Equal("Active"))
		})

		It("should delete namespaces when application is deleted", func() {
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "del-test-app",
					Namespace: "default",
				},
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

			reconciler := &ChoApplicationReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			// First reconcile adds finalizer
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())
			// Second reconcile creates namespaces
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Namespace should exist
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "del-test-app-web"}, ns)).To(Succeed())

			// Delete application
			Expect(k8sClient.Delete(ctx, app)).To(Succeed())

			// Reconcile handles finalizer cleanup
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Namespace should be deleted (or in Terminating phase)
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "del-test-app-web"}, ns)
			if err == nil {
				// In envtest, namespace deletion is asynchronous. Check it's terminating.
				Expect(ns.DeletionTimestamp).NotTo(BeNil())
			}
		})

		It("should handle domain add and remove", func() {
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "domain-change-app",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"developer"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: "api"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, app)
				controllerutil.RemoveFinalizer(app, applicationFinalizerName)
				_ = k8sClient.Update(ctx, app)
				_ = k8sClient.Delete(ctx, app)
			}()

			reconciler := &ChoApplicationReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			// Two reconciles: add finalizer + create resources
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})

			// Assert "api" namespace exists
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "domain-change-app-api"}, ns)).To(Succeed())

			// Add "frontend" domain, remove "api"
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, app)).To(Succeed())
			app.Spec.Domains = []choristerv1alpha1.DomainSpec{{Name: "frontend"}}
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: app.Name, Namespace: app.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// "frontend" namespace should exist
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "domain-change-app-frontend"}, ns)).To(Succeed())

			// "api" namespace should be deleted or terminating
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "domain-change-app-api"}, ns)
			if err == nil {
				Expect(ns.DeletionTimestamp).NotTo(BeNil())
			}
		})

		It("should create default deny NetworkPolicy per namespace", func() {
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
			defer func() {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, app)
				controllerutil.RemoveFinalizer(app, applicationFinalizerName)
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

			// Assert deny-all NetworkPolicy + DNS-allow exists
			npList := &networkingv1.NetworkPolicyList{}
			nsName := app.Name + "-payments"
			Expect(k8sClient.List(ctx, npList, client.InNamespace(nsName))).To(Succeed())
			Expect(npList.Items).NotTo(BeEmpty())

			np := npList.Items[0]
			Expect(np.Name).To(Equal("default-deny"))
			Expect(np.Spec.PolicyTypes).To(ContainElements(
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			))
			// Ingress should be empty (deny all)
			Expect(np.Spec.Ingress).To(BeEmpty())
			// Egress should allow DNS only
			Expect(np.Spec.Egress).To(HaveLen(1))
			Expect(np.Spec.Egress[0].Ports).To(HaveLen(2)) // UDP 53 + TCP 53
		})

		It("should create ResourceQuota from application policy", func() {
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
			defer func() {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, app)
				controllerutil.RemoveFinalizer(app, applicationFinalizerName)
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

			// Assert ResourceQuota exists in domain namespace
			rqList := &corev1.ResourceQuotaList{}
			nsName := app.Name + "-payments"
			Expect(k8sClient.List(ctx, rqList, client.InNamespace(nsName))).To(Succeed())
			Expect(rqList.Items).NotTo(BeEmpty())

			rq := rqList.Items[0]
			Expect(rq.Name).To(Equal("domain-quota"))
			Expect(rq.Spec.Hard[corev1.ResourceLimitsCPU]).To(Equal(resource.MustParse("4")))
			Expect(rq.Spec.Hard[corev1.ResourceLimitsMemory]).To(Equal(resource.MustParse("8Gi")))
		})

		It("should create LimitRange from application policy", func() {
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "lr-test-app",
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
					Domains: []choristerv1alpha1.DomainSpec{{Name: "api"}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: app.Name, Namespace: app.Namespace}, app)
				controllerutil.RemoveFinalizer(app, applicationFinalizerName)
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

			// Assert LimitRange exists in domain namespace
			lrList := &corev1.LimitRangeList{}
			nsName := app.Name + "-api"
			Expect(k8sClient.List(ctx, lrList, client.InNamespace(nsName))).To(Succeed())
			Expect(lrList.Items).NotTo(BeEmpty())

			lr := lrList.Items[0]
			Expect(lr.Name).To(Equal("domain-limit-range"))
			Expect(lr.Spec.Limits).To(HaveLen(1))
			Expect(lr.Spec.Limits[0].Type).To(Equal(corev1.LimitTypeContainer))
			Expect(lr.Spec.Limits[0].Default).To(HaveKey(corev1.ResourceCPU))
			Expect(lr.Spec.Limits[0].Default).To(HaveKey(corev1.ResourceMemory))
			Expect(lr.Spec.Limits[0].DefaultRequest).To(HaveKey(corev1.ResourceCPU))
			Expect(lr.Spec.Limits[0].DefaultRequest).To(HaveKey(corev1.ResourceMemory))
		})
	})
})
