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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var _ = Describe("ChoCluster Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		chocluster := &choristerv1alpha1.ChoCluster{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ChoCluster")
			err := k8sClient.Get(ctx, typeNamespacedName, chocluster)
			if err != nil && errors.IsNotFound(err) {
				resource := &choristerv1alpha1.ChoCluster{
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
			resource := &choristerv1alpha1.ChoCluster{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ChoCluster")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ChoClusterReconciler{
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
	// 1A.12 — ChoCluster bootstrap (envtest)
	// -----------------------------------------------------------------------

	Context("1A.12 — ChoCluster bootstrap", func() {
		It("should trigger operator installations", func() {
			Skip("awaiting Phase 12.1: ChoCluster reconciler — operator lifecycle")

			cluster := &choristerv1alpha1.ChoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-cluster"},
				Spec: choristerv1alpha1.ChoClusterSpec{
					Operators: &choristerv1alpha1.OperatorVersions{
						Kro:         "latest",
						StackGres:   "latest",
						NATS:        "latest",
						Dragonfly:   "latest",
						CertManager: "latest",
						Gatekeeper:  "latest",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cluster) }()

			reconciler := &ChoClusterReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cluster.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert operator status tracked
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cluster.Name}, cluster)).To(Succeed())
			Expect(cluster.Status.OperatorStatus).NotTo(BeEmpty())
		})

		It("should reinstall deleted operator", func() {
			Skip("awaiting Phase 12.1: ChoCluster reconciler — operator lifecycle")

			// Deleted operator → controller reinstalls on next reconciliation
		})

		It("should make sizing templates available for resource compilation", func() {
			Skip("awaiting Phase 21.1: Sizing template definitions")

			cluster := &choristerv1alpha1.ChoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "sizing-cluster"},
				Spec: choristerv1alpha1.ChoClusterSpec{
					SizingTemplates: map[string]choristerv1alpha1.SizingTemplateSet{
						"database": {
							Templates: map[string]choristerv1alpha1.SizingTemplate{
								"small":  {CPU: resource.MustParse("250m"), Memory: resource.MustParse("512Mi")},
								"medium": {CPU: resource.MustParse("1"), Memory: resource.MustParse("2Gi")},
								"large":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("8Gi")},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cluster) }()

			// Assert sizing templates readable
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cluster.Name}, cluster)).To(Succeed())
			Expect(cluster.Spec.SizingTemplates).To(HaveKey("database"))
			Expect(cluster.Spec.SizingTemplates["database"].Templates).To(HaveKey("medium"))
		})

		It("should expose FinOps cost rates", func() {
			Skip("awaiting Phase 20.2: FinOps cost estimation engine")

			// Cost rates readable from ChoCluster spec
		})

		It("should install default sizing templates on bootstrap", func() {
			Skip("awaiting Phase 21.1: Sizing template definitions")

			// chorister setup / ChoCluster bootstrap creates baseline templates
		})

		It("should block reconciliation on audit write failure", func() {
			Skip("awaiting Phase 11.2: Audit event logging to Loki")

			// Synchronous audit sink failure marks reconcile as failed
		})
	})
})
