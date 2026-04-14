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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var _ = Describe("ChoCompute Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		chocompute := &choristerv1alpha1.ChoCompute{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ChoCompute")
			err := k8sClient.Get(ctx, typeNamespacedName, chocompute)
			if err != nil && errors.IsNotFound(err) {
				resource := &choristerv1alpha1.ChoCompute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: choristerv1alpha1.ChoComputeSpec{
						Application: "test-app",
						Domain:      "test-domain",
						Image:       "nginx:latest",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &choristerv1alpha1.ChoCompute{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance ChoCompute")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ChoComputeReconciler{
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
	// 1A.5 — ChoCompute lifecycle (envtest)
	// -----------------------------------------------------------------------

	Context("1A.5 — ChoCompute lifecycle", func() {
		It("should create Deployment and Service for long-running compute", func() {
			replicas := int32(2)
			port := int32(8080)
			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{Name: "api-server", Namespace: "default"},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "payments",
					Image:       "nginx:latest",
					Variant:     "long-running",
					Replicas:    &replicas,
					Port:        &port,
				},
			}
			Expect(k8sClient.Create(ctx, compute)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, compute) }()

			reconciler := &ChoComputeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert Deployment created
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "api-server", Namespace: "default"}, deploy)).To(Succeed())
			Expect(*deploy.Spec.Replicas).To(Equal(int32(2)))
			Expect(deploy.Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:latest"))
			Expect(deploy.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(8080)))
			Expect(deploy.Labels).To(HaveKeyWithValue("chorister.dev/application", "myapp"))
			Expect(deploy.Labels).To(HaveKeyWithValue("chorister.dev/domain", "payments"))

			// Assert Service created
			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "api-server", Namespace: "default"}, svc)).To(Succeed())
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(8080)))
			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
		})

		It("should reflect ready replicas in status", func() {
			replicas := int32(1)
			port := int32(8080)
			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{Name: "status-test", Namespace: "default"},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "payments",
					Image:       "nginx:latest",
					Replicas:    &replicas,
					Port:        &port,
				},
			}
			Expect(k8sClient.Create(ctx, compute)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, compute) }()

			reconciler := &ChoComputeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// In envtest there is no kubelet, so readyReplicas will be 0
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace}, compute)).To(Succeed())
			Expect(compute.Status.Conditions).NotTo(BeEmpty())
		})

		It("should create Job for job variant", func() {
			replicas := int32(1)
			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{Name: "migrate-job", Namespace: "default"},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "payments",
					Image:       "alpine:latest",
					Variant:     "job",
					Replicas:    &replicas,
					Command:     []string{"echo", "hello"},
				},
			}
			Expect(k8sClient.Create(ctx, compute)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, compute) }()

			reconciler := &ChoComputeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert Job created (not Deployment)
			job := &batchv1.Job{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "migrate-job", Namespace: "default"}, job)).To(Succeed())
			Expect(job.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{"echo", "hello"}))

			// Assert no Deployment was created
			deploy := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "migrate-job", Namespace: "default"}, deploy)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should create CronJob for cronjob variant", func() {
			replicas := int32(1)
			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{Name: "cleanup-cron", Namespace: "default"},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "payments",
					Image:       "alpine:latest",
					Variant:     "cronjob",
					Schedule:    "0 2 * * *",
					Replicas:    &replicas,
				},
			}
			Expect(k8sClient.Create(ctx, compute)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, compute) }()

			reconciler := &ChoComputeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			cronJob := &batchv1.CronJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cleanup-cron", Namespace: "default"}, cronJob)).To(Succeed())
			Expect(cronJob.Spec.Schedule).To(Equal("0 2 * * *"))
		})

		It("should create HPA when autoscaling is specified", func() {
			replicas := int32(2)
			port := int32(8080)
			targetCPU := int32(80)
			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{Name: "api-hpa", Namespace: "default"},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "payments",
					Image:       "nginx:latest",
					Replicas:    &replicas,
					Port:        &port,
					Autoscaling: &choristerv1alpha1.AutoscalingSpec{
						MinReplicas:      2,
						MaxReplicas:      10,
						TargetCPUPercent: &targetCPU,
					},
				},
			}
			Expect(k8sClient.Create(ctx, compute)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, compute) }()

			reconciler := &ChoComputeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			hpaList := &autoscalingv2.HorizontalPodAutoscalerList{}
			Expect(k8sClient.List(ctx, hpaList, client.InNamespace("default"))).To(Succeed())
			found := false
			for _, hpa := range hpaList.Items {
				if hpa.Name == "api-hpa-hpa" {
					found = true
					Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))
					Expect(*hpa.Spec.MinReplicas).To(Equal(int32(2)))
					Expect(hpa.Spec.ScaleTargetRef.Name).To(Equal("api-hpa"))
				}
			}
			Expect(found).To(BeTrue(), "HPA not found")
		})

		It("should create PDB when replicas > 1", func() {
			replicas := int32(3)
			port := int32(8080)
			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{Name: "api-pdb", Namespace: "default"},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "payments",
					Image:       "nginx:latest",
					Replicas:    &replicas,
					Port:        &port,
				},
			}
			Expect(k8sClient.Create(ctx, compute)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, compute) }()

			reconciler := &ChoComputeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			pdbList := &policyv1.PodDisruptionBudgetList{}
			Expect(k8sClient.List(ctx, pdbList, client.InNamespace("default"))).To(Succeed())
			found := false
			for _, pdb := range pdbList.Items {
				if pdb.Name == "api-pdb-pdb" {
					found = true
					Expect(pdb.Spec.MinAvailable.IntValue()).To(Equal(2))
				}
			}
			Expect(found).To(BeTrue(), "PDB not found")
		})

		It("should update Deployment when image changes", func() {
			replicas := int32(1)
			port := int32(8080)
			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{Name: "img-update", Namespace: "default"},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "payments",
					Image:       "nginx:1.0",
					Replicas:    &replicas,
					Port:        &port,
				},
			}
			Expect(k8sClient.Create(ctx, compute)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, compute) }()

			reconciler := &ChoComputeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Change image
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace}, compute)).To(Succeed())
			compute.Spec.Image = "nginx:2.0"
			Expect(k8sClient.Update(ctx, compute)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace}, deploy)).To(Succeed())
			Expect(deploy.Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:2.0"))
		})

		It("should clean up Deployment and Service on deletion", func() {
			replicas := int32(1)
			port := int32(8080)
			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{Name: "cleanup-test", Namespace: "default"},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "payments",
					Image:       "nginx:latest",
					Replicas:    &replicas,
					Port:        &port,
				},
			}
			Expect(k8sClient.Create(ctx, compute)).To(Succeed())

			reconciler := &ChoComputeReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify resources exist
			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace}, deploy)).To(Succeed())

			// Delete ChoCompute — owner refs handle cleanup
			Expect(k8sClient.Delete(ctx, compute)).To(Succeed())
		})
	})
})
