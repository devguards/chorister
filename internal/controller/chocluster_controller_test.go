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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/audit"
)

var _ = Describe("ChoCluster Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name: resourceName,
		}
		chocluster := &choristerv1alpha1.ChoCluster{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind ChoCluster")
			err := k8sClient.Get(ctx, typeNamespacedName, chocluster)
			if err != nil && errors.IsNotFound(err) {
				resource := &choristerv1alpha1.ChoCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &choristerv1alpha1.ChoCluster{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance ChoCluster")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &ChoClusterReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				AuditLogger: audit.NewNoopLogger(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, typeNamespacedName, chocluster)).To(Succeed())
			Expect(chocluster.Status.Phase).To(Equal("Ready"))
		})
	})

	// -----------------------------------------------------------------------
	// Phase 11 — Observability stack
	// -----------------------------------------------------------------------

	Context("Phase 11 — Observability stack", func() {
		It("should create monitoring namespace and LGTM deployments (11.1)", func() {
			cluster := &choristerv1alpha1.ChoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "obs-cluster"},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cluster) }()

			reconciler := &ChoClusterReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				AuditLogger: audit.NewNoopLogger(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cluster.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert monitoring namespace exists
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cho-monitoring"}, ns)).To(Succeed())

			// Assert LGTM Deployments exist
			for _, name := range []string{"loki", "mimir", "tempo", "alloy", "grafana"} {
				deploy := &appsv1.Deployment{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: name, Namespace: "cho-monitoring",
				}, deploy)).To(Succeed(), "expected Deployment %q", name)
			}

			// Assert Services exist
			for _, name := range []string{"loki", "mimir", "tempo", "alloy", "grafana"} {
				svc := &corev1.Service{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: name, Namespace: "cho-monitoring",
				}, svc)).To(Succeed(), "expected Service %q", name)
			}

			// Assert status
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cluster.Name}, cluster)).To(Succeed())
			Expect(cluster.Status.ObservabilityReady).To(BeTrue())
		})

		It("should block reconciliation on audit write failure (11.2)", func() {
			cluster := &choristerv1alpha1.ChoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "audit-fail-cluster"},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cluster) }()

			reconciler := &ChoClusterReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				AuditLogger: audit.NewFailingLogger(fmt.Errorf("loki unavailable")),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cluster.Name},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("audit write failed"))

			// Check condition was set
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cluster.Name}, cluster)).To(Succeed())
			var auditCondition *metav1.Condition
			for i := range cluster.Status.Conditions {
				if cluster.Status.Conditions[i].Type == "AuditReady" {
					auditCondition = &cluster.Status.Conditions[i]
					break
				}
			}
			Expect(auditCondition).NotTo(BeNil())
			Expect(auditCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(auditCondition.Reason).To(Equal("AuditWriteFailed"))
		})

		It("should create Grafana dashboard ConfigMaps for applications (11.3)", func() {
			app := &choristerv1alpha1.ChoApplication{
				ObjectMeta: metav1.ObjectMeta{Name: "dashboard-app", Namespace: "default"},
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"test@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion: choristerv1alpha1.PromotionPolicy{
							RequiredApprovers: 1,
							AllowedRoles:      []string{"developer"},
						},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: "payments"},
						{Name: "auth"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, app)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, app) }()

			cluster := &choristerv1alpha1.ChoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "dashboard-cluster"},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cluster) }()

			reconciler := &ChoClusterReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				AuditLogger: audit.NewNoopLogger(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cluster.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert dashboard ConfigMaps exist in monitoring namespace
			for _, domain := range []string{"payments", "auth"} {
				cm := &corev1.ConfigMap{}
				cmName := fmt.Sprintf("dashboard-%s-%s", app.Name, domain)
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: cmName, Namespace: "cho-monitoring",
				}, cm)).To(Succeed(), "expected dashboard ConfigMap %q", cmName)
				Expect(cm.Labels).To(HaveKeyWithValue("grafana_dashboard", "1"))
			}
		})
	})

	// -----------------------------------------------------------------------
	// 1A.12 — ChoCluster bootstrap (envtest)
	// -----------------------------------------------------------------------

	Context("1A.12 — ChoCluster bootstrap", func() {
		It("should trigger operator installations", func() {
			cluster := &choristerv1alpha1.ChoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-cluster"},
				Spec: choristerv1alpha1.ChoClusterSpec{
					Operators: &choristerv1alpha1.OperatorVersions{
						Kro:         "latest",
						StackGres:   "latest",
						NATS:        "latest",
						Dragonfly:   "latest",
						CertManager: "latest",
						Gatekeeper:  "latest",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cluster) }()

			reconciler := &ChoClusterReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				AuditLogger: audit.NewNoopLogger(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cluster.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			// Assert operator status tracked
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cluster.Name}, cluster)).To(Succeed())
			Expect(cluster.Status.OperatorStatus).NotTo(BeEmpty())
			for _, name := range []string{"kro", "stackgres", "nats", "dragonfly", "cert-manager", "gatekeeper"} {
				Expect(cluster.Status.OperatorStatus).To(HaveKeyWithValue(name, "Installed"))
			}

			// Assert operator Deployments exist
			for _, def := range []struct{ name, ns string }{
				{"kro", "cho-kro-system"},
				{"stackgres", "cho-stackgres-system"},
				{"nats", "cho-nats-system"},
				{"dragonfly", "cho-dragonfly-system"},
				{"cert-manager", "cho-cert-manager-system"},
				{"gatekeeper", "cho-gatekeeper-system"},
			} {
				deploy := &appsv1.Deployment{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name: def.name + "-operator", Namespace: def.ns,
				}, deploy)).To(Succeed(), "expected operator Deployment %q in %q", def.name, def.ns)
			}
		})

		It("should reinstall deleted operator", func() {
			cluster := &choristerv1alpha1.ChoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "reinstall-cluster"},
				Spec: choristerv1alpha1.ChoClusterSpec{
					Operators: &choristerv1alpha1.OperatorVersions{
						Kro: "latest",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cluster) }()

			reconciler := &ChoClusterReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				AuditLogger: audit.NewNoopLogger(),
			}

			// First reconcile → operator installed
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cluster.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			deploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "kro-operator", Namespace: "cho-kro-system",
			}, deploy)).To(Succeed())

			// Delete the operator Deployment
			Expect(k8sClient.Delete(ctx, deploy)).To(Succeed())

			// Verify it's gone
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name: "kro-operator", Namespace: "cho-kro-system",
			}, &appsv1.Deployment{})
			Expect(errors.IsNotFound(err)).To(BeTrue())

			// Re-fetch cluster to avoid resource version conflicts
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cluster.Name}, cluster)).To(Succeed())

			// Second reconcile → operator reinstalled
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cluster.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify reinstalled
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: "kro-operator", Namespace: "cho-kro-system",
			}, &appsv1.Deployment{})).To(Succeed())
		})

		It("should make sizing templates available for resource compilation", func() {
			cluster := &choristerv1alpha1.ChoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "sizing-cluster"},
				Spec: choristerv1alpha1.ChoClusterSpec{
					SizingTemplates: map[string]choristerv1alpha1.SizingTemplateSet{
						"database": {
							Templates: map[string]choristerv1alpha1.SizingTemplate{
								"small":  {CPU: resource.MustParse("250m"), Memory: resource.MustParse("512Mi")},
								"medium": {CPU: resource.MustParse("1"), Memory: resource.MustParse("2Gi")},
								"large":  {CPU: resource.MustParse("4"), Memory: resource.MustParse("8Gi")},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cluster) }()

			reconciler := &ChoClusterReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				AuditLogger: audit.NewNoopLogger(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cluster.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cluster.Name}, cluster)).To(Succeed())
			Expect(cluster.Spec.SizingTemplates).To(HaveKey("database"))
			Expect(cluster.Spec.SizingTemplates["database"].Templates).To(HaveKey("medium"))

			// Check condition
			var sizingCondition *metav1.Condition
			for i := range cluster.Status.Conditions {
				if cluster.Status.Conditions[i].Type == "SizingTemplatesAvailable" {
					sizingCondition = &cluster.Status.Conditions[i]
					break
				}
			}
			Expect(sizingCondition).NotTo(BeNil())
			Expect(sizingCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(sizingCondition.Reason).To(Equal("UserDefined"))
		})

		It("should expose FinOps cost rates", func() {
			cpuRate := resource.MustParse("0.05")
			memRate := resource.MustParse("0.01")
			storageRate := resource.MustParse("0.10")

			cluster := &choristerv1alpha1.ChoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "finops-cluster"},
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

			reconciler := &ChoClusterReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				AuditLogger: audit.NewNoopLogger(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cluster.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify the cluster reconciled successfully with FinOps rates
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cluster.Name}, cluster)).To(Succeed())
			Expect(cluster.Status.Phase).To(Equal("Ready"))

			// Verify cost rates are accessible
			Expect(cluster.Spec.FinOps).NotTo(BeNil())
			Expect(cluster.Spec.FinOps.Rates).NotTo(BeNil())
			Expect(cluster.Spec.FinOps.Rates.CPUPerHour.AsApproximateFloat64()).To(BeNumerically("~", 0.05, 0.001))
			Expect(cluster.Spec.FinOps.Rates.MemoryPerGBHour.AsApproximateFloat64()).To(BeNumerically("~", 0.01, 0.001))
			Expect(cluster.Spec.FinOps.Rates.StoragePerGBMonth.AsApproximateFloat64()).To(BeNumerically("~", 0.10, 0.001))
		})

		It("should install default sizing templates on bootstrap", func() {
			cluster := &choristerv1alpha1.ChoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "default-sizing-cluster"},
				Spec:       choristerv1alpha1.ChoClusterSpec{},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cluster) }()

			reconciler := &ChoClusterReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				AuditLogger: audit.NewNoopLogger(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cluster.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			// Re-fetch the cluster to see updated spec
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cluster.Name}, cluster)).To(Succeed())
			Expect(cluster.Spec.SizingTemplates).NotTo(BeEmpty())
			for _, resType := range []string{"compute", "database", "cache", "queue"} {
				Expect(cluster.Spec.SizingTemplates).To(HaveKey(resType), "expected default templates for %s", resType)
				set := cluster.Spec.SizingTemplates[resType]
				for _, size := range []string{"small", "medium", "large"} {
					Expect(set.Templates).To(HaveKey(size), "expected %s size for %s", size, resType)
				}
			}

			// Check condition
			var sizingCondition *metav1.Condition
			for i := range cluster.Status.Conditions {
				if cluster.Status.Conditions[i].Type == "SizingTemplatesAvailable" {
					sizingCondition = &cluster.Status.Conditions[i]
					break
				}
			}
			Expect(sizingCondition).NotTo(BeNil())
			Expect(sizingCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(sizingCondition.Reason).To(Equal("DefaultsInstalled"))
		})

		It("should block reconciliation on audit write failure", func() {
			cluster := &choristerv1alpha1.ChoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "audit-block-cluster"},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cluster) }()

			reconciler := &ChoClusterReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				AuditLogger: audit.NewFailingLogger(fmt.Errorf("loki connection refused")),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cluster.Name},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("audit write failed"))
		})

		It("should warn about missing encrypted StorageClass (12.3)", func() {
			cluster := &choristerv1alpha1.ChoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "sc-cluster"},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cluster) }()

			reconciler := &ChoClusterReconciler{
				Client:      k8sClient,
				Scheme:      k8sClient.Scheme(),
				AuditLogger: audit.NewNoopLogger(),
			}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: cluster.Name},
			})
			Expect(err).NotTo(HaveOccurred())

			// In envtest, there are no StorageClasses — condition should be False
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cluster.Name}, cluster)).To(Succeed())
			var encCondition *metav1.Condition
			for i := range cluster.Status.Conditions {
				if cluster.Status.Conditions[i].Type == "EncryptedStorageAvailable" {
					encCondition = &cluster.Status.Conditions[i]
					break
				}
			}
			Expect(encCondition).NotTo(BeNil())
			Expect(encCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(encCondition.Reason).To(Equal("NotFound"))
		})

		It("should create kube-bench CronJob and update CIS benchmark status", func() {
			cluster := &choristerv1alpha1.ChoCluster{ObjectMeta: metav1.ObjectMeta{Name: "kube-bench-cluster"}}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cluster) }()

			reconciler := &ChoClusterReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), AuditLogger: audit.NewNoopLogger()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name}})
			Expect(err).NotTo(HaveOccurred())

			cronJob := &batchv1.CronJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "kube-bench", Namespace: "cho-system"}, cronJob)).To(Succeed())
			Expect(cronJob.Spec.Schedule).To(Equal("0 4 * * 0"))

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: cluster.Name}, cluster)).To(Succeed())
			Expect(cluster.Status.CISBenchmark).To(ContainSubstring("kube-bench"))
		})
	})

	Context("Phase 15.2 — Tetragon operator", func() {
		It("should include Tetragon in operator defs when version is set", func() {
			ops := &choristerv1alpha1.OperatorVersions{Tetragon: "v1.3.0"}
			defs := operatorDefs(ops)
			Expect(defs).To(HaveLen(1))
			Expect(defs[0].name).To(Equal("tetragon"))
			Expect(defs[0].namespace).To(Equal("cho-tetragon-system"))
		})
	})

	Context("Phase 16.1 — cert-manager ClusterIssuer", func() {
		It("should create ClusterIssuer when cert-manager is configured", func() {
			cluster := &choristerv1alpha1.ChoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "cert-cluster"},
				Spec: choristerv1alpha1.ChoClusterSpec{
					Operators: &choristerv1alpha1.OperatorVersions{CertManager: "v1.16.0"},
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cluster) }()

			reconciler := &ChoClusterReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), AuditLogger: audit.NewNoopLogger()}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: cluster.Name}})
			Expect(err).NotTo(HaveOccurred())

			// Verify ClusterIssuer was created
			issuer := &unstructured.Unstructured{}
			issuer.SetGroupVersionKind(schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "ClusterIssuer"})
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "chorister-cluster-issuer"}, issuer)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
