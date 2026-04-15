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
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

const (
	sandboxFinalizerName = "chorister.dev/sandbox-cleanup"
	labelSandbox         = "chorister.dev/sandbox"

	// hoursPerMonth is used for FinOps monthly cost estimation (730h/month).
	hoursPerMonth = 730.0
	// sandboxRequeueInterval is how often sandboxes are re-checked for idle detection.
	sandboxRequeueInterval = 1 * time.Hour
)

// ChoSandboxReconciler reconciles a ChoSandbox object
type ChoSandboxReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=chorister.dev,resources=chosandboxes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chosandboxes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=chosandboxes/finalizers,verbs=update
// +kubebuilder:rbac:groups=chorister.dev,resources=choapplications,verbs=get;list;watch
// +kubebuilder:rbac:groups=chorister.dev,resources=choclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=chorister.dev,resources=chocomputes,verbs=get;list;watch
// +kubebuilder:rbac:groups=chorister.dev,resources=chodatabases,verbs=get;list;watch
// +kubebuilder:rbac:groups=chorister.dev,resources=choqueues,verbs=get;list;watch
// +kubebuilder:rbac:groups=chorister.dev,resources=chocaches,verbs=get;list;watch

// SandboxNamespace returns the namespace name for a sandbox.
func SandboxNamespace(app, domain, name string) string {
	return fmt.Sprintf("%s-%s-sandbox-%s", app, domain, name)
}

// Reconcile moves the cluster state to match the desired ChoSandbox spec.
func (r *ChoSandboxReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	sandbox := &choristerv1alpha1.ChoSandbox{}
	if err := r.Get(ctx, req.NamespacedName, sandbox); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	nsName := SandboxNamespace(sandbox.Spec.Application, sandbox.Spec.Domain, sandbox.Spec.Name)

	// Handle deletion via finalizer
	if !sandbox.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(sandbox, sandboxFinalizerName) {
			if err := r.deleteSandboxNamespace(ctx, nsName); err != nil {
				return ctrl.Result{}, err
			}
			log.Info("Cleaned up sandbox namespace", "namespace", nsName)

			controllerutil.RemoveFinalizer(sandbox, sandboxFinalizerName)
			if err := r.Update(ctx, sandbox); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(sandbox, sandboxFinalizerName) {
		controllerutil.AddFinalizer(sandbox, sandboxFinalizerName)
		if err := r.Update(ctx, sandbox); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Get(ctx, req.NamespacedName, sandbox); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Look up parent ChoApplication for sandbox policies
	app := r.lookupApplication(ctx, sandbox)

	// Phase 20.3: Budget enforcement — check before creating namespace
	if app != nil && app.Spec.Policy.Sandbox != nil && app.Spec.Policy.Sandbox.DefaultBudgetPerDomain != nil {
		exceeded, totalCost, budget, err := r.checkDomainBudget(ctx, sandbox, app)
		if err != nil {
			log.Error(err, "Could not check domain budget")
		} else if exceeded {
			sandbox.Status.Phase = "BudgetExceeded"
			setCondition(&sandbox.Status.Conditions, metav1.Condition{
				Type:               "BudgetExceeded",
				Status:             metav1.ConditionTrue,
				Reason:             "DomainBudgetExceeded",
				Message:            fmt.Sprintf("Domain sandbox budget exceeded: total $%.2f exceeds budget $%.2f", totalCost, budget),
				ObservedGeneration: sandbox.Generation,
			})
			if err := r.Status().Update(ctx, sandbox); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: sandboxRequeueInterval}, nil
		}
	}

	// Ensure sandbox namespace exists
	if err := r.ensureSandboxNamespace(ctx, sandbox, nsName); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure default-deny NetworkPolicy in sandbox namespace
	if err := r.ensureSandboxDenyPolicy(ctx, sandbox, nsName); err != nil {
		return ctrl.Result{}, err
	}

	// Initialize lastApplyTime if not set
	if sandbox.Status.LastApplyTime == nil {
		now := metav1.Now()
		sandbox.Status.LastApplyTime = &now
	}

	// Phase 20.2: Estimate sandbox cost
	estimatedCost := r.estimateSandboxCost(ctx, sandbox, nsName)
	sandbox.Status.EstimatedMonthlyCost = fmt.Sprintf("%.2f", estimatedCost)

	// Phase 20.1: Idle detection and auto-destroy
	if app != nil && app.Spec.Policy.Sandbox != nil && app.Spec.Policy.Sandbox.MaxIdleDays != nil {
		maxIdleDays := *app.Spec.Policy.Sandbox.MaxIdleDays
		if maxIdleDays > 0 && sandbox.Status.LastApplyTime != nil {
			idleDuration := time.Since(sandbox.Status.LastApplyTime.Time)
			threshold := time.Duration(maxIdleDays) * 24 * time.Hour
			warningThreshold := threshold - 24*time.Hour

			if idleDuration >= threshold {
				// Auto-destroy: delete the sandbox
				log.Info("Auto-destroying idle sandbox", "name", sandbox.Name, "idle", idleDuration.String())
				sandbox.Status.Phase = "Destroying"
				setCondition(&sandbox.Status.Conditions, metav1.Condition{
					Type:               "IdleAutoDestroy",
					Status:             metav1.ConditionTrue,
					Reason:             "IdleThresholdExceeded",
					Message:            fmt.Sprintf("Sandbox idle for %s, exceeds max idle of %d days", idleDuration.Truncate(time.Hour).String(), maxIdleDays),
					ObservedGeneration: sandbox.Generation,
				})
				if err := r.Status().Update(ctx, sandbox); err != nil {
					return ctrl.Result{}, err
				}
				// Trigger deletion
				if err := r.Delete(ctx, sandbox); err != nil && !errors.IsNotFound(err) {
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			} else if idleDuration >= warningThreshold {
				// 24h warning
				setCondition(&sandbox.Status.Conditions, metav1.Condition{
					Type:               "IdleWarning",
					Status:             metav1.ConditionTrue,
					Reason:             "ApproachingIdleThreshold",
					Message:            fmt.Sprintf("Sandbox idle for %s, will be auto-destroyed after %d days", idleDuration.Truncate(time.Hour).String(), maxIdleDays),
					ObservedGeneration: sandbox.Generation,
				})
			}
		}
	}

	// Update status
	sandbox.Status.Namespace = nsName
	if sandbox.Status.Phase != "BudgetExceeded" && sandbox.Status.Phase != "Destroying" {
		sandbox.Status.Phase = "Active"
	}
	setCondition(&sandbox.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "NamespaceReady",
		Message:            fmt.Sprintf("Sandbox namespace %s is ready", nsName),
		ObservedGeneration: sandbox.Generation,
	})
	if err := r.Status().Update(ctx, sandbox); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciled ChoSandbox", "name", sandbox.Name, "namespace", nsName)
	return ctrl.Result{RequeueAfter: sandboxRequeueInterval}, nil
}

func (r *ChoSandboxReconciler) ensureSandboxNamespace(ctx context.Context, sandbox *choristerv1alpha1.ChoSandbox, nsName string) error {
	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: nsName}, ns)
	if errors.IsNotFound(err) {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
				Labels: map[string]string{
					labelApplication: sandbox.Spec.Application,
					labelDomain:      sandbox.Spec.Domain,
					labelSandbox:     sandbox.Spec.Name,
				},
			},
		}
		return r.Create(ctx, ns)
	}
	return err
}

