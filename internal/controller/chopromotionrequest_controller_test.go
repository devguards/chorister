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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var _ = Describe("ChoPromotionRequest Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		chopromotionrequest := &choristerv1alpha1.ChoPromotionRequest{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ChoPromotionRequest")
			err := k8sClient.Get(ctx, typeNamespacedName, chopromotionrequest)
			if err != nil && errors.IsNotFound(err) {
				resource := &choristerv1alpha1.ChoPromotionRequest{
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
			resource := &choristerv1alpha1.ChoPromotionRequest{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ChoPromotionRequest")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ChoPromotionRequestReconciler{
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
	// 1A.10 — Promotion lifecycle (envtest)
	// -----------------------------------------------------------------------

	Context("1A.10 — Promotion lifecycle", func() {
		It("should follow status lifecycle: Pending → Approved → Executing → Completed", func() {
			Skip("awaiting Phase 8.2: ChoPromotionRequest reconciler")

			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "promo-lifecycle", Namespace: "default"},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: "myapp",
					Domain:      "payments",
					Sandbox:     "alice",
					RequestedBy: "alice@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, pr) }()

			reconciler := &ChoPromotionRequestReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Initial reconcile → Pending
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pr.Name, Namespace: pr.Namespace}, pr)).To(Succeed())
			Expect(pr.Status.Phase).To(Equal("Pending"))
		})

		It("should stay Pending until required approvals met", func() {
			Skip("awaiting Phase 8.3: Approval gate enforcement")

			// Create promotion with policy requiring 2 approvers → 1 approval → still Pending
		})

		It("should reject approval from disallowed role", func() {
			Skip("awaiting Phase 8.3: Approval gate enforcement")

			// Approval from disallowed role does not satisfy policy
		})

		It("should copy compiled manifests on approval", func() {
			Skip("awaiting Phase 8.2: ChoPromotionRequest reconciler")

			// Production namespace updated on approval
		})

		It("should store diff and compiled revision in request status", func() {
			Skip("awaiting Phase 8.2: ChoPromotionRequest reconciler")

			// request/status captures resource diff and compiledWithRevision
		})

		It("should block promotion when domain is degraded", func() {
			Skip("awaiting Phase 15.3: Service health baseline and incident response")

			// Degraded domain → promotion rejected
		})

		It("should block promotion on critical image CVE", func() {
			Skip("awaiting Phase 14.1: Image scanning before promotion")

			// Critical CVE → promotion blocked
		})
	})
})
