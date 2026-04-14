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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var _ = Describe("ChoSandbox Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		chosandbox := &choristerv1alpha1.ChoSandbox{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ChoSandbox")
			err := k8sClient.Get(ctx, typeNamespacedName, chosandbox)
			if err != nil && errors.IsNotFound(err) {
				resource := &choristerv1alpha1.ChoSandbox{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: choristerv1alpha1.ChoSandboxSpec{
						Application: "test-app",
						Domain:      "payments",
						Name:        "alice",
						Owner:       "alice@example.com",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &choristerv1alpha1.ChoSandbox{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ChoSandbox")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ChoSandboxReconciler{
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
	// 1A.9 — Sandbox lifecycle (envtest)
	// -----------------------------------------------------------------------

	Context("1A.9 — Sandbox lifecycle", func() {
		It("should create isolated sandbox namespace", func() {
			Skip("awaiting Phase 7.1: Sandbox creation and isolation")

			sandbox := &choristerv1alpha1.ChoSandbox{
				ObjectMeta: metav1.ObjectMeta{Name: "sandbox-test", Namespace: "default"},
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "myapp",
					Domain:      "payments",
					Name:        "alice",
					Owner:       "alice@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, sandbox)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, sandbox) }()

			reconciler := &ChoSandboxReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sandbox.Name, Namespace: sandbox.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert sandbox namespace created: {app}-{domain}-sandbox-{name}
			expectedNS := "myapp-payments-sandbox-alice"
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: expectedNS}, ns)).To(Succeed())
		})

		It("should copy domain config into sandbox namespace", func() {
			Skip("awaiting Phase 7.1: Sandbox creation and isolation")

			// Sandbox gets domain's compute/db/queue/cache specs
		})

		It("should have own NetworkPolicy", func() {
			Skip("awaiting Phase 7.1: Sandbox creation and isolation")

			// Sandbox has independent deny-all policy
			npList := &networkingv1.NetworkPolicyList{}
			Expect(k8sClient.List(ctx, npList, client.InNamespace("myapp-payments-sandbox-alice"))).To(Succeed())
			Expect(npList.Items).NotTo(BeEmpty())
		})

		It("should remove namespace and resources on destruction", func() {
			Skip("awaiting Phase 7.2: Sandbox destruction and cleanup")

			// Delete sandbox → namespace and all resources removed
		})

		It("should delete stateful resources immediately (no archive)", func() {
			Skip("awaiting Phase 18.4: Sandbox exemption from archive lifecycle")

			// DB in sandbox deleted immediately, no Archived state
		})

		It("should auto-destroy idle sandbox past threshold", func() {
			Skip("awaiting Phase 20.1: Sandbox idle detection and auto-destroy")

			// Idle past threshold → warning condition → destroyed
		})
	})
})