func (r *ChoSandboxReconciler) ensureSandboxDenyPolicy(ctx context.Context, sandbox *choristerv1alpha1.ChoSandbox, nsName string) error {
	policyName := "default-deny"
	udp := corev1.ProtocolUDP
	tcp := corev1.ProtocolTCP
	dnsPort := intstr.FromInt32(53)

	desired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: nsName,
			Labels: map[string]string{
				labelApplication: sandbox.Spec.Application,
				labelSandbox:     sandbox.Spec.Name,
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &udp, Port: &dnsPort},
						{Protocol: &tcp, Port: &dnsPort},
					},
				},
			},
		},
	}

	existing := &networkingv1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: nsName}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	return err
}

func (r *ChoSandboxReconciler) deleteSandboxNamespace(ctx context.Context, nsName string) error {
	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: nsName}, ns)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.Delete(ctx, ns)
}

// lookupApplication finds the parent ChoApplication in the sandbox's namespace.
func (r *ChoSandboxReconciler) lookupApplication(ctx context.Context, sandbox *choristerv1alpha1.ChoSandbox) *choristerv1alpha1.ChoApplication {
	app := &choristerv1alpha1.ChoApplication{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      sandbox.Spec.Application,
		Namespace: sandbox.Namespace,
	}, app); err != nil {
		return nil
	}
	return app
}

