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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/scanning"
)

// ChoPromotionRequestReconciler reconciles a ChoPromotionRequest object
type ChoPromotionRequestReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Scanner scanning.Scanner
}

// +kubebuilder:rbac:groups=chorister.dev,resources=chopromotionrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chopromotionrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=chopromotionrequests/finalizers,verbs=update
// +kubebuilder:rbac:groups=chorister.dev,resources=choapplications,verbs=get;list;watch
// +kubebuilder:rbac:groups=chorister.dev,resources=chocomputes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chodatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=choqueues,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chocaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chovulnerabilityreports,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chovulnerabilityreports/status,verbs=get;update;patch

// Reconcile drives the promotion request through its lifecycle:
// Pending → Approved → Executing → Completed/Failed
func (r *ChoPromotionRequestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	pr := &choristerv1alpha1.ChoPromotionRequest{}
	if err := r.Get(ctx, req.NamespacedName, pr); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Terminal states — nothing to do
	if pr.Status.Phase == "Completed" || pr.Status.Phase == "Failed" || pr.Status.Phase == "Rejected" {
		return ctrl.Result{}, nil
	}

	// Initial state: set to Pending
	if pr.Status.Phase == "" {
		pr.Status.Phase = "Pending"
		setCondition(&pr.Status.Conditions, metav1.Condition{
			Type:               "Pending",
			Status:             metav1.ConditionTrue,
			Reason:             "AwaitingApproval",
			Message:            "Promotion request is awaiting required approvals",
			ObservedGeneration: pr.Generation,
		})
		if err := r.Status().Update(ctx, pr); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Created promotion request", "name", pr.Name, "phase", "Pending")
		return ctrl.Result{}, nil
	}

	// If Pending, check if approvals are sufficient
	if pr.Status.Phase == "Pending" {
		// Look up the ChoApplication to get promotion policy
		app := &choristerv1alpha1.ChoApplication{}
		if err := r.Get(ctx, types.NamespacedName{Name: pr.Spec.Application, Namespace: pr.Namespace}, app); err != nil {
			if errors.IsNotFound(err) {
				pr.Status.Phase = "Failed"
				setCondition(&pr.Status.Conditions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					Reason:             "ApplicationNotFound",
					Message:            fmt.Sprintf("Application %q not found", pr.Spec.Application),
					ObservedGeneration: pr.Generation,
				})
				return ctrl.Result{}, r.Status().Update(ctx, pr)
			}
			return ctrl.Result{}, err
		}

		required := app.Spec.Policy.Promotion.RequiredApprovers
		allowedRoles := app.Spec.Policy.Promotion.AllowedRoles

		// Count only approvals from allowed roles
		validApprovals := 0
		for _, approval := range pr.Status.Approvals {
			if isRoleAllowed(approval.Role, allowedRoles) {
				validApprovals++
			}
		}

		if validApprovals >= required {
			if securityScanRequired(app) {
				blocked, err := r.runPromotionSecurityScan(ctx, pr)
				if err != nil {
					return ctrl.Result{}, err
				}
				if blocked {
					return ctrl.Result{}, nil
				}
			}

			pr.Status.Phase = "Approved"
			setCondition(&pr.Status.Conditions, metav1.Condition{
				Type:               "Approved",
				Status:             metav1.ConditionTrue,
				Reason:             "ApprovalsComplete",
				Message:            fmt.Sprintf("%d of %d required approvals received", validApprovals, required),
				ObservedGeneration: pr.Generation,
			})
			if err := r.Status().Update(ctx, pr); err != nil {
				return ctrl.Result{}, err
			}
			log.Info("Promotion request approved", "name", pr.Name, "approvals", validApprovals)
			// Re-fetch and continue to execution
			if err := r.Get(ctx, req.NamespacedName, pr); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			return ctrl.Result{}, nil
		}
	}

	// If Approved, execute the promotion
	if pr.Status.Phase == "Approved" {
		pr.Status.Phase = "Executing"
		setCondition(&pr.Status.Conditions, metav1.Condition{
			Type:               "Executing",
			Status:             metav1.ConditionTrue,
			Reason:             "CopyingManifests",
			Message:            "Copying sandbox resources to production namespace",
			ObservedGeneration: pr.Generation,
		})
		if err := r.Status().Update(ctx, pr); err != nil {
			return ctrl.Result{}, err
		}

		// Re-fetch after status update
		if err := r.Get(ctx, req.NamespacedName, pr); err != nil {
			return ctrl.Result{}, err
		}

		// Execute: copy resources from sandbox to production
		sandboxNs := SandboxNamespace(pr.Spec.Application, pr.Spec.Domain, pr.Spec.Sandbox)
		prodNs := fmt.Sprintf("%s-%s", pr.Spec.Application, pr.Spec.Domain)

		if err := r.copyResources(ctx, sandboxNs, prodNs, pr.Spec.Application, pr.Spec.Domain); err != nil {
			pr.Status.Phase = "Failed"
			setCondition(&pr.Status.Conditions, metav1.Condition{
				Type:               "Failed",
				Status:             metav1.ConditionTrue,
				Reason:             "ExecutionFailed",
				Message:            fmt.Sprintf("Failed to copy resources: %v", err),
				ObservedGeneration: pr.Generation,
			})
			return ctrl.Result{}, r.Status().Update(ctx, pr)
		}

		pr.Status.Phase = "Completed"
		pr.Status.CompiledWithRevision = pr.Spec.CompiledWithRevision
		setCondition(&pr.Status.Conditions, metav1.Condition{
			Type:               "Completed",
			Status:             metav1.ConditionTrue,
			Reason:             "PromotionComplete",
			Message:            "Resources copied to production namespace",
			ObservedGeneration: pr.Generation,
		})
		if err := r.Status().Update(ctx, pr); err != nil {
			return ctrl.Result{}, err
		}

		log.Info("Promotion completed", "name", pr.Name, "sandbox", sandboxNs, "production", prodNs)
	}

	return ctrl.Result{}, nil
}

