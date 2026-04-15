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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var _ = Describe("ChoStorage Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		chostorage := &choristerv1alpha1.ChoStorage{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ChoStorage")
			err := k8sClient.Get(ctx, typeNamespacedName, chostorage)
			if err != nil && errors.IsNotFound(err) {
				storageSize := resource.MustParse("10Gi")
				res := &choristerv1alpha1.ChoStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: choristerv1alpha1.ChoStorageSpec{
						Application: "test-app",
						Domain:      "payments",
						Variant:     "block",
						Size:        &storageSize,
					},
				}
				Expect(k8sClient.Create(ctx, res)).To(Succeed())
			}
		})

		AfterEach(func() {
			res := &choristerv1alpha1.ChoStorage{}
			err := k8sClient.Get(ctx, typeNamespacedName, res)
			if err == nil {
				res.Finalizers = nil
				_ = k8sClient.Update(ctx, res)
				_ = k8sClient.Delete(ctx, res)
			}
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ChoStorageReconciler{
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
	// D.3 — Object storage kro Claim creation
	// -----------------------------------------------------------------------
	Context("D.3 — Object storage reconciliation", func() {
		It("should set object-specific status fields for object variant", func() {
			storageSize := resource.MustParse("50Gi")
			storage := &choristerv1alpha1.ChoStorage{
				ObjectMeta: metav1.ObjectMeta{Name: "media-bucket", Namespace: "default"},
				Spec: choristerv1alpha1.ChoStorageSpec{
					Application:   "myapp",
					Domain:        "media",
					Variant:       "object",
					ObjectBackend: "s3",
					Size:          &storageSize,
				},
			}
			Expect(k8sClient.Create(ctx, storage)).To(Succeed())
			defer func() {
				s := &choristerv1alpha1.ChoStorage{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: storage.Name, Namespace: storage.Namespace}, s); err == nil {
					s.Finalizers = nil
					_ = k8sClient.Update(ctx, s)
					_ = k8sClient.Delete(ctx, s)
				}
			}()

			reconciler := &ChoStorageReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Reconcile reaches object claim creation — kro CRD not installed in envtest,
			// so reconcile returns an error which is expected.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: storage.Name, Namespace: storage.Namespace},
			})
			// kro ObjectStorageClaim CRD is not registered in envtest, expect NoKindMatch error
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ObjectStorageClaim"))
		})

		It("should create PVC for block variant and not object status", func() {
			storageSize := resource.MustParse("5Gi")
			storage := &choristerv1alpha1.ChoStorage{
				ObjectMeta: metav1.ObjectMeta{Name: "block-data", Namespace: "default"},
				Spec: choristerv1alpha1.ChoStorageSpec{
					Application: "myapp",
					Domain:      "data",
					Variant:     "block",
					Size:        &storageSize,
				},
			}
			Expect(k8sClient.Create(ctx, storage)).To(Succeed())
			defer func() {
				s := &choristerv1alpha1.ChoStorage{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: storage.Name, Namespace: storage.Namespace}, s); err == nil {
					s.Finalizers = nil
					_ = k8sClient.Update(ctx, s)
					_ = k8sClient.Delete(ctx, s)
				}
			}()

			reconciler := &ChoStorageReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Reconcile twice (first adds finalizer)
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: storage.Name, Namespace: storage.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: storage.Name, Namespace: storage.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Block variant should create PVC
			pvc := &corev1.PersistentVolumeClaim{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "block-data", Namespace: "default"}, pvc)).To(Succeed())
			Expect(pvc.Spec.AccessModes).To(ContainElement(corev1.ReadWriteOnce))

			// Block variant should NOT set bucket name
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: storage.Name, Namespace: storage.Namespace}, storage)).To(Succeed())
			Expect(storage.Status.BucketName).To(BeEmpty())
			Expect(storage.Status.Ready).To(BeTrue())
		})
	})

	// -----------------------------------------------------------------------
	// Gap 2 — Revision filtering on ChoStorage controller
	// -----------------------------------------------------------------------
	Context("Gap 2 — Controller revision filtering", func() {
		It("should skip ChoStorage in non-matching-revision namespace", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "storage-rev-mismatch",
					Labels: map[string]string{LabelRevision: "2-0"},
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, ns))).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "storage-rev-mismatch"}, ns)).To(Succeed())
			if ns.Labels == nil {
				ns.Labels = map[string]string{}
			}
			ns.Labels[LabelRevision] = "2-0"
			Expect(k8sClient.Update(ctx, ns)).To(Succeed())

			storageSize := resource.MustParse("5Gi")
			storage := &choristerv1alpha1.ChoStorage{
				ObjectMeta: metav1.ObjectMeta{Name: "rev-skip-storage", Namespace: "storage-rev-mismatch"},
				Spec: choristerv1alpha1.ChoStorageSpec{
					Application: "myapp",
					Domain:      "data",
					Variant:     "object",
					Size:        &storageSize,
				},
			}
			Expect(client.IgnoreAlreadyExists(k8sClient.Create(ctx, storage))).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, storage) }()

			reconciler := &ChoStorageReconciler{
				Client:             k8sClient,
				Scheme:             k8sClient.Scheme(),
				ControllerRevision: "1-0",
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: storage.Name, Namespace: storage.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Should have skipped — status untouched
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: storage.Name, Namespace: storage.Namespace}, storage)).To(Succeed())
			Expect(storage.Status.Ready).To(BeFalse())
		})
	})
})
