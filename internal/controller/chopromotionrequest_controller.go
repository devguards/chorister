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
	"slices"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/multicluster"
	"github.com/chorister-dev/chorister/internal/scanning"
	"github.com/chorister-dev/chorister/internal/validation"
)

// ChoPromotionRequestReconciler reconciles a ChoPromotionRequest object
type ChoPromotionRequestReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Scanner        scanning.Scanner
	ClusterClients multicluster.ClientFactory
}

// +kubebuilder:rbac:groups=chorister.dev,resources=chopromotionrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chopromotionrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=chopromotionrequests/finalizers,verbs=update
// +kubebuilder:rbac:groups=chorister.dev,resources=choapplications,verbs=get;list;watch
// +kubebuilder:rbac:groups=chorister.dev,resources=chocomputes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chodatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=choqueues,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chocaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chonetworks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chostorages,verbs=get;list;watch;create;update;patch;delete
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
		// Check domain isolation before executing
		app := &choristerv1alpha1.ChoApplication{}
		if err := r.Get(ctx, types.NamespacedName{Name: pr.Spec.Application, Namespace: pr.Namespace}, app); err != nil {
			return ctrl.Result{}, err
		}
		if IsDomainIsolated(app, pr.Spec.Domain) {
			pr.Status.Phase = "Failed"
			setCondition(&pr.Status.Conditions, metav1.Condition{
				Type:               "Failed",
				Status:             metav1.ConditionTrue,
				Reason:             "DomainIsolated",
				Message:            fmt.Sprintf("Domain %q is isolated; promotion blocked", pr.Spec.Domain),
				ObservedGeneration: pr.Generation,
			})
			log.Info("Promotion blocked: domain is isolated", "name", pr.Name, "domain", pr.Spec.Domain)
			return ctrl.Result{}, r.Status().Update(ctx, pr)
		}

		// Check for archived dependencies in the production namespace
		prodNs := fmt.Sprintf("%s-%s", pr.Spec.Application, pr.Spec.Domain)
		if archivedErrs := r.checkArchivedDependencies(ctx, prodNs); len(archivedErrs) > 0 {
			pr.Status.Phase = "Failed"
			setCondition(&pr.Status.Conditions, metav1.Condition{
				Type:               "Failed",
				Status:             metav1.ConditionTrue,
				Reason:             "ArchivedDependency",
				Message:            fmt.Sprintf("Promotion blocked: %s", archivedErrs[0]),
				ObservedGeneration: pr.Generation,
			})
			log.Info("Promotion blocked: archived dependencies found", "name", pr.Name, "errors", archivedErrs)
			return ctrl.Result{}, r.Status().Update(ctx, pr)
		}

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
	}

	// If Executing, copy resources (handles both fresh transition and retry after conflict/crash)
	if pr.Status.Phase == "Executing" {
		sandboxNs := SandboxNamespace(pr.Spec.Application, pr.Spec.Domain, pr.Spec.Sandbox)
		prodNs := fmt.Sprintf("%s-%s", pr.Spec.Application, pr.Spec.Domain)

		// Resolve the production cluster client.
		prodClient, targetClusterName := r.resolveProductionClient(ctx)

		if err := r.copyResources(ctx, sandboxNs, prodNs, pr.Spec.Application, pr.Spec.Domain, prodClient); err != nil {
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

		// Archive stateful resources that exist in production but not in sandbox
		if err := r.archiveOrphanedStatefulResources(ctx, sandboxNs, prodNs, pr); err != nil {
			pr.Status.Phase = "Failed"
			setCondition(&pr.Status.Conditions, metav1.Condition{
				Type:               "Failed",
				Status:             metav1.ConditionTrue,
				Reason:             "ArchiveFailed",
				Message:            fmt.Sprintf("Failed to archive orphaned resources: %v", err),
				ObservedGeneration: pr.Generation,
			})
			return ctrl.Result{}, r.Status().Update(ctx, pr)
		}

		pr.Status.Phase = "Completed"
		pr.Status.CompiledWithRevision = pr.Spec.CompiledWithRevision
		pr.Status.TargetCluster = targetClusterName
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

		log.Info("Promotion completed", "name", pr.Name, "sandbox", sandboxNs, "production", prodNs, "targetCluster", targetClusterName)
	}

	return ctrl.Result{}, nil
}