// copyResources copies chorister CRDs from sandbox namespace to production namespace.
func (r *ChoPromotionRequestReconciler) copyResources(ctx context.Context, sandboxNs, prodNs, app, domain string) error {
	// Copy ChoCompute resources
	computeList := &choristerv1alpha1.ChoComputeList{}
	if err := r.List(ctx, computeList, client.InNamespace(sandboxNs)); err != nil {
		return fmt.Errorf("listing sandbox ChoComputes: %w", err)
	}
	for i := range computeList.Items {
		src := &computeList.Items[i]
		if err := r.ensureResourceInNamespace(ctx, prodNs, src.Name, "ChoCompute",
			func() client.Object {
				return &choristerv1alpha1.ChoCompute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      src.Name,
						Namespace: prodNs,
						Labels:    src.Labels,
					},
					Spec: src.Spec,
				}
			},
			func(existing client.Object) {
				e := existing.(*choristerv1alpha1.ChoCompute)
				e.Spec = src.Spec
			},
		); err != nil {
			return err
		}
	}

	// Copy ChoDatabase resources
	dbList := &choristerv1alpha1.ChoDatabaseList{}
	if err := r.List(ctx, dbList, client.InNamespace(sandboxNs)); err != nil {
		return fmt.Errorf("listing sandbox ChoDatabases: %w", err)
	}
	for i := range dbList.Items {
		src := &dbList.Items[i]
		if err := r.ensureResourceInNamespace(ctx, prodNs, src.Name, "ChoDatabase",
			func() client.Object {
				return &choristerv1alpha1.ChoDatabase{
					ObjectMeta: metav1.ObjectMeta{
						Name:      src.Name,
						Namespace: prodNs,
						Labels:    src.Labels,
					},
					Spec: src.Spec,
				}
			},
			func(existing client.Object) {
				e := existing.(*choristerv1alpha1.ChoDatabase)
				e.Spec = src.Spec
			},
		); err != nil {
			return err
		}
	}

	// Copy ChoQueue resources
	queueList := &choristerv1alpha1.ChoQueueList{}
	if err := r.List(ctx, queueList, client.InNamespace(sandboxNs)); err != nil {
		return fmt.Errorf("listing sandbox ChoQueues: %w", err)
	}
	for i := range queueList.Items {
		src := &queueList.Items[i]
		if err := r.ensureResourceInNamespace(ctx, prodNs, src.Name, "ChoQueue",
			func() client.Object {
				return &choristerv1alpha1.ChoQueue{
					ObjectMeta: metav1.ObjectMeta{
						Name:      src.Name,
						Namespace: prodNs,
						Labels:    src.Labels,
					},
					Spec: src.Spec,
				}
			},
			func(existing client.Object) {
				e := existing.(*choristerv1alpha1.ChoQueue)
				e.Spec = src.Spec
			},
		); err != nil {
			return err
		}
	}

	// Copy ChoCache resources
	cacheList := &choristerv1alpha1.ChoCacheList{}
	if err := r.List(ctx, cacheList, client.InNamespace(sandboxNs)); err != nil {
		return fmt.Errorf("listing sandbox ChoCaches: %w", err)
	}
	for i := range cacheList.Items {
		src := &cacheList.Items[i]
		if err := r.ensureResourceInNamespace(ctx, prodNs, src.Name, "ChoCache",
			func() client.Object {
				return &choristerv1alpha1.ChoCache{
					ObjectMeta: metav1.ObjectMeta{
						Name:      src.Name,
						Namespace: prodNs,
						Labels:    src.Labels,
					},
					Spec: src.Spec,
				}
			},
			func(existing client.Object) {
				e := existing.(*choristerv1alpha1.ChoCache)
				e.Spec = src.Spec
			},
		); err != nil {
			return err
		}
	}

	return nil
}

