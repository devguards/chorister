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
	"k8s.io/apimachinery/pkg/types"
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
})