// copyResources copies chorister CRDs from sandbox namespace to production namespace.
// The prodClient is used to write to the production cluster (may be remote or local).
func (r *ChoPromotionRequestReconciler) copyResources(ctx context.Context, sandboxNs, prodNs, app, domain string, prodClient client.Client) error {
	// Copy ChoCompute resources
	computeList := &choristerv1alpha1.ChoComputeList{}
	if err := r.List(ctx, computeList, client.InNamespace(sandboxNs)); err != nil {
		return fmt.Errorf("listing sandbox ChoComputes: %w", err)
	}
	for i := range computeList.Items {
		src := &computeList.Items[i]
		if err := r.ensureResourceInNamespace(ctx, prodNs, src.Name, "ChoCompute", prodClient,
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
		if err := r.ensureResourceInNamespace(ctx, prodNs, src.Name, "ChoDatabase", prodClient,
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
		if err := r.ensureResourceInNamespace(ctx, prodNs, src.Name, "ChoQueue", prodClient,
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
		if err := r.ensureResourceInNamespace(ctx, prodNs, src.Name, "ChoCache", prodClient,
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

	// Copy ChoNetwork resources
	networkList := &choristerv1alpha1.ChoNetworkList{}
	if err := r.List(ctx, networkList, client.InNamespace(sandboxNs)); err != nil {
		return fmt.Errorf("listing sandbox ChoNetworks: %w", err)
	}
	for i := range networkList.Items {
		src := &networkList.Items[i]
		if err := r.ensureResourceInNamespace(ctx, prodNs, src.Name, "ChoNetwork", prodClient,
			func() client.Object {
				return &choristerv1alpha1.ChoNetwork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      src.Name,
						Namespace: prodNs,
						Labels:    src.Labels,
					},
					Spec: src.Spec,
				}
			},
			func(existing client.Object) {
				e := existing.(*choristerv1alpha1.ChoNetwork)
				e.Spec = src.Spec
			},
		); err != nil {
			return err
		}
	}

	// Copy ChoStorage resources
	storageList := &choristerv1alpha1.ChoStorageList{}
	if err := r.List(ctx, storageList, client.InNamespace(sandboxNs)); err != nil {
		return fmt.Errorf("listing sandbox ChoStorages: %w", err)
	}
	for i := range storageList.Items {
		src := &storageList.Items[i]
		if err := r.ensureResourceInNamespace(ctx, prodNs, src.Name, "ChoStorage", prodClient,
			func() client.Object {
				return &choristerv1alpha1.ChoStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      src.Name,
						Namespace: prodNs,
						Labels:    src.Labels,
					},
					Spec: src.Spec,
				}
			},
			func(existing client.Object) {
				e := existing.(*choristerv1alpha1.ChoStorage)
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
	targetClient client.Client,
	newFn func() client.Object,
	updateFn func(client.Object),
) error {
	// Use unstructured to check existence first
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(newFn().GetObjectKind().GroupVersionKind())

	switch kind {
	case "ChoCompute":
		obj := &choristerv1alpha1.ChoCompute{}
		err := targetClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, obj)
		if errors.IsNotFound(err) {
			return targetClient.Create(ctx, newFn())
		}
		if err != nil {
			return err
		}
		updateFn(obj)
		return targetClient.Update(ctx, obj)
	case "ChoDatabase":
		obj := &choristerv1alpha1.ChoDatabase{}
		err := targetClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, obj)
		if errors.IsNotFound(err) {
			return targetClient.Create(ctx, newFn())
		}
		if err != nil {
			return err
		}
		updateFn(obj)
		return targetClient.Update(ctx, obj)
	case "ChoQueue":
		obj := &choristerv1alpha1.ChoQueue{}
		err := targetClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, obj)
		if errors.IsNotFound(err) {
			return targetClient.Create(ctx, newFn())
		}
		if err != nil {
			return err
		}
		updateFn(obj)
		return targetClient.Update(ctx, obj)
	case "ChoCache":
		obj := &choristerv1alpha1.ChoCache{}
		err := targetClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, obj)
		if errors.IsNotFound(err) {
			return targetClient.Create(ctx, newFn())
		}
		if err != nil {
			return err
		}
		updateFn(obj)
		return targetClient.Update(ctx, obj)
	case "ChoNetwork":
		obj := &choristerv1alpha1.ChoNetwork{}
		err := targetClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, obj)
		if errors.IsNotFound(err) {
			return targetClient.Create(ctx, newFn())
		}
		if err != nil {
			return err
		}
		updateFn(obj)
		return targetClient.Update(ctx, obj)
	case "ChoStorage":
		obj := &choristerv1alpha1.ChoStorage{}
		err := targetClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, obj)
		if errors.IsNotFound(err) {
			return targetClient.Create(ctx, newFn())
		}
		if err != nil {
			return err
		}
		updateFn(obj)
		return targetClient.Update(ctx, obj)
	}

	return fmt.Errorf("unsupported resource kind: %s", kind)
}

