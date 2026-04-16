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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var _ = Describe("ChoDatabase Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		chodatabase := &choristerv1alpha1.ChoDatabase{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ChoDatabase")
			err := k8sClient.Get(ctx, typeNamespacedName, chodatabase)
			if err != nil && errors.IsNotFound(err) {
				resource := &choristerv1alpha1.ChoDatabase{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: choristerv1alpha1.ChoDatabaseSpec{
						Application: "test-app",
						Domain:      "payments",
						Engine:      "postgres",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &choristerv1alpha1.ChoDatabase{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ChoDatabase")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ChoDatabaseReconciler{
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
	// 1A.6 — ChoDatabase lifecycle (envtest, skip SGCluster assertions)
	// -----------------------------------------------------------------------

	Context("1A.6 — ChoDatabase lifecycle", func() {
		It("should create credential Secret with expected keys", func() {
			db := &choristerv1alpha1.ChoDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "main-db", Namespace: "default"},
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Domain:      "payments",
					Engine:      "postgres",
					Size:        "small",
				},
			}
			Expect(k8sClient.Create(ctx, db)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, db) }()

			reconciler := &ChoDatabaseReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: db.Name, Namespace: db.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert credential Secret with host/port/username/password/uri keys
			secretName := "payments--database--main-db-credentials"
			secret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: "default"}, secret)).To(Succeed())
			for _, key := range []string{"host", "port", "username", "password", "uri"} {
				Expect(secret.Data).To(HaveKey(key))
			}
		})

		It("should use 2+ instances for ha=true", func() {
			db := &choristerv1alpha1.ChoDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "ha-db", Namespace: "default"},
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Domain:      "payments",
					Engine:      "postgres",
					Size:        "medium",
					HA:          true,
				},
			}
			Expect(k8sClient.Create(ctx, db)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, db) }()

			reconciler := &ChoDatabaseReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: db.Name, Namespace: db.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert compiled output has >= 2 instances
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: db.Name, Namespace: db.Namespace}, db)).To(Succeed())
			Expect(db.Status.Instances).To(BeNumerically(">=", int32(2)))
		})

		It("should use 1 instance for ha=false", func() {
			db := &choristerv1alpha1.ChoDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "single-db", Namespace: "default"},
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Domain:      "payments",
					Engine:      "postgres",
					Size:        "small",
					HA:          false,
				},
			}
			Expect(k8sClient.Create(ctx, db)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, db) }()

			reconciler := &ChoDatabaseReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: db.Name, Namespace: db.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: db.Name, Namespace: db.Namespace}, db)).To(Succeed())
			Expect(db.Status.Instances).To(Equal(int32(1)))
		})

		It("should archive on deletion instead of removing (production)", func() {
			// Create a ChoDatabase in the default namespace (non-sandbox)
			db := &choristerv1alpha1.ChoDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "archive-test-db", Namespace: "default"},
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Domain:      "payments",
					Engine:      "postgres",
					Size:        "small",
				},
			}
			Expect(k8sClient.Create(ctx, db)).To(Succeed())
			defer func() {
				// Clean up: remove finalizer if present, then delete
				_ = k8sClient.Get(ctx, types.NamespacedName{Name: db.Name, Namespace: db.Namespace}, db)
				db.Finalizers = nil
				_ = k8sClient.Update(ctx, db)
				_ = k8sClient.Delete(ctx, db)
			}()

			reconciler := &ChoDatabaseReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// First reconcile: adds finalizer
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: db.Name, Namespace: db.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: db.Name, Namespace: db.Namespace}, db)).To(Succeed())
			Expect(db.Finalizers).To(ContainElement("chorister.dev/database-archive"))

			// Simulate promotion-driven archival: set lifecycle=Archived
			now := metav1.Now()
			deletableAfter := metav1.NewTime(now.Add(-1 * time.Hour)) // already past retention
			db.Status.Lifecycle = "Archived"
			db.Status.ArchivedAt = &now
			db.Status.DeletableAfter = &deletableAfter
			Expect(k8sClient.Status().Update(ctx, db)).To(Succeed())

			// Reconcile: should transition from Archived to Deletable (past retention)
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: db.Name, Namespace: db.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify lifecycle transitioned to Deletable
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: db.Name, Namespace: db.Namespace}, db)).To(Succeed())
			Expect(db.Status.Lifecycle).To(Equal("Deletable"))
			Expect(db.Status.Ready).To(BeFalse())
		})
	})

	// -----------------------------------------------------------------------
	// Gap 2 — Revision filtering on ChoDatabase controller
	// -----------------------------------------------------------------------
	Context("Gap 2 — Controller revision filtering", func() {
		It("should reconcile ChoDatabase in matching-revision namespace", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "db-rev-match",
					Labels: map[string]string{LabelRevision: "1-0"},
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ns))).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "db-rev-match"}, ns)).To(Succeed())
			if ns.Labels == nil {
				ns.Labels = map[string]string{}
			}
			ns.Labels[LabelRevision] = "1-0"
			Expect(k8sClient.Update(ctx, ns)).To(Succeed())

			db := &choristerv1alpha1.ChoDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "rev-match-db", Namespace: "db-rev-match"},
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Domain:      "payments",
					Engine:      "postgres",
					Size:        "small",
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, db))).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, db) }()

			reconciler := &ChoDatabaseReconciler{
				Client:             k8sClient,
				Scheme:             k8sClient.Scheme(),
				ControllerRevision: "1-0",
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: db.Name, Namespace: db.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Should have reconciled (credential Secret created)
			secret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "payments--database--rev-match-db-credentials",
				Namespace: "db-rev-match",
			}, secret)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should skip ChoDatabase in non-matching-revision namespace", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "db-rev-mismatch",
					Labels: map[string]string{LabelRevision: "2-0"},
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ns))).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "db-rev-mismatch"}, ns)).To(Succeed())
			if ns.Labels == nil {
				ns.Labels = map[string]string{}
			}
			ns.Labels[LabelRevision] = "2-0"
			Expect(k8sClient.Update(ctx, ns)).To(Succeed())

			db := &choristerv1alpha1.ChoDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "rev-skip-db", Namespace: "db-rev-mismatch"},
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Domain:      "payments",
					Engine:      "postgres",
					Size:        "small",
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, db))).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, db) }()

			reconciler := &ChoDatabaseReconciler{
				Client:             k8sClient,
				Scheme:             k8sClient.Scheme(),
				ControllerRevision: "1-0",
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: db.Name, Namespace: db.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Should have skipped — no credential Secret created
			secret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "payments--database--rev-skip-db-credentials",
				Namespace: "db-rev-mismatch",
			}, secret)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "expected no Secret when revision mismatches")
		})
	})

	// -----------------------------------------------------------------------
	// D.1 — StackGres SGCluster construction
	// -----------------------------------------------------------------------
	Context("D.1 — SGCluster construction", func() {
		It("should build SGCluster with correct HA settings", func() {
			db := &choristerv1alpha1.ChoDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "main", Namespace: "myapp-payments"},
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Domain:      "payments",
					Engine:      "postgres",
					Size:        "medium",
					HA:          true,
				},
			}
			sgCluster := buildSGCluster(db, "payments-main", "payments-main", 2)

			Expect(sgCluster.GetKind()).To(Equal("SGCluster"))
			Expect(sgCluster.GetName()).To(Equal("payments-main"))
			Expect(sgCluster.GetNamespace()).To(Equal("myapp-payments"))

			spec, ok := sgCluster.Object["spec"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(spec["instances"]).To(Equal(int64(2)))

			postgres := spec["postgres"].(map[string]any)
			Expect(postgres["version"]).To(Equal("16"))

			pods := spec["pods"].(map[string]any)
			pv := pods["persistentVolume"].(map[string]any)
			Expect(pv["size"]).To(Equal("50Gi"))

			nonProd := spec["nonProductionOptions"].(map[string]any)
			Expect(nonProd["disableClusterPodAntiAffinity"]).To(BeFalse())
		})

		It("should build SGCluster with single instance for non-HA", func() {
			db := &choristerv1alpha1.ChoDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "myapp-dev"},
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Domain:      "dev",
					Engine:      "postgres",
					Size:        "small",
					HA:          false,
				},
			}
			sgCluster := buildSGCluster(db, "dev-test", "dev-test", 1)

			spec := sgCluster.Object["spec"].(map[string]any)
			Expect(spec["instances"]).To(Equal(int64(1)))

			pods := spec["pods"].(map[string]any)
			pv := pods["persistentVolume"].(map[string]any)
			Expect(pv["size"]).To(Equal("10Gi"))

			nonProd := spec["nonProductionOptions"].(map[string]any)
			Expect(nonProd["disableClusterPodAntiAffinity"]).To(BeTrue())
		})

		It("should include backup configuration", func() {
			db := &choristerv1alpha1.ChoDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "main", Namespace: "ns"},
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "app",
					Domain:      "data",
					Engine:      "postgres",
					Size:        "large",
				},
			}
			sgCluster := buildSGCluster(db, "data-main", "data-main", 1)

			spec := sgCluster.Object["spec"].(map[string]any)
			configs := spec["configurations"].(map[string]any)
			backups := configs["backups"].([]any)
			Expect(backups).To(HaveLen(1))

			backup := backups[0].(map[string]any)
			Expect(backup["cronSchedule"]).To(Equal("0 3 * * *"))
			Expect(backup["retention"]).To(Equal(int64(7)))
		})

		It("should map sizing tiers to resources", func() {
			Expect(dbVolumeSize("small")).To(Equal("10Gi"))
			Expect(dbVolumeSize("medium")).To(Equal("50Gi"))
			Expect(dbVolumeSize("large")).To(Equal("200Gi"))
			Expect(dbVolumeSize("unknown")).To(Equal("10Gi"))

			cpu, mem := dbProfileResources("small")
			Expect(cpu).To(Equal("1"))
			Expect(mem).To(Equal("2Gi"))

			cpu, mem = dbProfileResources("large")
			Expect(cpu).To(Equal("4"))
			Expect(mem).To(Equal("8Gi"))
		})
	})

	// -----------------------------------------------------------------------
	// Idempotency tests
	// -----------------------------------------------------------------------
	Context("Idempotency", func() {
		It("should not update StackGres cluster on second reconcile", func() {
			db := &choristerv1alpha1.ChoDatabase{
				ObjectMeta: metav1.ObjectMeta{Name: "db-idemp", Namespace: "default"},
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Domain:      "data",
					Engine:      "postgres",
					Size:        "small",
				},
			}
			Expect(k8sClient.Create(ctx, db)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, db) }()

			reconciler := &ChoDatabaseReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: db.Name, Namespace: db.Namespace}}

			// First reconcile
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Get SGCluster after first reconcile
			sgCluster := &unstructured.Unstructured{}
			sgCluster.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "stackgres.io",
				Version: "v1",
				Kind:    "SGCluster",
			})
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "db-idemp", Namespace: "default"}, sgCluster)
			if err == nil {
				rv1 := sgCluster.GetResourceVersion()

				// Second reconcile
				_, err = reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())

				err = k8sClient.Get(ctx, types.NamespacedName{Name: "db-idemp", Namespace: "default"}, sgCluster)
				Expect(err).NotTo(HaveOccurred())
				Expect(sgCluster.GetResourceVersion()).To(Equal(rv1))
			}
		})
	})

	// -----------------------------------------------------------------------
	// Not-found handling
	// -----------------------------------------------------------------------
	Context("Not-found handling", func() {
		It("should return nil when ChoDatabase does not exist", func() {
			reconciler := &ChoDatabaseReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent-db", Namespace: "default"},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
