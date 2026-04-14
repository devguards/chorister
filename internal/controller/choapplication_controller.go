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

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
	applicationFinalizerName = "chorister.dev/application-cleanup"
	labelApplication         = "chorister.dev/application"
	labelDomain              = "chorister.dev/domain"
)

// ChoApplicationReconciler reconciles a ChoApplication object
type ChoApplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=chorister.dev,resources=choapplications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=choapplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=choapplications/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=limitranges,verbs=get;list;watch;create;update;patch;delete

// Reconcile moves the cluster state to match the desired ChoApplication spec.
func (r *ChoApplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the ChoApplication
	app := &choristerv1alpha1.ChoApplication{}
	if err := r.Get(ctx, req.NamespacedName, app); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion via finalizer
	if !app.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(app, applicationFinalizerName) {
			if err := r.cleanupDomainNamespaces(ctx, app); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(app, applicationFinalizerName)
			if err := r.Update(ctx, app); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(app, applicationFinalizerName) {
		controllerutil.AddFinalizer(app, applicationFinalizerName)
		if err := r.Update(ctx, app); err != nil {
			return ctrl.Result{}, err
		}
		// Re-fetch after update to avoid conflicts
		if err := r.Get(ctx, req.NamespacedName, app); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Build desired domain namespace set
	desiredDomains := make(map[string]string, len(app.Spec.Domains))
	for _, d := range app.Spec.Domains {
		nsName := fmt.Sprintf("%s-%s", app.Name, d.Name)
		desiredDomains[d.Name] = nsName
	}

	// Reconcile namespaces and their resources
	for _, domain := range app.Spec.Domains {
		nsName := desiredDomains[domain.Name]

		if err := r.ensureNamespace(ctx, app, domain.Name, nsName); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.ensureDefaultDenyNetworkPolicy(ctx, app, nsName); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.ensureResourceQuota(ctx, app, nsName); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.ensureLimitRange(ctx, app, nsName); err != nil {
			return ctrl.Result{}, err
		}

		log.Info("Reconciled domain namespace", "namespace", nsName, "domain", domain.Name)
	}

	// Remove namespaces for domains that no longer exist
	if app.Status.DomainNamespaces != nil {
		for domainName, nsName := range app.Status.DomainNamespaces {
			if _, exists := desiredDomains[domainName]; !exists {
				if err := r.deleteNamespace(ctx, nsName); err != nil {
					return ctrl.Result{}, err
				}
				log.Info("Deleted removed domain namespace", "namespace", nsName, "domain", domainName)
			}
		}
	}

	// Update status
	app.Status.DomainNamespaces = desiredDomains
	app.Status.Phase = "Active"
	setCondition(&app.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "All domain namespaces reconciled",
		ObservedGeneration: app.Generation,
	})

	if err := r.Status().Update(ctx, app); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// ensureNamespace creates or updates a domain namespace with the correct labels.
func (r *ChoApplicationReconciler) ensureNamespace(ctx context.Context, app *choristerv1alpha1.ChoApplication, domainName, nsName string) error {
	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: nsName}, ns)
	if errors.IsNotFound(err) {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
				Labels: map[string]string{
					labelApplication: app.Name,
					labelDomain:      domainName,
				},
			},
		}
		return r.Create(ctx, ns)
	}
	if err != nil {
		return err
	}

	// Update labels if needed
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}
	needsUpdate := false
	if ns.Labels[labelApplication] != app.Name {
		ns.Labels[labelApplication] = app.Name
		needsUpdate = true
	}
	if ns.Labels[labelDomain] != domainName {
		ns.Labels[labelDomain] = domainName
		needsUpdate = true
	}
	if needsUpdate {
		return r.Update(ctx, ns)
	}
	return nil
}

// ensureDefaultDenyNetworkPolicy creates a deny-all ingress+egress NetworkPolicy
// with an exception for DNS egress (kube-dns port 53).
func (r *ChoApplicationReconciler) ensureDefaultDenyNetworkPolicy(ctx context.Context, app *choristerv1alpha1.ChoApplication, nsName string) error {
	policyName := "default-deny"
	udp := corev1.ProtocolUDP
	tcp := corev1.ProtocolTCP
	dnsPort := intstr.FromInt32(53)

	desired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: nsName,
			Labels: map[string]string{
				labelApplication: app.Name,
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{}, // select all pods
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{}, // deny all ingress
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					// Allow DNS egress
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
	if err != nil {
		return err
	}

	// Update if spec changed
	if !equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		existing.Spec = desired.Spec
		existing.Labels = desired.Labels
		return r.Update(ctx, existing)
	}
	return nil
}

