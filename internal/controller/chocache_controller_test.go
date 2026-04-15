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
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	// -----------------------------------------------------------------------
	// Gap 2 — Revision filtering on ChoCache controller
	// -----------------------------------------------------------------------
	Context("Gap 2 — Controller revision filtering", func() {
		It("should skip ChoCache in non-matching-revision namespace", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "cache-rev-mismatch",
					Labels: map[string]string{LabelRevision: "2-0"},
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ns))).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cache-rev-mismatch"}, ns)).To(Succeed())
			if ns.Labels == nil {
				ns.Labels = map[string]string{}
			}
			ns.Labels[LabelRevision] = "2-0"
			Expect(k8sClient.Update(ctx, ns)).To(Succeed())

			cache := &choristerv1alpha1.ChoCache{
				ObjectMeta: metav1.ObjectMeta{Name: "rev-skip-cache", Namespace: "cache-rev-mismatch"},
				Spec: choristerv1alpha1.ChoCacheSpec{
					Application: "myapp",
					Domain:      "auth",
					Size:        "small",
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, cache))).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cache) }()

			reconciler := &ChoCacheReconciler{
				Client:             k8sClient,
				Scheme:             k8sClient.Scheme(),
				ControllerRevision: "1-0",
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Should have skipped — no StatefulSet created
			sts := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace}, sts)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "expected no StatefulSet when revision mismatches")
		})
	})

	// -----------------------------------------------------------------------
	// D.4.2 — ChoCache persistence (PVC)
	// -----------------------------------------------------------------------
	Context("D.4.2 — ChoCache persistence", func() {
		It("should create StatefulSet with PVC when persistence is enabled", func() {
			cache := &choristerv1alpha1.ChoCache{
				ObjectMeta: metav1.ObjectMeta{Name: "cache-persist", Namespace: "default"},
				Spec: choristerv1alpha1.ChoCacheSpec{
					Application: "myapp",
					Domain:      "auth",
					Size:        "small",
					Persistence: &choristerv1alpha1.CachePersistenceSpec{
						Enabled: true,
						Size:    "2Gi",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cache)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cache) }()

			reconciler := &ChoCacheReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Should create StatefulSet, not Deployment
			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cache-persist", Namespace: "default"}, sts)).To(Succeed())
			Expect(sts.Spec.VolumeClaimTemplates).To(HaveLen(1))
			Expect(sts.Spec.VolumeClaimTemplates[0].Name).To(Equal("data"))

			storageReq := sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
			Expect(storageReq.String()).To(Equal("2Gi"))

			// Container should have volume mount and args
			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(1))
			Expect(container.VolumeMounts[0].MountPath).To(Equal("/data"))
			Expect(container.Args).To(ContainElements("--dir", "/data"))

			// Deployment should NOT exist
			deploy := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "cache-persist", Namespace: "default"}, deploy)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should use Deployment without PVC when persistence is disabled", func() {
			cache := &choristerv1alpha1.ChoCache{
				ObjectMeta: metav1.ObjectMeta{Name: "cache-no-persist", Namespace: "default"},
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

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cache-no-persist", Namespace: "default"}, deploy)).To(Succeed())

			// No volumes should be set
			Expect(deploy.Spec.Template.Spec.Volumes).To(BeEmpty())
		})

		It("should use custom StorageClass when specified", func() {
			cache := &choristerv1alpha1.ChoCache{
				ObjectMeta: metav1.ObjectMeta{Name: "cache-sc", Namespace: "default"},
				Spec: choristerv1alpha1.ChoCacheSpec{
					Application: "myapp",
					Domain:      "auth",
					Size:        "small",
					Persistence: &choristerv1alpha1.CachePersistenceSpec{
						Enabled:      true,
						Size:         "5Gi",
						StorageClass: "fast-ssd",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cache)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cache) }()

			reconciler := &ChoCacheReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cache-sc", Namespace: "default"}, sts)).To(Succeed())
			Expect(sts.Spec.VolumeClaimTemplates[0].Spec.StorageClassName).To(HaveValue(Equal("fast-ssd")))
		})
	})

	// -----------------------------------------------------------------------
	// D.4.3 — ChoCache HA replica support
	// -----------------------------------------------------------------------
	Context("D.4.3 — ChoCache HA replicas", func() {
		It("should create StatefulSet with 2 replicas when HA is enabled", func() {
			cache := &choristerv1alpha1.ChoCache{
				ObjectMeta: metav1.ObjectMeta{Name: "cache-ha", Namespace: "default"},
				Spec: choristerv1alpha1.ChoCacheSpec{
					Application: "myapp",
					Domain:      "auth",
					Size:        "medium",
					HA:          true,
				},
			}
			Expect(k8sClient.Create(ctx, cache)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cache) }()

			reconciler := &ChoCacheReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cache-ha", Namespace: "default"}, sts)).To(Succeed())
			Expect(*sts.Spec.Replicas).To(Equal(int32(2)))

			// Should have cluster_mode emulated arg
			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Args).To(ContainElements("--cluster_mode", "emulated"))

			// Should also have headless service
			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cache-ha-headless", Namespace: "default"}, svc)).To(Succeed())
			Expect(svc.Spec.ClusterIP).To(Equal("None"))
		})

		It("should use explicit replica count when specified", func() {
			replicas := int32(3)
			cache := &choristerv1alpha1.ChoCache{
				ObjectMeta: metav1.ObjectMeta{Name: "cache-replicas", Namespace: "default"},
				Spec: choristerv1alpha1.ChoCacheSpec{
					Application: "myapp",
					Domain:      "auth",
					Size:        "small",
					HA:          true,
					Replicas:    &replicas,
				},
			}
			Expect(k8sClient.Create(ctx, cache)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cache) }()

			reconciler := &ChoCacheReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cache-replicas", Namespace: "default"}, sts)).To(Succeed())
			Expect(*sts.Spec.Replicas).To(Equal(int32(3)))
		})

		It("should use 1 replica by default (no HA)", func() {
			cache := &choristerv1alpha1.ChoCache{
				ObjectMeta: metav1.ObjectMeta{Name: "cache-single", Namespace: "default"},
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

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cache-single", Namespace: "default"}, deploy)).To(Succeed())
			Expect(*deploy.Spec.Replicas).To(Equal(int32(1)))
		})
	})

	// -----------------------------------------------------------------------
	// Idempotency tests
	// -----------------------------------------------------------------------
	Context("Idempotency", func() {
		It("should produce same result when reconciled twice", func() {
			cache := &choristerv1alpha1.ChoCache{
				ObjectMeta: metav1.ObjectMeta{Name: "cache-idempotent", Namespace: "default"},
				Spec: choristerv1alpha1.ChoCacheSpec{
					Application: "myapp",
					Domain:      "auth",
					Size:        "medium",
				},
			}
			Expect(k8sClient.Create(ctx, cache)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cache) }()

			reconciler := &ChoCacheReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace}}

			// First reconcile
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Get state after first reconcile
			deploy1 := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cache.Name, Namespace: "default"}, deploy1)).To(Succeed())
			rv1 := deploy1.ResourceVersion

			// Second reconcile
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Get state after second reconcile — should be unchanged
			deploy2 := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cache.Name, Namespace: "default"}, deploy2)).To(Succeed())
			Expect(deploy2.ResourceVersion).To(Equal(rv1), "Deployment should not be updated on second reconcile")
		})
	})

	// -----------------------------------------------------------------------
	// Owner reference tests
	// -----------------------------------------------------------------------
	Context("Owner references", func() {
		It("should set owner reference on created resources", func() {
			cache := &choristerv1alpha1.ChoCache{
				ObjectMeta: metav1.ObjectMeta{Name: "cache-owner", Namespace: "default"},
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

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cache-owner", Namespace: "default"}, deploy)).To(Succeed())
			Expect(deploy.OwnerReferences).To(HaveLen(1))
			Expect(deploy.OwnerReferences[0].Name).To(Equal("cache-owner"))

			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cache-owner", Namespace: "default"}, svc)).To(Succeed())
			Expect(svc.OwnerReferences).To(HaveLen(1))
			Expect(svc.OwnerReferences[0].Name).To(Equal("cache-owner"))
		})
	})

	// -----------------------------------------------------------------------
	// Not-found handling
	// -----------------------------------------------------------------------
	Context("Not-found handling", func() {
		It("should return nil when resource is not found", func() {
			reconciler := &ChoCacheReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
