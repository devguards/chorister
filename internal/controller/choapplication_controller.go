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
	"strings"

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

	// Validate consumes/supplies declarations
	validationErrors := validateConsumesSupplies(app)
	cycleErr := validateNoCycles(app)
	if cycleErr != nil {
		validationErrors = append(validationErrors, cycleErr.Error())
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

		// Create allow-rules based on consumes declarations
		if err := r.ensureConsumesNetworkPolicies(ctx, app, domain, desiredDomains); err != nil {
			return ctrl.Result{}, err
		}

		// Create ingress-allow rules for domains that supply
		if err := r.ensureSuppliesNetworkPolicies(ctx, app, domain, desiredDomains); err != nil {
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

	if len(validationErrors) > 0 {
		setCondition(&app.Status.Conditions, metav1.Condition{
			Type:               "Valid",
			Status:             metav1.ConditionFalse,
			Reason:             "ValidationFailed",
			Message:            strings.Join(validationErrors, "; "),
			ObservedGeneration: app.Generation,
		})
	} else {
		setCondition(&app.Status.Conditions, metav1.Condition{
			Type:               "Valid",
			Status:             metav1.ConditionTrue,
			Reason:             "Valid",
			Message:            "All consumes/supplies declarations are valid",
			ObservedGeneration: app.Generation,
		})
	}

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

// ensureConsumesNetworkPolicies creates NetworkPolicies allowing egress from a consumer
// domain to the supplied domain's namespace on the declared port.
func (r *ChoApplicationReconciler) ensureConsumesNetworkPolicies(ctx context.Context, app *choristerv1alpha1.ChoApplication, domain choristerv1alpha1.DomainSpec, domainNamespaces map[string]string) error {
	consumerNs := domainNamespaces[domain.Name]

	for _, consume := range domain.Consumes {
		_, exists := domainNamespaces[consume.Domain]
		if !exists {
			continue // validation will catch this
		}

		// Find the supplier domain to check it actually supplies on this port
		supplierDomain := findDomain(app, consume.Domain)
		if supplierDomain == nil || supplierDomain.Supplies == nil || supplierDomain.Supplies.Port != consume.Port {
			continue // validation will catch mismatches
		}

		tcp := corev1.ProtocolTCP
		port := intstr.FromInt32(int32(consume.Port))
		policyName := fmt.Sprintf("allow-egress-to-%s-%d", consume.Domain, consume.Port)

		desired := &networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      policyName,
				Namespace: consumerNs,
				Labels: map[string]string{
					labelApplication:            app.Name,
					"chorister.dev/netpol-type": "consumes-egress",
				},
			},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{}, // all pods in consumer ns
				PolicyTypes: []networkingv1.PolicyType{
					networkingv1.PolicyTypeEgress,
				},
				Egress: []networkingv1.NetworkPolicyEgressRule{
					{
						To: []networkingv1.NetworkPolicyPeer{
							{
								NamespaceSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										labelApplication: app.Name,
										labelDomain:      consume.Domain,
									},
								},
							},
						},
						Ports: []networkingv1.NetworkPolicyPort{
							{Protocol: &tcp, Port: &port},
						},
					},
				},
			},
		}

		existing := &networkingv1.NetworkPolicy{}
		err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: consumerNs}, existing)
		if errors.IsNotFound(err) {
			if err := r.Create(ctx, desired); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}

		if !equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
			existing.Spec = desired.Spec
			existing.Labels = desired.Labels
			if err := r.Update(ctx, existing); err != nil {
				return err
			}
		}
	}

	// Clean up stale consumes policies: remove egress allow policies for consumes
	// that no longer exist in the spec.
	existingPolicies := &networkingv1.NetworkPolicyList{}
	if err := r.List(ctx, existingPolicies, client.InNamespace(consumerNs),
		client.MatchingLabels{"chorister.dev/netpol-type": "consumes-egress"}); err != nil {
		return err
	}

	desiredNames := make(map[string]bool)
	for _, consume := range domain.Consumes {
		desiredNames[fmt.Sprintf("allow-egress-to-%s-%d", consume.Domain, consume.Port)] = true
	}

	for i := range existingPolicies.Items {
		if !desiredNames[existingPolicies.Items[i].Name] {
			if err := r.Delete(ctx, &existingPolicies.Items[i]); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

// ensureSuppliesNetworkPolicies creates NetworkPolicies allowing ingress to a supplier
// domain from consumer domains on the declared port.
func (r *ChoApplicationReconciler) ensureSuppliesNetworkPolicies(ctx context.Context, app *choristerv1alpha1.ChoApplication, domain choristerv1alpha1.DomainSpec, domainNamespaces map[string]string) error {
	if domain.Supplies == nil {
		// Clean up any stale supplies policies
		supplierNs := domainNamespaces[domain.Name]
		existingPolicies := &networkingv1.NetworkPolicyList{}
		if err := r.List(ctx, existingPolicies, client.InNamespace(supplierNs),
			client.MatchingLabels{"chorister.dev/netpol-type": "supplies-ingress"}); err != nil {
			return err
		}
		for i := range existingPolicies.Items {
			if err := r.Delete(ctx, &existingPolicies.Items[i]); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
		return nil
	}

	supplierNs := domainNamespaces[domain.Name]

	// Find all consuming domains
	var consumers []choristerv1alpha1.DomainSpec
	for _, d := range app.Spec.Domains {
		for _, consume := range d.Consumes {
			if consume.Domain == domain.Name && consume.Port == domain.Supplies.Port {
				consumers = append(consumers, d)
				break
			}
		}
	}

	if len(consumers) == 0 {
		return nil
	}

	tcp := corev1.ProtocolTCP
	port := intstr.FromInt32(int32(domain.Supplies.Port))

	// Build ingress peers from all consumer namespaces
	var ingressPeers []networkingv1.NetworkPolicyPeer
	for _, consumer := range consumers {
		ingressPeers = append(ingressPeers, networkingv1.NetworkPolicyPeer{
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					labelApplication: app.Name,
					labelDomain:      consumer.Name,
				},
			},
		})
	}

	policyName := fmt.Sprintf("allow-ingress-from-consumers-%d", domain.Supplies.Port)
	desired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: supplierNs,
			Labels: map[string]string{
				labelApplication:            app.Name,
				"chorister.dev/netpol-type": "supplies-ingress",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{}, // all pods in supplier ns
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From:  ingressPeers,
					Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &port}},
				},
			},
		},
	}

	existing := &networkingv1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: supplierNs}, existing)
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