// estimateSandboxCost calculates the estimated monthly cost of sandbox resources
// using cost rates from the ChoCluster.
func (r *ChoSandboxReconciler) estimateSandboxCost(ctx context.Context, sandbox *choristerv1alpha1.ChoSandbox, nsName string) float64 {
	rates := r.lookupCostRates(ctx)
	if rates == nil {
		return 0
	}

	var totalCPUCores, totalMemoryGB, totalStorageGB float64

	// Sum ChoCompute resources
	computeList := &choristerv1alpha1.ChoComputeList{}
	if err := r.List(ctx, computeList, client.InNamespace(nsName)); err == nil {
		for _, c := range computeList.Items {
			cpu, mem := extractResourceRequests(c.Spec.Resources)
			replicas := float64(1)
			if c.Spec.Replicas != nil {
				replicas = float64(*c.Spec.Replicas)
			}
			totalCPUCores += cpu * replicas
			totalMemoryGB += mem * replicas
		}
	}

	// Sum ChoDatabase resources
	dbList := &choristerv1alpha1.ChoDatabaseList{}
	if err := r.List(ctx, dbList, client.InNamespace(nsName)); err == nil {
		for _, d := range dbList.Items {
			cpu, mem := extractResourceRequests(d.Spec.Resources)
			instances := 1.0
			if d.Spec.HA {
				instances = 2.0
			}
			totalCPUCores += cpu * instances
			totalMemoryGB += mem * instances
			totalStorageGB += extractStorageGB(d.Spec.Resources)
		}
	}

	// Sum ChoQueue resources
	queueList := &choristerv1alpha1.ChoQueueList{}
	if err := r.List(ctx, queueList, client.InNamespace(nsName)); err == nil {
		for _, q := range queueList.Items {
			cpu, mem := extractResourceRequests(q.Spec.Resources)
			totalCPUCores += cpu
			totalMemoryGB += mem
			totalStorageGB += extractStorageGB(q.Spec.Resources)
		}
	}

	// Sum ChoCache resources
	cacheList := &choristerv1alpha1.ChoCacheList{}
	if err := r.List(ctx, cacheList, client.InNamespace(nsName)); err == nil {
		for _, c := range cacheList.Items {
			cpu, mem := extractResourceRequests(c.Spec.Resources)
			totalCPUCores += cpu
			totalMemoryGB += mem
		}
	}

	// Calculate monthly cost
	var cost float64
	if rates.CPUPerHour != nil {
		cost += totalCPUCores * rates.CPUPerHour.AsApproximateFloat64() * hoursPerMonth
	}
	if rates.MemoryPerGBHour != nil {
		cost += totalMemoryGB * rates.MemoryPerGBHour.AsApproximateFloat64() * hoursPerMonth
	}
	if rates.StoragePerGBMonth != nil {
		cost += totalStorageGB * rates.StoragePerGBMonth.AsApproximateFloat64()
	}

	return math.Round(cost*100) / 100
}

// lookupCostRates finds cost rates from the first ChoCluster in the cluster.
func (r *ChoSandboxReconciler) lookupCostRates(ctx context.Context) *choristerv1alpha1.CostRates {
	clusterList := &choristerv1alpha1.ChoClusterList{}
	if err := r.List(ctx, clusterList); err != nil || len(clusterList.Items) == 0 {
		return nil
	}
	cluster := &clusterList.Items[0]
	if cluster.Spec.FinOps == nil || cluster.Spec.FinOps.Rates == nil {
		return nil
	}
	return cluster.Spec.FinOps.Rates
}

// checkDomainBudget checks if creating/maintaining this sandbox would exceed
// the domain's sandbox budget. Returns (exceeded, totalCost, budget, error).
func (r *ChoSandboxReconciler) checkDomainBudget(ctx context.Context, sandbox *choristerv1alpha1.ChoSandbox, app *choristerv1alpha1.ChoApplication) (bool, float64, float64, error) {
	budget := float64(*app.Spec.Policy.Sandbox.DefaultBudgetPerDomain)
	if budget <= 0 {
		return false, 0, 0, nil
	}

	// List all sandboxes for the same application and domain
	sandboxList := &choristerv1alpha1.ChoSandboxList{}
	if err := r.List(ctx, sandboxList, client.InNamespace(sandbox.Namespace)); err != nil {
		return false, 0, budget, err
	}

	var totalCost float64
	for i := range sandboxList.Items {
		s := &sandboxList.Items[i]
		if s.Spec.Application == sandbox.Spec.Application && s.Spec.Domain == sandbox.Spec.Domain {
			nsName := SandboxNamespace(s.Spec.Application, s.Spec.Domain, s.Spec.Name)
			cost := r.estimateSandboxCost(ctx, s, nsName)
			totalCost += cost
		}
	}

	return totalCost > budget, totalCost, budget, nil
}

// extractResourceRequests extracts CPU (in cores) and memory (in GB) from resource requirements.
func extractResourceRequests(resources *corev1.ResourceRequirements) (cpuCores, memoryGB float64) {
	if resources == nil {
		return 0, 0
	}
	if resources.Requests != nil {
		if cpu, ok := resources.Requests[corev1.ResourceCPU]; ok {
			cpuCores = cpu.AsApproximateFloat64()
		}
		if mem, ok := resources.Requests[corev1.ResourceMemory]; ok {
			memoryGB = mem.AsApproximateFloat64() / (1024 * 1024 * 1024)
		}
	}
	return cpuCores, memoryGB
}

// extractStorageGB extracts storage (in GB) from resource requirements.
func extractStorageGB(resources *corev1.ResourceRequirements) float64 {
	if resources == nil || resources.Requests == nil {
		return 0
	}
	if storage, ok := resources.Requests[corev1.ResourceStorage]; ok {
		return storage.AsApproximateFloat64() / (1024 * 1024 * 1024)
	}
	return 0
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChoSandboxReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choristerv1alpha1.ChoSandbox{}).
		Named("chosandbox").
		Complete(r)
}
