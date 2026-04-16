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
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/audit"
	"github.com/chorister-dev/chorister/internal/compiler"
	"github.com/chorister-dev/chorister/internal/scanning"
	"github.com/chorister-dev/chorister/internal/validation"
)

const (
	applicationFinalizerName = "chorister.dev/application-cleanup"
	labelApplication         = "chorister.dev/application"
	labelDomain              = "chorister.dev/domain"
)

// ChoApplicationReconciler reconciles a ChoApplication object
type ChoApplicationReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	Scanner     scanning.Scanner
	AuditLogger audit.Logger
}

// +kubebuilder:rbac:groups=chorister.dev,resources=choapplications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=choapplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=choapplications/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=limitranges,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chovulnerabilityreports,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chovulnerabilityreports/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes;referencegrants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cilium.io,resources=ciliumnetworkpolicies;ciliumenvoyconfigs,verbs=get;list;watch;create;update;patch;delete

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

	// Audit: log the reconciliation start (fail-fast if audit sink fails)
	if r.AuditLogger == nil {
		log.Error(nil, "AuditLogger not configured, blocking reconciliation")
		return ctrl.Result{}, fmt.Errorf("audit write failed, blocking reconciliation: AuditLogger not configured")
	}
	if err := r.AuditLogger.Log(ctx, audit.Event{
		Timestamp:   time.Now(),
		Action:      "Reconcile",
		Resource:    "ChoApplication/" + app.Name,
		Namespace:   app.Namespace,
		Application: app.Name,
		Result:      "started",
	}); err != nil {
		log.Error(err, "Audit write failed, blocking reconciliation")
		setCondition(&app.Status.Conditions, metav1.Condition{
			Type:    "AuditReady",
			Status:  metav1.ConditionFalse,
			Reason:  "AuditWriteFailed",
			Message: fmt.Sprintf("Audit sink write failed: %v", err),
		})
		_ = r.Status().Update(ctx, app)
		return ctrl.Result{}, fmt.Errorf("audit write failed, blocking reconciliation: %w", err)
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

	// Validate compliance escalation (domain sensitivity vs app compliance)
	validationErrors = append(validationErrors, validation.ValidateComplianceEscalation(app)...)

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

		if err := r.ensurePeriodicVulnerabilityScanning(ctx, app, domain, nsName); err != nil {
			return ctrl.Result{}, err
		}

		// Phase 15.1: L7 CiliumNetworkPolicy for restricted domains
		if err := r.ensureRestrictedDomainL7Policy(ctx, app, domain); err != nil {
			return ctrl.Result{}, err
		}

		// Phase 15.2: Tetragon TracingPolicy for restricted/regulated domains
		if err := r.ensureTetragonTracingPolicy(ctx, app, domain); err != nil {
			return ctrl.Result{}, err
		}

		// Phase 16.1: cert-manager Certificate for confidential/restricted domains
		if err := r.ensureDomainCertificate(ctx, app, domain); err != nil {
			return ctrl.Result{}, err
		}

		// Phase 16.2: Cilium encryption policy for confidential/restricted cross-domain traffic
		if err := r.ensureCiliumEncryptionPolicy(ctx, app, domain); err != nil {
			return ctrl.Result{}, err
		}

		// Phase 15.3: Domain health monitoring
		r.updateDomainHealthStatus(ctx, app, domain, nsName)

		log.Info("Reconciled domain namespace", "namespace", nsName, "domain", domain.Name)
	}

	if err := r.ensureCrossApplicationLinks(ctx, app); err != nil {
		return ctrl.Result{}, err
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

func (r *ChoApplicationReconciler) ensurePeriodicVulnerabilityScanning(ctx context.Context, app *choristerv1alpha1.ChoApplication, domain choristerv1alpha1.DomainSpec, nsName string) error {
	if app.Spec.Policy.Compliance != "standard" && app.Spec.Policy.Compliance != "regulated" {
		return nil
	}

	cronJobName := "vulnerability-scan"
	cronJob := &batchv1.CronJob{}
	err := r.Get(ctx, types.NamespacedName{Name: cronJobName, Namespace: nsName}, cronJob)
	if errors.IsNotFound(err) {
		cronJob = &batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cronJobName,
				Namespace: nsName,
				Labels: map[string]string{
					labelApplication: app.Name,
					labelDomain:      domain.Name,
				},
			},
			Spec: batchv1.CronJobSpec{
				Schedule: "0 3 * * *",
				JobTemplate: batchv1.JobTemplateSpec{
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								RestartPolicy: corev1.RestartPolicyNever,
								Containers: []corev1.Container{{
									Name:  "scanner",
									Image: "ghcr.io/aquasecurity/trivy:latest",
									Args:  []string{"image", "--quiet"},
								}},
							},
						},
					},
				},
			},
		}
		if err := r.Create(ctx, cronJob); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	computeList := &choristerv1alpha1.ChoComputeList{}
	if err := r.List(ctx, computeList, client.InNamespace(nsName)); err != nil {
		return err
	}
	images := uniqueImagesFromComputes(computeList.Items)
	result, err := r.getScanner().ScanImages(ctx, images)
	if err != nil {
		return err
	}

	reportName := fmt.Sprintf("%s-%s-vulnerability-report", app.Name, domain.Name)
	report := &choristerv1alpha1.ChoVulnerabilityReport{}
	reportKey := types.NamespacedName{Name: reportName, Namespace: app.Namespace}
	if err := r.Get(ctx, reportKey, report); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		report = &choristerv1alpha1.ChoVulnerabilityReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      reportName,
				Namespace: app.Namespace,
				Labels: map[string]string{
					labelApplication: app.Name,
					labelDomain:      domain.Name,
				},
			},
			Spec: choristerv1alpha1.ChoVulnerabilityReportSpec{
				Application: app.Name,
				Domain:      domain.Name,
				Trigger:     "periodic",
				TargetRef:   nsName,
				Images:      images,
			},
		}
		if err := r.Create(ctx, report); err != nil {
			return err
		}
		if err := r.Get(ctx, reportKey, report); err != nil {
			return err
		}
	} else {
		report.Spec.Application = app.Name
		report.Spec.Domain = domain.Name
		report.Spec.Trigger = "periodic"
		report.Spec.TargetRef = nsName
		report.Spec.Images = images
		if err := r.Update(ctx, report); err != nil {
			return err
		}
	}

	now := metav1.Now()
	report.Status.Scanner = result.Scanner
	report.Status.CriticalCount = result.CriticalCount
	report.Status.Findings = result.Findings
	report.Status.ScannedAt = &now
	if result.CriticalCount > 0 {
		report.Status.Phase = "FindingsPresent"
		setCondition(&report.Status.Conditions, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "CriticalFindings", Message: "Periodic scan found critical vulnerabilities"})
	} else {
		report.Status.Phase = "Clean"
		setCondition(&report.Status.Conditions, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "ScanComplete", Message: "Periodic scan completed without critical findings"})
	}
	return r.Status().Update(ctx, report)
}