// findDomain returns the DomainSpec with the given name, or nil.
func findDomain(app *choristerv1alpha1.ChoApplication, name string) *choristerv1alpha1.DomainSpec {
	for i := range app.Spec.Domains {
		if app.Spec.Domains[i].Name == name {
			return &app.Spec.Domains[i]
		}
	}
	return nil
}

// validateConsumesSupplies checks that every consumes reference has a matching supplies declaration.
func validateConsumesSupplies(app *choristerv1alpha1.ChoApplication) []string {
	domainMap := make(map[string]*choristerv1alpha1.DomainSpec, len(app.Spec.Domains))
	for i := range app.Spec.Domains {
		domainMap[app.Spec.Domains[i].Name] = &app.Spec.Domains[i]
	}

	var errs []string
	for _, domain := range app.Spec.Domains {
		for _, consume := range domain.Consumes {
			supplier, exists := domainMap[consume.Domain]
			if !exists {
				errs = append(errs, fmt.Sprintf("domain %q consumes %q but domain %q does not exist", domain.Name, consume.Domain, consume.Domain))
				continue
			}
			if supplier.Supplies == nil {
				errs = append(errs, fmt.Sprintf("domain %q consumes %q but %q does not declare supplies", domain.Name, consume.Domain, consume.Domain))
				continue
			}
			if supplier.Supplies.Port != consume.Port {
				errs = append(errs, fmt.Sprintf("domain %q consumes %q on port %d but %q supplies on port %d", domain.Name, consume.Domain, consume.Port, consume.Domain, supplier.Supplies.Port))
			}
		}
	}
	return errs
}

// validateNoCycles checks for circular dependencies in the consumes graph.
func validateNoCycles(app *choristerv1alpha1.ChoApplication) error {
	// Build adjacency list
	graph := make(map[string][]string)
	for _, domain := range app.Spec.Domains {
		for _, consume := range domain.Consumes {
			graph[domain.Name] = append(graph[domain.Name], consume.Domain)
		}
	}

	// DFS-based cycle detection
	const (
		white = 0 // unvisited
		gray  = 1 // in current path
		black = 2 // fully explored
	)

	color := make(map[string]int)
	parent := make(map[string]string)

	var dfs func(node string) (string, bool)
	dfs = func(node string) (string, bool) {
		color[node] = gray
		for _, neighbor := range graph[node] {
			if color[neighbor] == gray {
				// Found a cycle — reconstruct path
				cycle := []string{neighbor, node}
				cur := node
				for cur != neighbor {
					cur = parent[cur]
					cycle = append(cycle, cur)
				}
				// Reverse to get proper order
				for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
					cycle[i], cycle[j] = cycle[j], cycle[i]
				}
				return strings.Join(cycle, " → "), true
			}
			if color[neighbor] == white {
				parent[neighbor] = node
				if path, found := dfs(neighbor); found {
					return path, true
				}
			}
		}
		color[node] = black
		return "", false
	}

	for _, domain := range app.Spec.Domains {
		if color[domain.Name] == white {
			if path, found := dfs(domain.Name); found {
				return fmt.Errorf("dependency cycle detected: %s", path)
			}
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChoApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choristerv1alpha1.ChoApplication{}).
		Named("choapplication").
		Complete(r)
}