// ensureResourceInNamespace creates or updates a resource in the target namespace.
func (r *ChoPromotionRequestReconciler) ensureResourceInNamespace(
	ctx context.Context,
	ns, name, kind string,
	newFn func() client.Object,
	updateFn func(client.Object),
) error {
	// Use unstructured to check existence first
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(newFn().GetObjectKind().GroupVersionKind())

	switch kind {
	case "ChoCompute":
		obj := &choristerv1alpha1.ChoCompute{}
		err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, obj)
		if errors.IsNotFound(err) {
			return r.Create(ctx, newFn())
		}
		if err != nil {
			return err
		}
		updateFn(obj)
		return r.Update(ctx, obj)
	case "ChoDatabase":
		obj := &choristerv1alpha1.ChoDatabase{}
		err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, obj)
		if errors.IsNotFound(err) {
			return r.Create(ctx, newFn())
		}
		if err != nil {
			return err
		}
		updateFn(obj)
		return r.Update(ctx, obj)
	case "ChoQueue":
		obj := &choristerv1alpha1.ChoQueue{}
		err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, obj)
		if errors.IsNotFound(err) {
			return r.Create(ctx, newFn())
		}
		if err != nil {
			return err
		}
		updateFn(obj)
		return r.Update(ctx, obj)
	case "ChoCache":
		obj := &choristerv1alpha1.ChoCache{}
		err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, obj)
		if errors.IsNotFound(err) {
			return r.Create(ctx, newFn())
		}
		if err != nil {
			return err
		}
		updateFn(obj)
		return r.Update(ctx, obj)
	}

	return fmt.Errorf("unsupported resource kind: %s", kind)
}

func isRoleAllowed(role string, allowed []string) bool {
	for _, r := range allowed {
		if r == role {
			return true
		}
	}
	return false
}

func (r *ChoPromotionRequestReconciler) getScanner() scanning.Scanner {
	if r.Scanner != nil {
		return r.Scanner
	}
	return scanning.NewDefaultScanner()
}

func securityScanRequired(app *choristerv1alpha1.ChoApplication) bool {
	if app.Spec.Policy.Promotion.RequireSecurityScan {
		return true
	}
	return app.Spec.Policy.Compliance == "standard" || app.Spec.Policy.Compliance == "regulated"
}

func (r *ChoPromotionRequestReconciler) runPromotionSecurityScan(ctx context.Context, pr *choristerv1alpha1.ChoPromotionRequest) (bool, error) {
	sandboxNs := SandboxNamespace(pr.Spec.Application, pr.Spec.Domain, pr.Spec.Sandbox)
	computeList := &choristerv1alpha1.ChoComputeList{}
	if err := r.List(ctx, computeList, client.InNamespace(sandboxNs)); err != nil {
		return false, fmt.Errorf("list sandbox images for scan: %w", err)
	}

	images := uniqueImagesFromComputes(computeList.Items)
	result, err := r.getScanner().ScanImages(ctx, images)
	if err != nil {
		return false, fmt.Errorf("scan images: %w", err)
	}

	report := &choristerv1alpha1.ChoVulnerabilityReport{}
	key := types.NamespacedName{Name: pr.Name + "-scan", Namespace: pr.Namespace}
	if err := r.Get(ctx, key, report); err != nil {
		if !errors.IsNotFound(err) {
			return false, err
		}
		report = &choristerv1alpha1.ChoVulnerabilityReport{
			ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace},
			Spec: choristerv1alpha1.ChoVulnerabilityReportSpec{
				Application: pr.Spec.Application,
				Domain:      pr.Spec.Domain,
				Trigger:     "promotion",
				TargetRef:   pr.Name,
				Images:      images,
			},
		}
		if err := r.Create(ctx, report); err != nil {
			return false, err
		}
		if err := r.Get(ctx, key, report); err != nil {
			return false, err
		}
	} else {
		report.Spec.Images = images
		report.Spec.Application = pr.Spec.Application
		report.Spec.Domain = pr.Spec.Domain
		report.Spec.Trigger = "promotion"
		report.Spec.TargetRef = pr.Name
		if err := r.Update(ctx, report); err != nil {
			return false, err
		}
	}

	now := metav1.Now()
	report.Status.Scanner = result.Scanner
	report.Status.CriticalCount = result.CriticalCount
	report.Status.Findings = result.Findings
	report.Status.ScannedAt = &now
	if result.CriticalCount > 0 {
		report.Status.Phase = "Blocked"
		setCondition(&report.Status.Conditions, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "CriticalFindings", Message: "Promotion blocked by critical image vulnerabilities"})
		if err := r.Status().Update(ctx, report); err != nil {
			return false, err
		}

		pr.Status.Phase = "Rejected"
		setCondition(&pr.Status.Conditions, metav1.Condition{
			Type:               "Rejected",
			Status:             metav1.ConditionTrue,
			Reason:             "ImageScanFailed",
			Message:            "Promotion blocked by critical image vulnerabilities",
			ObservedGeneration: pr.Generation,
		})
		if err := r.Status().Update(ctx, pr); err != nil {
			return false, err
		}
		return true, nil
	}

	report.Status.Phase = "Passed"
	setCondition(&report.Status.Conditions, metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "ScanPassed", Message: "Promotion scan completed without critical findings"})
	return false, r.Status().Update(ctx, report)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChoPromotionRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choristerv1alpha1.ChoPromotionRequest{}).
		Named("chopromotionrequest").
		Complete(r)
}
