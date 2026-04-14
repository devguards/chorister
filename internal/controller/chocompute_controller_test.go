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
					// TODO(user): Specify other spec details if needed.
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
			Skip("awaiting Phase 3.1: ChoCompute reconciler → Deployment + Service")

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

			// Assert Service created
			svc := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "api-server", Namespace: "default"}, svc)).To(Succeed())
		})

		It("should reflect ready replicas in status", func() {
			Skip("awaiting Phase 3.1: ChoCompute reconciler → Deployment + Service")

			// Create ChoCompute → reconcile → assert status.ready tracks Deployment readiness
		})

		It("should create Job for job variant", func() {
			Skip("awaiting Phase 3.3: Compute variants — Job and CronJob")

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
		})

		It("should create CronJob for cronjob variant", func() {
			Skip("awaiting Phase 3.3: Compute variants — Job and CronJob")

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
			Skip("awaiting Phase 3.2: HPA and PDB for compute")

			replicas := int32(2)
			targetCPU := int32(80)
			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{Name: "api-hpa", Namespace: "default"},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "payments",
					Image:       "nginx:latest",
					Replicas:    &replicas,
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
			Expect(hpaList.Items).NotTo(BeEmpty())
		})

		It("should create PDB when replicas > 1", func() {
			Skip("awaiting Phase 3.2: HPA and PDB for compute")

			replicas := int32(3)
			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{Name: "api-pdb", Namespace: "default"},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "payments",
					Image:       "nginx:latest",
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

			pdbList := &policyv1.PodDisruptionBudgetList{}
			Expect(k8sClient.List(ctx, pdbList, client.InNamespace("default"))).To(Succeed())
			Expect(pdbList.Items).NotTo(BeEmpty())
		})

		It("should update Deployment when image changes", func() {
			Skip("awaiting Phase 3.1: ChoCompute reconciler → Deployment + Service")

			// Create ChoCompute → reconcile → change image → reconcile → Deployment updated
		})

		It("should clean up Deployment and Service on deletion", func() {
			Skip("awaiting Phase 3.1: ChoCompute reconciler → Deployment + Service")

			// Delete CRD → Deployment + Service cleaned up via owner refs
		})
	})
})
