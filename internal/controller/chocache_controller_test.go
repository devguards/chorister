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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var _ = Describe("ChoCache Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-cache-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		chocache := &choristerv1alpha1.ChoCache{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ChoCache")
			err := k8sClient.Get(ctx, typeNamespacedName, chocache)
			if err != nil && errors.IsNotFound(err) {
				resource := &choristerv1alpha1.ChoCache{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: choristerv1alpha1.ChoCacheSpec{
						Application: "test-app",
						Domain:      "payments",
						Size:        "small",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &choristerv1alpha1.ChoCache{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ChoCache")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ChoCacheReconciler{
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
	// 1A.7 — ChoCache lifecycle (envtest)
	// -----------------------------------------------------------------------

	Context("1A.7 — ChoCache lifecycle", func() {
		It("should create Dragonfly Deployment and Service", func() {
			cache := &choristerv1alpha1.ChoCache{
				ObjectMeta: metav1.ObjectMeta{Name: "sessions", Namespace: "default"},
				Spec: choristerv1alpha1.ChoCacheSpec{
					Application: "myapp",
					Domain:      "auth",
					Size:        "small",
				},
			}
			Expect(k8sClient.Create(ctx, cache)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cache) }()

			reconciler := &ChoCacheReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert Deployment created
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "sessions", Namespace: "default"}, deploy)).To(Succeed())
			Expect(deploy.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deploy.Spec.Template.Spec.Containers[0].Image).To(ContainSubstring("dragonfly"))

			// Assert Service created
			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "sessions", Namespace: "default"}, svc)).To(Succeed())
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(6379)))
		})

		It("should map size to correct resource requests", func() {
			sizes := map[string]string{
				"small":  "128Mi",
				"medium": "512Mi",
				"large":  "1Gi",
			}

			for size, expectedMem := range sizes {
				cache := &choristerv1alpha1.ChoCache{
					ObjectMeta: metav1.ObjectMeta{Name: "cache-size-" + size, Namespace: "default"},
					Spec: choristerv1alpha1.ChoCacheSpec{
						Application: "myapp",
						Domain:      "auth",
						Size:        size,
					},
				}
				Expect(k8sClient.Create(ctx, cache)).To(Succeed())

				reconciler := &ChoCacheReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace},
				})
				Expect(err).NotTo(HaveOccurred())

				deploy := &appsv1.Deployment{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cache.Name, Namespace: "default"}, deploy)).To(Succeed())
				container := deploy.Spec.Template.Spec.Containers[0]
				memReq := container.Resources.Requests.Memory()
				Expect(memReq.String()).To(Equal(expectedMem))

				_ = k8sClient.Delete(ctx, cache)
			}
		})

		It("should create credential Secret for Redis connection", func() {
			cache := &choristerv1alpha1.ChoCache{
				ObjectMeta: metav1.ObjectMeta{Name: "sessions-cred", Namespace: "default"},
				Spec: choristerv1alpha1.ChoCacheSpec{
					Application: "myapp",
					Domain:      "auth",
					Size:        "small",
				},
			}
			Expect(k8sClient.Create(ctx, cache)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cache) }()

			reconciler := &ChoCacheReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "auth--cache--sessions-cred-credentials", Namespace: "default",
			}, secret)).To(Succeed())
			Expect(secret.Data).To(HaveKey("host"))
			Expect(secret.Data).To(HaveKey("port"))
			Expect(secret.Data).To(HaveKey("uri"))
			Expect(string(secret.Data["port"])).To(Equal("6379"))
		})

		It("should update status after reconciliation", func() {
			cache := &choristerv1alpha1.ChoCache{
				ObjectMeta: metav1.ObjectMeta{Name: "sessions-status", Namespace: "default"},
				Spec: choristerv1alpha1.ChoCacheSpec{
					Application: "myapp",
					Domain:      "auth",
					Size:        "small",
				},
			}
			Expect(k8sClient.Create(ctx, cache)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cache) }()

			reconciler := &ChoCacheReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace}, cache)).To(Succeed())
			Expect(cache.Status.Ready).To(BeTrue())
			Expect(cache.Status.CredentialsSecretRef).To(Equal("auth--cache--sessions-status-credentials"))
		})
	})
})