func isRoleAllowed(role string, allowed []string) bool {
	return slices.Contains(allowed, role)
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

// archiveOrphanedStatefulResources transitions stateful resources to Archived
// when they exist in production but not in sandbox.
func (r *ChoPromotionRequestReconciler) archiveOrphanedStatefulResources(
	ctx context.Context, sandboxNs, prodNs string, pr *choristerv1alpha1.ChoPromotionRequest,
) error {
	log := logf.FromContext(ctx)
	retention := r.getArchiveRetention(ctx, pr)

	// Archive orphaned ChoDatabase resources
	if err := r.archiveOrphanedDatabases(ctx, sandboxNs, prodNs, retention, log); err != nil {
		return err
	}

	// Archive orphaned ChoQueue resources
	if err := r.archiveOrphanedQueues(ctx, sandboxNs, prodNs, retention, log); err != nil {
		return err
	}

	// Archive orphaned ChoStorage resources
	if err := r.archiveOrphanedStorages(ctx, sandboxNs, prodNs, retention, log); err != nil {
		return err
	}

	// Archive orphaned ChoCache resources
	if err := r.archiveOrphanedCaches(ctx, sandboxNs, prodNs, retention, log); err != nil {
		return err
	}

	return nil
}

func (r *ChoPromotionRequestReconciler) archiveOrphanedDatabases(
	ctx context.Context, sandboxNs, prodNs string, retention time.Duration, log logr.Logger,
) error {
	sandboxDBs := &choristerv1alpha1.ChoDatabaseList{}
	if err := r.List(ctx, sandboxDBs, client.InNamespace(sandboxNs)); err != nil {
		return fmt.Errorf("listing sandbox ChoDatabases: %w", err)
	}
	sandboxNames := make(map[string]bool, len(sandboxDBs.Items))
	for _, db := range sandboxDBs.Items {
		sandboxNames[db.Name] = true
	}

	prodDBs := &choristerv1alpha1.ChoDatabaseList{}
	if err := r.List(ctx, prodDBs, client.InNamespace(prodNs)); err != nil {
		return fmt.Errorf("listing production ChoDatabases: %w", err)
	}

	for i := range prodDBs.Items {
		db := &prodDBs.Items[i]
		if sandboxNames[db.Name] || db.Status.Lifecycle == "Archived" || db.Status.Lifecycle == "Deletable" {
			continue
		}
		now := metav1.Now()
		deletableAfter := metav1.NewTime(now.Add(retention))
		db.Status.Lifecycle = "Archived"
		db.Status.ArchivedAt = &now
		db.Status.DeletableAfter = &deletableAfter
		db.Status.Ready = false
		setCondition(&db.Status.Conditions, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "Archived",
			Message: "Resource archived during promotion (removed from sandbox DSL)",
		})
		if err := r.Status().Update(ctx, db); err != nil {
			return fmt.Errorf("archiving ChoDatabase %s: %w", db.Name, err)
		}
		log.Info("Archived orphaned ChoDatabase", "name", db.Name, "namespace", prodNs)
	}
	return nil
}

