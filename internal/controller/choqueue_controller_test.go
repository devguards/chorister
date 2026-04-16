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
	"time"

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

var _ = Describe("ChoQueue Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		choqueue := &choristerv1alpha1.ChoQueue{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ChoQueue")
			err := k8sClient.Get(ctx, typeNamespacedName, choqueue)
			if err != nil && errors.IsNotFound(err) {
				resource := &choristerv1alpha1.ChoQueue{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: choristerv1alpha1.ChoQueueSpec{
						Application: "test-app",
						Domain:      "payments",
						Type:        "nats",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &choristerv1alpha1.ChoQueue{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ChoQueue")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ChoQueueReconciler{
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
	// 1A.7 — ChoQueue lifecycle (envtest)
	// -----------------------------------------------------------------------

	Context("1A.7 — ChoQueue lifecycle", func() {
		It("should create credential Secret for NATS connection", func() {
			queue := &choristerv1alpha1.ChoQueue{
				ObjectMeta: metav1.ObjectMeta{Name: "events", Namespace: "default"},
				Spec: choristerv1alpha1.ChoQueueSpec{
					Application: "myapp",
					Domain:      "payments",
					Type:        "nats",
					Size:        "small",
				},
			}
			Expect(k8sClient.Create(ctx, queue)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, queue) }()

			reconciler := &ChoQueueReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert NATS connection Secret exists
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "payments--queue--events-credentials", Namespace: "default",
			}, secret)).To(Succeed())
			Expect(secret.Data).To(HaveKey("host"))
			Expect(secret.Data).To(HaveKey("port"))
			Expect(secret.Data).To(HaveKey("uri"))
		})

		It("should create StatefulSet for NATS", func() {
			queue := &choristerv1alpha1.ChoQueue{
				ObjectMeta: metav1.ObjectMeta{Name: "events-sts", Namespace: "default"},
				Spec: choristerv1alpha1.ChoQueueSpec{
					Application: "myapp",
					Domain:      "payments",
					Type:        "nats",
					Size:        "medium",
				},
			}
			Expect(k8sClient.Create(ctx, queue)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, queue) }()

			reconciler := &ChoQueueReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert StatefulSet created
			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "events-sts", Namespace: "default",
			}, sts)).To(Succeed())
			Expect(sts.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("nats:2-alpine"))

			// Assert PVC-based storage (VolumeClaimTemplates, not EmptyDir)
			Expect(sts.Spec.VolumeClaimTemplates).To(HaveLen(1))
			Expect(sts.Spec.VolumeClaimTemplates[0].Name).To(Equal("data"))
			Expect(sts.Spec.VolumeClaimTemplates[0].Spec.AccessModes).To(ContainElement(corev1.ReadWriteOnce))
			storageReq := sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
			Expect(storageReq.String()).To(Equal("1Gi"))

			// Assert headless Service created
			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "events-sts-headless", Namespace: "default",
			}, svc)).To(Succeed())
			Expect(svc.Spec.ClusterIP).To(Equal("None"))

			// Assert client Service created
			clientSvc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "events-sts", Namespace: "default",
			}, clientSvc)).To(Succeed())
		})

		It("should use custom storageSize for PVC", func() {
			queue := &choristerv1alpha1.ChoQueue{
				ObjectMeta: metav1.ObjectMeta{Name: "events-pvc-size", Namespace: "default"},
				Spec: choristerv1alpha1.ChoQueueSpec{
					Application: "myapp",
					Domain:      "payments",
					Type:        "nats",
					StorageSize: "10Gi",
				},
			}
			Expect(k8sClient.Create(ctx, queue)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, queue) }()

			reconciler := &ChoQueueReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: queue.Name, Namespace: queue.Namespace,
			}, sts)).To(Succeed())
			Expect(sts.Spec.VolumeClaimTemplates).To(HaveLen(1))
			storageReq := sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
			Expect(storageReq.String()).To(Equal("10Gi"))
		})

		It("should create ConfigMap when JetStream config is specified", func() {
			queue := &choristerv1alpha1.ChoQueue{
				ObjectMeta: metav1.ObjectMeta{Name: "events-js-config", Namespace: "default"},
				Spec: choristerv1alpha1.ChoQueueSpec{
					Application: "myapp",
					Domain:      "payments",
					Type:        "nats",
					JetStream: &choristerv1alpha1.JetStreamConfig{
						MaxBytes:  "512Mi",
						Retention: "interest",
					},
				},
			}
			Expect(k8sClient.Create(ctx, queue)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, queue) }()

			reconciler := &ChoQueueReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert ConfigMap created for JetStream config
			cm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "events-js-config-js-config", Namespace: "default",
			}, cm)).To(Succeed())
			Expect(cm.Data).To(HaveKey("jetstream.conf"))
			Expect(cm.Data["jetstream.conf"]).To(ContainSubstring("max_mem_store: 512Mi"))

			// Assert StatefulSet has config volume mount
			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: queue.Name, Namespace: queue.Namespace,
			}, sts)).To(Succeed())
			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Args).To(ContainElement("-c"))
			Expect(container.Args).To(ContainElement("/etc/nats/jetstream.conf"))
			Expect(container.VolumeMounts).To(HaveLen(2)) // data + js-config
		})

		It("should update status after reconciliation", func() {
			queue := &choristerv1alpha1.ChoQueue{
				ObjectMeta: metav1.ObjectMeta{Name: "events-status", Namespace: "default"},
				Spec: choristerv1alpha1.ChoQueueSpec{
					Application: "myapp",
					Domain:      "payments",
					Type:        "nats",
					Size:        "small",
				},
			}
			Expect(k8sClient.Create(ctx, queue)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, queue) }()

			reconciler := &ChoQueueReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Re-fetch and check status
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace}, queue)).To(Succeed())
			Expect(queue.Status.Ready).To(BeTrue())
			Expect(queue.Status.Lifecycle).To(Equal("Active"))
			Expect(queue.Status.CredentialsSecretRef).To(Equal("payments--queue--events-status-credentials"))
		})
	})

	// -----------------------------------------------------------------------
	// Gap 3 — 30-day retention lower-bound negative test
	// -----------------------------------------------------------------------
	Context("Gap 3 — Archive retention lower bound", func() {
		It("should NOT transition to Deletable before retention period expires", func() {
			queue := &choristerv1alpha1.ChoQueue{
				ObjectMeta: metav1.ObjectMeta{Name: "retention-test-queue", Namespace: "default"},
				Spec: choristerv1alpha1.ChoQueueSpec{
					Application: "myapp",
					Domain:      "payments",
					Type:        "nats",
					Size:        "small",
				},
			}
			Expect(k8sClient.Create(ctx, queue)).To(Succeed())
			defer func() {
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace}, queue)
				queue.Finalizers = nil
				_ = k8sClient.Update(ctx, queue)
				_ = k8sClient.Delete(ctx, queue)
			}()

			reconciler := &ChoQueueReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// First reconcile: adds finalizer
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Re-fetch to get latest resourceVersion after reconcile
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace}, queue)).To(Succeed())

			// Set lifecycle=Archived with deletableAfter 29 days in the future
			now := metav1.Now()
			deletableAfter := metav1.NewTime(now.Add(29 * 24 * time.Hour))
			queue.Status.Lifecycle = "Archived"
			queue.Status.ArchivedAt = &now
			queue.Status.DeletableAfter = &deletableAfter
			Expect(k8sClient.Status().Update(ctx, queue)).To(Succeed())

			// Reconcile: should NOT transition to Deletable (retention not expired)
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify lifecycle is still Archived
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace}, queue)).To(Succeed())
			Expect(queue.Status.Lifecycle).To(Equal("Archived"))
			Expect(queue.Status.Ready).To(BeFalse())
		})
	})

	// -----------------------------------------------------------------------
	// Gap 2 — Revision filtering on ChoQueue controller
	// -----------------------------------------------------------------------
	Context("Gap 2 — Controller revision filtering", func() {
		It("should skip ChoQueue in non-matching-revision namespace", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "queue-rev-mismatch",
					Labels: map[string]string{LabelRevision: "2-0"},
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ns))).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "queue-rev-mismatch"}, ns)).To(Succeed())
			if ns.Labels == nil {
				ns.Labels = map[string]string{}
			}
			ns.Labels[LabelRevision] = "2-0"
			Expect(k8sClient.Update(ctx, ns)).To(Succeed())

			queue := &choristerv1alpha1.ChoQueue{
				ObjectMeta: metav1.ObjectMeta{Name: "rev-skip-queue", Namespace: "queue-rev-mismatch"},
				Spec: choristerv1alpha1.ChoQueueSpec{
					Application: "myapp",
					Domain:      "payments",
					Type:        "nats",
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, queue))).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, queue) }()

			reconciler := &ChoQueueReconciler{
				Client:             k8sClient,
				Scheme:             k8sClient.Scheme(),
				ControllerRevision: "1-0",
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Should have skipped — no StatefulSet created
			sts := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace}, sts)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "expected no StatefulSet when revision mismatches")
		})
	})

	// -----------------------------------------------------------------------
	// Gap 16 — Streaming queue variant routing
	// -----------------------------------------------------------------------
	Context("Gap 16 — Streaming queue variant", func() {
		It("should not create NATS StatefulSet for streaming variant", func() {
			queue := &choristerv1alpha1.ChoQueue{
				ObjectMeta: metav1.ObjectMeta{Name: "stream-events", Namespace: "default"},
				Spec: choristerv1alpha1.ChoQueueSpec{
					Application: "myapp",
					Domain:      "analytics",
					Type:        "streaming",
				},
			}
			Expect(k8sClient.Create(ctx, queue)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, queue) }()

			reconciler := &ChoQueueReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace},
			})
			// Expect an error because Strimzi/AutoMQ CRDs are not installed
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("streaming queue variant not supported"))

			// Assert no NATS StatefulSet was created
			sts := &appsv1.StatefulSet{}
			stsErr := k8sClient.Get(ctx, types.NamespacedName{
				Name: "stream-events", Namespace: "default",
			}, sts)
			Expect(errors.IsNotFound(stsErr)).To(BeTrue(),
				"streaming variant should not create a NATS StatefulSet")

			// Assert no NATS headless Service was created
			svc := &corev1.Service{}
			svcErr := k8sClient.Get(ctx, types.NamespacedName{
				Name: "stream-events-headless", Namespace: "default",
			}, svc)
			Expect(errors.IsNotFound(svcErr)).To(BeTrue(),
				"streaming variant should not create a NATS headless Service")

			// Verify condition is set explaining the missing operator
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace}, queue)).To(Succeed())
			var found bool
			for _, c := range queue.Status.Conditions {
				if c.Reason == "StreamingOperatorNotInstalled" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected StreamingOperatorNotInstalled condition")
		})

		It("should still create NATS StatefulSet for nats variant (regression guard)", func() {
			queue := &choristerv1alpha1.ChoQueue{
				ObjectMeta: metav1.ObjectMeta{Name: "nats-regression-queue", Namespace: "default"},
				Spec: choristerv1alpha1.ChoQueueSpec{
					Application: "myapp",
					Domain:      "payments",
					Type:        "nats",
					Size:        "small",
				},
			}
			Expect(k8sClient.Create(ctx, queue)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, queue) }()

			reconciler := &ChoQueueReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			sts := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: queue.Name, Namespace: queue.Namespace,
			}, sts)).To(Succeed())
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("nats:2-alpine"))
		})
	})
})
