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
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
			// Create a sandbox namespace with the sandbox label
			sandboxNS := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "myapp-payments-sandbox-delete-test",
					Labels: map[string]string{
						"chorister.dev/sandbox": "delete-test",
					},
				},
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: sandboxNS.Name}, sandboxNS)
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, sandboxNS)).To(Succeed())
			}

			// Create a ChoDatabase in the sandbox namespace
			db := &choristerv1alpha1.ChoDatabase{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sandbox-db",
					Namespace: sandboxNS.Name,
				},
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Domain:      "payments",
					Engine:      "postgres",
					Size:        "small",
				},
			}
			Expect(k8sClient.Create(ctx, db)).To(Succeed())

			reconciler := &ChoDatabaseReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// First reconcile: adds finalizer
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: db.Name, Namespace: db.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: db.Name, Namespace: db.Namespace}, db)).To(Succeed())
			Expect(db.Finalizers).To(ContainElement("chorister.dev/database-archive"))

			// Delete the resource
			Expect(k8sClient.Delete(ctx, db)).To(Succeed())

			// Reconcile deletion: sandbox resources should be allowed to delete immediately
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: db.Name, Namespace: db.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify the resource is gone (finalizer removed, deletion proceeds)
			err = k8sClient.Get(ctx, types.NamespacedName{Name: db.Name, Namespace: db.Namespace}, db)
			Expect(errors.IsNotFound(err)).To(BeTrue(), "sandbox ChoDatabase should be fully deleted (no archive)")
		})

		It("should auto-destroy idle sandbox past threshold", func() {
			ctx := context.Background()

			// Create ChoApplication with maxIdleDays=1
			maxIdleDays := 1
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "idle-app", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"test@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion: choristerv1alpha1.PromotionPolicy{
							RequiredApprovers: 1,
							AllowedRoles:      []string{"developer"},
						},
						Sandbox: &choristerv1alpha1.SandboxPolicy{
							MaxIdleDays: &maxIdleDays,
						},
					},
					Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, app) }()

			sandbox := &choristerv1alpha1.ChoSandbox{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sandbox-idle-test",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "idle-app",
					Domain:      "payments",
					Name:        "idle",
					Owner:       "test@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, sandbox)).To(Succeed())

			reconciler := &ChoSandboxReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Reconcile to set up sandbox and initialize lastApplyTime
			for i := 0; i < 3; i++ {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: sandbox.Name, Namespace: sandbox.Namespace},
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Verify sandbox is active
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sandbox.Name, Namespace: sandbox.Namespace}, sandbox)).To(Succeed())
			Expect(sandbox.Status.Phase).To(Equal("Active"))

			// Set lastApplyTime to 3 days ago (well past 1-day threshold)
			threeDaysAgo := metav1.NewTime(time.Now().Add(-3 * 24 * time.Hour))
			sandbox.Status.LastApplyTime = &threeDaysAgo
			Expect(k8sClient.Status().Update(ctx, sandbox)).To(Succeed())

			// Reconcile — should trigger auto-destroy
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sandbox.Name, Namespace: sandbox.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// The sandbox should be marked for deletion
			err = k8sClient.Get(ctx, types.NamespacedName{Name: sandbox.Name, Namespace: sandbox.Namespace}, sandbox)
			if err == nil {
				// If still exists, it should have DeletionTimestamp or Destroying phase
				Expect(sandbox.DeletionTimestamp).NotTo(BeNil(), "expected sandbox to be deleted or in Destroying phase")
			}
			// If NotFound, that's also fine — sandbox was destroyed
		})
	})

	// -----------------------------------------------------------------------
	// Phase 20.2 — FinOps cost estimation
	// -----------------------------------------------------------------------

	Context("Phase 20.2 — FinOps cost estimation", func() {
		It("should estimate sandbox cost from resource declarations", func() {
			ctx := context.Background()

			// Create ChoCluster with cost rates
			cpuRate := resource.MustParse("0.05")
			memRate := resource.MustParse("0.01")
			storageRate := resource.MustParse("0.10")
			cluster := &choristerv1alpha1.ChoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "cost-cluster"},
				Spec: choristerv1alpha1.ChoClusterSpec{
					FinOps: &choristerv1alpha1.FinOpsSpec{
						Rates: &choristerv1alpha1.CostRates{
							CPUPerHour:        &cpuRate,
							MemoryPerGBHour:   &memRate,
							StoragePerGBMonth: &storageRate,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cluster) }()

			// Create sandbox
			sandbox := &choristerv1alpha1.ChoSandbox{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sandbox-cost-test",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "costapp",
					Domain:      "payments",
					Name:        "cost",
					Owner:       "test@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, sandbox)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, sandbox) }()

			reconciler := &ChoSandboxReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			for i := 0; i < 3; i++ {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: sandbox.Name, Namespace: sandbox.Namespace},
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Create a ChoCompute resource in sandbox namespace
			nsName := SandboxNamespace("costapp", "payments", "cost")
			replicas := int32(2)
			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web",
					Namespace: nsName,
				},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "costapp",
					Domain:      "payments",
					Image:       "nginx:latest",
					Replicas:    &replicas,
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, compute)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, compute) }()

			// Re-reconcile to pick up cost
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sandbox.Name, Namespace: sandbox.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sandbox.Name, Namespace: sandbox.Namespace}, sandbox)).To(Succeed())
			Expect(sandbox.Status.EstimatedMonthlyCost).NotTo(Equal("0.00"))
			Expect(sandbox.Status.EstimatedMonthlyCost).NotTo(BeEmpty())
		})
	})

	// -----------------------------------------------------------------------
	// Phase 20.3 — Domain sandbox budget enforcement
	// -----------------------------------------------------------------------

	Context("Phase 20.3 — Domain sandbox budget enforcement", func() {
		It("should set BudgetExceeded when domain budget is exceeded", func() {
			ctx := context.Background()

			// Create ChoCluster with cost rates (high rates so even small resources cost a lot)
			cpuRate := resource.MustParse("100")
			memRate := resource.MustParse("50")
			cluster := &choristerv1alpha1.ChoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "budget-cluster"},
				Spec: choristerv1alpha1.ChoClusterSpec{
					FinOps: &choristerv1alpha1.FinOpsSpec{
						Rates: &choristerv1alpha1.CostRates{
							CPUPerHour:      &cpuRate,
							MemoryPerGBHour: &memRate,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cluster) }()

			// Create ChoApplication with a small budget ($10)
			budget := int64(10)
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "budget-app", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"test@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion: choristerv1alpha1.PromotionPolicy{
							RequiredApprovers: 1,
							AllowedRoles:      []string{"developer"},
						},
						Sandbox: &choristerv1alpha1.SandboxPolicy{
							DefaultBudgetPerDomain: &budget,
						},
					},
					Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, app) }()

			// Create first sandbox (will set up namespace and resources)
			sandbox1 := &choristerv1alpha1.ChoSandbox{
				ObjectMeta: metav1.ObjectMeta{Name: "budget-sandbox-1", Namespace: "default"},
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "budget-app",
					Domain:      "payments",
					Name:        "expensive1",
					Owner:       "test@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, sandbox1)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, sandbox1) }()

			reconciler := &ChoSandboxReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			for i := 0; i < 3; i++ {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: sandbox1.Name, Namespace: sandbox1.Namespace},
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Create expensive ChoCompute in sandbox1 namespace
			nsName := SandboxNamespace("budget-app", "payments", "expensive1")
			replicas := int32(4)
			compute := &choristerv1alpha1.ChoCompute{
				ObjectMeta: metav1.ObjectMeta{Name: "big-compute", Namespace: nsName},
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "budget-app",
					Domain:      "payments",
					Image:       "nginx:latest",
					Replicas:    &replicas,
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, compute)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, compute) }()

			// Re-reconcile sandbox1 to compute its cost
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sandbox1.Name, Namespace: sandbox1.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Now create a second sandbox — should trigger budget exceeded
			sandbox2 := &choristerv1alpha1.ChoSandbox{
				ObjectMeta: metav1.ObjectMeta{Name: "budget-sandbox-2", Namespace: "default"},
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "budget-app",
					Domain:      "payments",
					Name:        "expensive2",
					Owner:       "test@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, sandbox2)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, sandbox2) }()

			for i := 0; i < 3; i++ {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: sandbox2.Name, Namespace: sandbox2.Namespace},
				})
				Expect(err).NotTo(HaveOccurred())
			}

			// Check that the cost of sandbox1 causes budget check for domain to fail
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sandbox1.Name, Namespace: sandbox1.Namespace}, sandbox1)).To(Succeed())
			// The first sandbox has expensive resources, but itself was created before budget check
			// The budget enforcement checks the total across all domain sandboxes
			// Since sandbox1 alone likely exceeds the $10 budget with $100/CPU/hr rates:
			// 4 replicas * 2 CPU * $100/hr * 730h = $584,000 — way over $10

			// Re-reconcile sandbox1 — now that budget is exceeded, it should get BudgetExceeded
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: sandbox1.Name, Namespace: sandbox1.Namespace},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: sandbox1.Name, Namespace: sandbox1.Namespace}, sandbox1)).To(Succeed())
			Expect(sandbox1.Status.Phase).To(Equal("BudgetExceeded"))

			// Check BudgetExceeded condition
			var budgetCondition *metav1.Condition
			for i := range sandbox1.Status.Conditions {
				if sandbox1.Status.Conditions[i].Type == "BudgetExceeded" {
					budgetCondition = &sandbox1.Status.Conditions[i]
					break
				}
			}
			Expect(budgetCondition).NotTo(BeNil(), "expected BudgetExceeded condition")
			Expect(budgetCondition.Status).To(Equal(metav1.ConditionTrue))
		})
	})
})