func (r *ChoPromotionRequestReconciler) archiveOrphanedQueues(
	ctx context.Context, sandboxNs, prodNs string, retention time.Duration, log logr.Logger,
) error {
	sandboxQueues := &choristerv1alpha1.ChoQueueList{}
	if err := r.List(ctx, sandboxQueues, client.InNamespace(sandboxNs)); err != nil {
		return fmt.Errorf("listing sandbox ChoQueues: %w", err)
	}
	sandboxNames := make(map[string]bool, len(sandboxQueues.Items))
	for _, q := range sandboxQueues.Items {
		sandboxNames[q.Name] = true
	}

	prodQueues := &choristerv1alpha1.ChoQueueList{}
	if err := r.List(ctx, prodQueues, client.InNamespace(prodNs)); err != nil {
		return fmt.Errorf("listing production ChoQueues: %w", err)
	}

	for i := range prodQueues.Items {
		q := &prodQueues.Items[i]
		if sandboxNames[q.Name] || q.Status.Lifecycle == "Archived" || q.Status.Lifecycle == "Deletable" {
			continue
		}
		now := metav1.Now()
		deletableAfter := metav1.NewTime(now.Add(retention))
		q.Status.Lifecycle = "Archived"
		q.Status.ArchivedAt = &now
		q.Status.DeletableAfter = &deletableAfter
		q.Status.Ready = false
		setCondition(&q.Status.Conditions, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "Archived",
			Message: "Resource archived during promotion (removed from sandbox DSL)",
		})
		if err := r.Status().Update(ctx, q); err != nil {
			return fmt.Errorf("archiving ChoQueue %s: %w", q.Name, err)
		}
		log.Info("Archived orphaned ChoQueue", "name", q.Name, "namespace", prodNs)
	}
	return nil
}

func (r *ChoPromotionRequestReconciler) archiveOrphanedStorages(
	ctx context.Context, sandboxNs, prodNs string, retention time.Duration, log logr.Logger,
) error {
	sandboxStorages := &choristerv1alpha1.ChoStorageList{}
	if err := r.List(ctx, sandboxStorages, client.InNamespace(sandboxNs)); err != nil {
		return fmt.Errorf("listing sandbox ChoStorages: %w", err)
	}
	sandboxNames := make(map[string]bool, len(sandboxStorages.Items))
	for _, s := range sandboxStorages.Items {
		sandboxNames[s.Name] = true
	}

	prodStorages := &choristerv1alpha1.ChoStorageList{}
	if err := r.List(ctx, prodStorages, client.InNamespace(prodNs)); err != nil {
		return fmt.Errorf("listing production ChoStorages: %w", err)
	}

	for i := range prodStorages.Items {
		s := &prodStorages.Items[i]
		if sandboxNames[s.Name] || s.Status.Lifecycle == "Archived" || s.Status.Lifecycle == "Deletable" {
			continue
		}
		now := metav1.Now()
		deletableAfter := metav1.NewTime(now.Add(retention))
		s.Status.Lifecycle = "Archived"
		s.Status.ArchivedAt = &now
		s.Status.DeletableAfter = &deletableAfter
		s.Status.Ready = false
		setCondition(&s.Status.Conditions, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "Archived",
			Message: "Resource archived during promotion (removed from sandbox DSL)",
		})
		if err := r.Status().Update(ctx, s); err != nil {
			return fmt.Errorf("archiving ChoStorage %s: %w", s.Name, err)
		}
		log.Info("Archived orphaned ChoStorage", "name", s.Name, "namespace", prodNs)
	}
	return nil
}

func (r *ChoPromotionRequestReconciler) archiveOrphanedCaches(
	ctx context.Context, sandboxNs, prodNs string, retention time.Duration, log logr.Logger,
) error {
	sandboxCaches := &choristerv1alpha1.ChoCacheList{}
	if err := r.List(ctx, sandboxCaches, client.InNamespace(sandboxNs)); err != nil {
		return fmt.Errorf("listing sandbox ChoCaches: %w", err)
	}
	sandboxNames := make(map[string]bool, len(sandboxCaches.Items))
	for _, c := range sandboxCaches.Items {
		sandboxNames[c.Name] = true
	}

	prodCaches := &choristerv1alpha1.ChoCacheList{}
	if err := r.List(ctx, prodCaches, client.InNamespace(prodNs)); err != nil {
		return fmt.Errorf("listing production ChoCaches: %w", err)
	}

	for i := range prodCaches.Items {
		c := &prodCaches.Items[i]
		if sandboxNames[c.Name] || c.Status.Lifecycle == "Archived" || c.Status.Lifecycle == "Deletable" {
			continue
		}
		now := metav1.Now()
		deletableAfter := metav1.NewTime(now.Add(retention))
		c.Status.Lifecycle = "Archived"
		c.Status.ArchivedAt = &now
		c.Status.DeletableAfter = &deletableAfter
		c.Status.Ready = false
		setCondition(&c.Status.Conditions, metav1.Condition{
			Type:    "Ready",
			Status:  metav1.ConditionFalse,
			Reason:  "Archived",
			Message: "Resource archived during promotion (removed from sandbox DSL)",
		})
		if err := r.Status().Update(ctx, c); err != nil {
			return fmt.Errorf("archiving ChoCache %s: %w", c.Name, err)
		}
		log.Info("Archived orphaned ChoCache", "name", c.Name, "namespace", prodNs)
	}
	return nil
}