// ensureResourceQuota creates a ResourceQuota in the domain namespace if quotas are configured.
func (r *ChoApplicationReconciler) ensureResourceQuota(ctx context.Context, app *choristerv1alpha1.ChoApplication, nsName string) error {
	if app.Spec.Policy.Quotas == nil || app.Spec.Policy.Quotas.DefaultPerDomain == nil {
		return nil
	}

	quota := app.Spec.Policy.Quotas.DefaultPerDomain
	quotaName := "domain-quota"

	hard := corev1.ResourceList{}
	if !quota.CPU.IsZero() {
		hard[corev1.ResourceLimitsCPU] = quota.CPU
		hard[corev1.ResourceRequestsCPU] = quota.CPU
	}
	if !quota.Memory.IsZero() {
		hard[corev1.ResourceLimitsMemory] = quota.Memory
		hard[corev1.ResourceRequestsMemory] = quota.Memory
	}
	if !quota.Storage.IsZero() {
		hard[corev1.ResourceRequestsStorage] = quota.Storage
	}

	desired := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      quotaName,
			Namespace: nsName,
			Labels: map[string]string{
				labelApplication: app.Name,
			},
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: hard,
		},
	}

	existing := &corev1.ResourceQuota{}
	err := r.Get(ctx, types.NamespacedName{Name: quotaName, Namespace: nsName}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	if !equality.Semantic.DeepEqual(existing.Spec.Hard, desired.Spec.Hard) {
		existing.Spec.Hard = desired.Spec.Hard
		existing.Labels = desired.Labels
		return r.Update(ctx, existing)
	}
	return nil
}

// ensureLimitRange creates a LimitRange in the domain namespace if quotas are configured.
func (r *ChoApplicationReconciler) ensureLimitRange(ctx context.Context, app *choristerv1alpha1.ChoApplication, nsName string) error {
	if app.Spec.Policy.Quotas == nil || app.Spec.Policy.Quotas.DefaultPerDomain == nil {
		return nil
	}

	quota := app.Spec.Policy.Quotas.DefaultPerDomain
	limitRangeName := "domain-limit-range"

	defaultLimit := corev1.ResourceList{}
	defaultRequest := corev1.ResourceList{}
	if !quota.CPU.IsZero() {
		// Default container limit = 1/4 of total quota, request = 1/8
		cpuMillis := quota.CPU.MilliValue()
		defaultLimit[corev1.ResourceCPU] = *resource.NewMilliQuantity(cpuMillis/4, resource.DecimalSI)
		defaultRequest[corev1.ResourceCPU] = *resource.NewMilliQuantity(cpuMillis/8, resource.DecimalSI)
	}
	if !quota.Memory.IsZero() {
		// Default container limit = 1/4 of total quota, request = 1/8
		memBytes := quota.Memory.Value()
		defaultLimit[corev1.ResourceMemory] = *resource.NewQuantity(memBytes/4, resource.BinarySI)
		defaultRequest[corev1.ResourceMemory] = *resource.NewQuantity(memBytes/8, resource.BinarySI)
	}

	desired := &corev1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:      limitRangeName,
			Namespace: nsName,
			Labels: map[string]string{
				labelApplication: app.Name,
			},
		},
		Spec: corev1.LimitRangeSpec{
			Limits: []corev1.LimitRangeItem{
				{
					Type:           corev1.LimitTypeContainer,
					Default:        defaultLimit,
					DefaultRequest: defaultRequest,
				},
			},
		},
	}

	existing := &corev1.LimitRange{}
	err := r.Get(ctx, types.NamespacedName{Name: limitRangeName, Namespace: nsName}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	if !equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		existing.Spec = desired.Spec
		existing.Labels = desired.Labels
		return r.Update(ctx, existing)
	}
	return nil
}

// cleanupDomainNamespaces deletes all domain namespaces tracked in status.
func (r *ChoApplicationReconciler) cleanupDomainNamespaces(ctx context.Context, app *choristerv1alpha1.ChoApplication) error {
	log := logf.FromContext(ctx)
	for domainName, nsName := range app.Status.DomainNamespaces {
		if err := r.deleteNamespace(ctx, nsName); err != nil {
			return err
		}
		log.Info("Cleaned up domain namespace", "namespace", nsName, "domain", domainName)
	}
	return nil
}

// deleteNamespace deletes a namespace if it exists.
func (r *ChoApplicationReconciler) deleteNamespace(ctx context.Context, nsName string) error {
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

// setCondition updates or adds a condition in the conditions slice.
func setCondition(conditions *[]metav1.Condition, condition metav1.Condition) {
	now := metav1.Now()
	condition.LastTransitionTime = now
	for i, existing := range *conditions {
		if existing.Type == condition.Type {
			if existing.Status != condition.Status {
				(*conditions)[i] = condition
			} else {
				// Keep existing transition time if status hasn't changed
				condition.LastTransitionTime = existing.LastTransitionTime
				(*conditions)[i] = condition
			}
			return
		}
	}
	*conditions = append(*conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChoApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choristerv1alpha1.ChoApplication{}).
		Named("choapplication").
		Complete(r)
}