func (r *ChoApplicationReconciler) ensureCrossApplicationLinks(ctx context.Context, app *choristerv1alpha1.ChoApplication) error {
	for _, link := range app.Spec.Links {
		for _, consumer := range link.Consumers {
			artifacts := compiler.CompileCrossApplicationLink(app, link, consumer)
			for _, obj := range []*unstructured.Unstructured{artifacts.HTTPRoute, artifacts.ReferenceGrant, artifacts.CiliumPolicy, artifacts.CiliumEnvoyConfig} {
				if err := ensureUnstructured(ctx, r.Client, obj); err != nil {
					return err
				}
			}

			existingPolicy := &networkingv1.NetworkPolicy{}
			key := types.NamespacedName{Name: artifacts.DirectDenyPolicy.Name, Namespace: artifacts.DirectDenyPolicy.Namespace}
			if err := r.Get(ctx, key, existingPolicy); err != nil {
				if errors.IsNotFound(err) {
					if err := r.Create(ctx, artifacts.DirectDenyPolicy); err != nil {
						return err
					}
					continue
				}
				return err
			}
			existingPolicy.Spec = artifacts.DirectDenyPolicy.Spec
			existingPolicy.Labels = artifacts.DirectDenyPolicy.Labels
			if err := r.Update(ctx, existingPolicy); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ChoApplicationReconciler) getScanner() scanning.Scanner {
	if r.Scanner != nil {
		return r.Scanner
	}
	return scanning.NewDefaultScanner()
}

func uniqueImagesFromComputes(computes []choristerv1alpha1.ChoCompute) []string {
	seen := map[string]struct{}{}
	images := make([]string, 0, len(computes))
	for _, compute := range computes {
		if compute.Spec.Image == "" {
			continue
		}
		if _, ok := seen[compute.Spec.Image]; ok {
			continue
		}
		seen[compute.Spec.Image] = struct{}{}
		images = append(images, compute.Spec.Image)
	}
	return images
}

// ensureNamespace creates or updates a domain namespace with the correct labels.
func (r *ChoApplicationReconciler) ensureNamespace(ctx context.Context, app *choristerv1alpha1.ChoApplication, domainName, nsName string) error {
	ns := &corev1.Namespace{}
	err := r.Get(ctx, types.NamespacedName{Name: nsName}, ns)

	psaLevel := psaLevelForCompliance(app.Spec.Policy.Compliance)

	if errors.IsNotFound(err) {
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
				Labels: map[string]string{
					labelApplication:                     app.Name,
					labelDomain:                          domainName,
					"pod-security.kubernetes.io/enforce": psaLevel,
					"pod-security.kubernetes.io/audit":   psaLevel,
					"pod-security.kubernetes.io/warn":    psaLevel,
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
	for k, v := range map[string]string{
		labelApplication:                     app.Name,
		labelDomain:                          domainName,
		"pod-security.kubernetes.io/enforce": psaLevel,
		"pod-security.kubernetes.io/audit":   psaLevel,
		"pod-security.kubernetes.io/warn":    psaLevel,
	} {
		if ns.Labels[k] != v {
			ns.Labels[k] = v
			needsUpdate = true
		}
	}
	if needsUpdate {
		return r.Update(ctx, ns)
	}
	return nil
}

// psaLevelForCompliance maps a compliance profile to a Pod Security Admission level.
func psaLevelForCompliance(compliance string) string {
	switch compliance {
	case "regulated":
		return "restricted"
	case "standard":
		return "restricted"
	case "essential":
		return "restricted"
	default:
		return "baseline"
	}
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

// ---------------------------------------------------------------------------
// Phase 15.1: Restricted domain L7 CiliumNetworkPolicy
// ---------------------------------------------------------------------------

func (r *ChoApplicationReconciler) ensureRestrictedDomainL7Policy(ctx context.Context, app *choristerv1alpha1.ChoApplication, domain choristerv1alpha1.DomainSpec) error {
	if domain.Sensitivity != "restricted" {
		return nil
	}
	policy := compiler.CompileRestrictedDomainL7Policy(app, domain)
	return ensureUnstructured(ctx, r.Client, policy)
}

// ---------------------------------------------------------------------------
// Phase 15.2: Tetragon TracingPolicy for runtime detection
// ---------------------------------------------------------------------------

func (r *ChoApplicationReconciler) ensureTetragonTracingPolicy(ctx context.Context, app *choristerv1alpha1.ChoApplication, domain choristerv1alpha1.DomainSpec) error {
	isRegulated := app.Spec.Policy.Compliance == "regulated"
	isRestricted := domain.Sensitivity == "restricted"
	if !isRegulated && !isRestricted {
		return nil
	}
	policy := compiler.CompileTetragonTracingPolicy(app, domain)
	return ensureUnstructured(ctx, r.Client, policy)
}

// ---------------------------------------------------------------------------
// Phase 15.3: Domain health monitoring
// ---------------------------------------------------------------------------

func (r *ChoApplicationReconciler) updateDomainHealthStatus(ctx context.Context, app *choristerv1alpha1.ChoApplication, domain choristerv1alpha1.DomainSpec, nsName string) {
	log := logf.FromContext(ctx)

	// Check if any pods are unhealthy in the domain namespace
	var podList corev1.PodList
	if err := r.List(ctx, &podList, client.InNamespace(nsName)); err != nil {
		log.Error(err, "Failed to list pods for health check", "namespace", nsName)
		return
	}

	healthy := true
	for i := range podList.Items {
		pod := &podList.Items[i]
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status != corev1.ConditionTrue {
				healthy = false
				break
			}
		}
	}

	status := metav1.ConditionTrue
	reason := "Healthy"
	message := fmt.Sprintf("Domain %s is healthy", domain.Name)
	if !healthy {
		status = metav1.ConditionFalse
		reason = "Degraded"
		message = fmt.Sprintf("Domain %s has unhealthy pods", domain.Name)
	}
	setCondition(&app.Status.Conditions, metav1.Condition{
		Type:               fmt.Sprintf("DomainHealthy-%s", domain.Name),
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: app.Generation,
	})
}

// IsDomainIsolated checks if the given domain has the isolation annotation.
func IsDomainIsolated(app *choristerv1alpha1.ChoApplication, domainName string) bool {
	if app.Annotations == nil {
		return false
	}
	return app.Annotations[fmt.Sprintf("chorister.dev/isolate-%s", domainName)] == "true"
}

// ---------------------------------------------------------------------------
// Phase 16.1: cert-manager Certificate per domain
// ---------------------------------------------------------------------------

func (r *ChoApplicationReconciler) ensureDomainCertificate(ctx context.Context, app *choristerv1alpha1.ChoApplication, domain choristerv1alpha1.DomainSpec) error {
	if domain.Sensitivity != "confidential" && domain.Sensitivity != "restricted" {
		return nil
	}
	cert := compiler.CompileCertManagerCertificate(app, domain)
	return ensureUnstructured(ctx, r.Client, cert)
}

// ---------------------------------------------------------------------------
// Phase 16.2: Cilium encryption for cross-domain traffic
// ---------------------------------------------------------------------------

func (r *ChoApplicationReconciler) ensureCiliumEncryptionPolicy(ctx context.Context, app *choristerv1alpha1.ChoApplication, domain choristerv1alpha1.DomainSpec) error {
	if domain.Sensitivity != "confidential" && domain.Sensitivity != "restricted" {
		return nil
	}
	// Only enforce if the domain has cross-domain traffic (consumes or supplies)
	if domain.Consumes == nil && domain.Supplies == nil {
		return nil
	}
	policy := compiler.CompileCiliumEncryptionPolicy(app, domain)
	return ensureUnstructured(ctx, r.Client, policy)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChoApplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choristerv1alpha1.ChoApplication{}).
		Named("choapplication").
		Complete(r)
}