// checkArchivedDependencies lists stateful resources in the production namespace
// and returns validation errors if any are in the Archived lifecycle state.
func (r *ChoPromotionRequestReconciler) checkArchivedDependencies(ctx context.Context, prodNs string) []string {
	databases := &choristerv1alpha1.ChoDatabaseList{}
	if err := r.List(ctx, databases, client.InNamespace(prodNs)); err != nil {
		return []string{fmt.Sprintf("listing production ChoDatabases: %v", err)}
	}

	queues := &choristerv1alpha1.ChoQueueList{}
	if err := r.List(ctx, queues, client.InNamespace(prodNs)); err != nil {
		return []string{fmt.Sprintf("listing production ChoQueues: %v", err)}
	}

	storages := &choristerv1alpha1.ChoStorageList{}
	if err := r.List(ctx, storages, client.InNamespace(prodNs)); err != nil {
		return []string{fmt.Sprintf("listing production ChoStorages: %v", err)}
	}

	return validation.ValidateArchivedResourceDependencies(databases.Items, queues.Items, storages.Items)
}

// getArchiveRetention returns the archive retention duration from the application policy.
func (r *ChoPromotionRequestReconciler) getArchiveRetention(ctx context.Context, pr *choristerv1alpha1.ChoPromotionRequest) time.Duration {
	defaultRetention := 30 * 24 * time.Hour

	app := &choristerv1alpha1.ChoApplication{}
	if err := r.Get(ctx, types.NamespacedName{Name: pr.Spec.Application, Namespace: pr.Namespace}, app); err != nil {
		return defaultRetention
	}

	if app.Spec.Policy.ArchiveRetention == "" {
		return defaultRetention
	}

	duration, err := validation.ParseRetentionDuration(app.Spec.Policy.ArchiveRetention)
	if err != nil {
		return defaultRetention
	}
	if duration < defaultRetention {
		logf.FromContext(ctx).Info("ArchiveRetention below 30-day minimum; clamping to default",
			"configured", app.Spec.Policy.ArchiveRetention, "applied", defaultRetention)
		return defaultRetention
	}
	return duration
}

// resolveProductionClient returns the client for production workloads and the cluster name.
// If a production-role cluster is registered, it returns that remote client.
// Otherwise it falls back to the local (home) client with an empty cluster name.
func (r *ChoPromotionRequestReconciler) resolveProductionClient(ctx context.Context) (client.Client, string) {
	if r.ClusterClients == nil {
		return r.Client, ""
	}
	c, err := r.ClusterClients.ClientForRole(ctx, multicluster.ClusterRoleProduction)
	if err != nil {
		return r.Client, ""
	}
	// If the returned client is the same as local, we're in single-cluster mode.
	if c == r.ClusterClients.Local() {
		return r.Client, ""
	}
	// Find the cluster name for the production role.
	name := r.resolveClusterNameForRole(ctx, multicluster.ClusterRoleProduction)
	return c, name
}

// resolveClusterNameForRole looks up the ChoCluster to find the first cluster name with the given role.
func (r *ChoPromotionRequestReconciler) resolveClusterNameForRole(ctx context.Context, role multicluster.ClusterRole) string {
	clusterList := &choristerv1alpha1.ChoClusterList{}
	if err := r.List(ctx, clusterList); err != nil || len(clusterList.Items) == 0 {
		return ""
	}
	for _, entry := range clusterList.Items[0].Spec.Clusters {
		if entry.Role == string(role) {
			return entry.Name
		}
	}
	return ""
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChoPromotionRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choristerv1alpha1.ChoPromotionRequest{}).
		Named("chopromotionrequest").
		Complete(r)
}
