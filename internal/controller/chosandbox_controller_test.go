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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var _ = Describe("ChoSandbox Controller", func() {

	// -----------------------------------------------------------------------
	// 1A.9 — Sandbox lifecycle (envtest)
	// -----------------------------------------------------------------------

	Context("1A.9 — Sandbox lifecycle", func() {
		It("should create isolated sandbox namespace", func() {
			ctx := context.Background()

			sandbox := &choristerv1alpha1.ChoSandbox{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sandbox-create-test",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "myapp",
					Domain:      "payments",
					Name:        "alice",
					Owner:       "alice@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, sandbox)).To(Succeed())

			reconciler := &ChoSandboxReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Reconcile multiple times (finalizer add, then actual reconcile)
			for i := 0; i < 3; i++ {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: sandbox.Name, Namespace: sandbox.Namespace},
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Assert sandbox namespace created: {app}-{domain}-sandbox-{name}
			expectedNS := "myapp-payments-sandbox-alice"
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: expectedNS}, ns)).To(Succeed())
			Expect(ns.Labels[labelApplication]).To(Equal("myapp"))
			Expect(ns.Labels[labelDomain]).To(Equal("payments"))
			Expect(ns.Labels[labelSandbox]).To(Equal("alice"))

			// Check status
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sandbox.Name, Namespace: sandbox.Namespace}, sandbox)).To(Succeed())
			Expect(sandbox.Status.Namespace).To(Equal(expectedNS))
			Expect(sandbox.Status.Phase).To(Equal("Active"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, sandbox)).To(Succeed())
		})

		It("should have own NetworkPolicy", func() {
			ctx := context.Background()

			sandbox := &choristerv1alpha1.ChoSandbox{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sandbox-netpol-test",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "myapp2",
					Domain:      "auth",
					Name:        "bob",
					Owner:       "bob@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, sandbox)).To(Succeed())

			reconciler := &ChoSandboxReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			for i := 0; i < 3; i++ {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: sandbox.Name, Namespace: sandbox.Namespace},
				})
				Expect(err).NotTo(HaveOccurred())
			}

			expectedNS := "myapp2-auth-sandbox-bob"
			npList := &networkingv1.NetworkPolicyList{}
			Expect(k8sClient.List(ctx, npList, client.InNamespace(expectedNS))).To(Succeed())
			Expect(npList.Items).NotTo(BeEmpty())

			// Verify the default-deny policy
			found := false
			for _, np := range npList.Items {
				if np.Name == "default-deny" {
					found = true
					Expect(np.Spec.PolicyTypes).To(ContainElements(
						networkingv1.PolicyTypeIngress,
						networkingv1.PolicyTypeEgress,
					))
				}
			}
			Expect(found).To(BeTrue(), "expected default-deny NetworkPolicy")

			// Cleanup
			Expect(k8sClient.Delete(ctx, sandbox)).To(Succeed())
		})

		It("should remove namespace and resources on destruction", func() {
			ctx := context.Background()

			sandbox := &choristerv1alpha1.ChoSandbox{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sandbox-destroy-test",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "myapp3",
					Domain:      "billing",
					Name:        "carol",
					Owner:       "carol@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, sandbox)).To(Succeed())

			reconciler := &ChoSandboxReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			for i := 0; i < 3; i++ {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: sandbox.Name, Namespace: sandbox.Namespace},
				})
				Expect(err).NotTo(HaveOccurred())
			}

			expectedNS := "myapp3-billing-sandbox-carol"
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: expectedNS}, ns)).To(Succeed())

			// Delete sandbox
			Expect(k8sClient.Delete(ctx, sandbox)).To(Succeed())

			// Reconcile deletion
			for i := 0; i < 3; i++ {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: sandbox.Name, Namespace: sandbox.Namespace},
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Namespace should be deleted (or marked for deletion)
			err := k8sClient.Get(ctx, types.NamespacedName{Name: expectedNS}, ns)
			if err == nil {
				// envtest may not fully delete namespaces, but DeletionTimestamp should be set
				Expect(ns.DeletionTimestamp).NotTo(BeNil())
			}
		})

		It("should delete stateful resources immediately (no archive)", func() {
			Skip("awaiting Phase 18.4: Sandbox exemption from archive lifecycle")
		})

		It("should auto-destroy idle sandbox past threshold", func() {
			Skip("awaiting Phase 20.1: Sandbox idle detection and auto-destroy")
		})
	})
})
