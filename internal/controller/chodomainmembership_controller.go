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
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

const (
	labelMembership = "chorister.dev/membership"
	// oidcSyncRequeueInterval is how often OIDC-sourced memberships are re-checked.
	oidcSyncRequeueInterval = 5 * time.Minute
)

// OIDCGroupChecker verifies whether an identity belongs to an OIDC group.
type OIDCGroupChecker interface {
	IsMember(ctx context.Context, group, identity string) (bool, error)
}

// ChoDomainMembershipReconciler reconciles a ChoDomainMembership object
type ChoDomainMembershipReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	OIDCGroupChecker OIDCGroupChecker
}

// +kubebuilder:rbac:groups=chorister.dev,resources=chodomainmemberships,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chodomainmemberships/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=chodomainmemberships/finalizers,verbs=update
// +kubebuilder:rbac:groups=chorister.dev,resources=choapplications,verbs=get;list;watch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete

// Reconcile creates or removes RoleBindings based on membership spec and expiry.
func (r *ChoDomainMembershipReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	membership := &choristerv1alpha1.ChoDomainMembership{}
	if err := r.Get(ctx, req.NamespacedName, membership); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check membership expiry
	if membership.Spec.ExpiresAt != nil && !membership.Spec.ExpiresAt.Time.IsZero() {
		if time.Now().After(membership.Spec.ExpiresAt.Time) {
			return r.handleExpiredMembership(ctx, membership)
		}
	}

	// OIDC group sync: verify identity still belongs to the group
	if membership.Spec.Source == "oidc-group" && membership.Spec.OIDCGroup != "" && r.OIDCGroupChecker != nil {
		isMember, err := r.OIDCGroupChecker.IsMember(ctx, membership.Spec.OIDCGroup, membership.Spec.Identity)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("checking OIDC group membership: %w", err)
		}
		if !isMember {
			return r.handleDeprovisionedMembership(ctx, membership)
		}
	}

	// Look up the ChoApplication to find domain sensitivity and validate restricted domains
	app := &choristerv1alpha1.ChoApplication{}
	appKey := types.NamespacedName{Name: membership.Spec.Application, Namespace: membership.Namespace}
	if err := r.Get(ctx, appKey, app); err != nil {
		if errors.IsNotFound(err) {
			setCondition(&membership.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "ApplicationNotFound",
				Message:            fmt.Sprintf("Application %q not found", membership.Spec.Application),
				ObservedGeneration: membership.Generation,
			})
			return ctrl.Result{}, r.Status().Update(ctx, membership)
		}
		return ctrl.Result{}, err
	}

	// Find domain and check sensitivity
	var domainSensitivity string
	for _, d := range app.Spec.Domains {
		if d.Name == membership.Spec.Domain {
			domainSensitivity = d.Sensitivity
			break
		}
	}

	// Restricted domains require expiresAt
	if domainSensitivity == "restricted" && membership.Spec.ExpiresAt == nil {
		setCondition(&membership.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "ExpiryRequired",
			Message:            "Restricted domain membership requires expiresAt to be set",
			ObservedGeneration: membership.Generation,
		})
		membership.Status.Phase = ""
		return ctrl.Result{}, r.Status().Update(ctx, membership)
	}

	// Determine target namespaces — sandbox namespaces get the role-appropriate binding,
	// production namespace always gets view-only for human roles.
	domainNs := fmt.Sprintf("%s-%s", membership.Spec.Application, membership.Spec.Domain)

	// Map role to ClusterRole name for sandbox/domain namespace
	clusterRole := roleToClusterRole(membership.Spec.Role)

	// Create RoleBinding in sandbox namespace (domain namespace used as primary workspace)
	rbName := fmt.Sprintf("membership-%s", membership.Name)
	if err := r.ensureRoleBinding(ctx, membership, rbName, domainNs, clusterRole); err != nil {
		return ctrl.Result{}, err
	}

	// Production RBAC lockdown: all human roles get view-only in production namespace
	// Production namespace is the same as domain namespace for now.
	// The sandbox namespaces get the full role, production namespaces get view-only.
	// For sandbox-specific bindings, list all sandbox namespaces for this domain.
	sandboxList := &choristerv1alpha1.ChoSandboxList{}
	if err := r.List(ctx, sandboxList, client.InNamespace(membership.Namespace)); err != nil {
		return ctrl.Result{}, err
	}

	for _, sb := range sandboxList.Items {
		if sb.Spec.Application == membership.Spec.Application && sb.Spec.Domain == membership.Spec.Domain {
			sbNs := SandboxNamespace(sb.Spec.Application, sb.Spec.Domain, sb.Spec.Name)
			sbRbName := fmt.Sprintf("membership-%s", membership.Name)
			if err := r.ensureRoleBinding(ctx, membership, sbRbName, sbNs, clusterRole); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// Production namespace: view-only for all human roles
	prodRbName := fmt.Sprintf("membership-%s-prod", membership.Name)
	if err := r.ensureRoleBinding(ctx, membership, prodRbName, domainNs, "view"); err != nil {
		return ctrl.Result{}, err
	}

	// Update status
	membership.Status.Phase = "Active"
	membership.Status.RoleBindingRef = rbName
	setCondition(&membership.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            fmt.Sprintf("RoleBinding created in namespace %s", domainNs),
		ObservedGeneration: membership.Generation,
	})

	if err := r.Status().Update(ctx, membership); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciled ChoDomainMembership", "identity", membership.Spec.Identity, "role", membership.Spec.Role, "namespace", domainNs)

	// If there's an expiry, requeue before it expires
	if membership.Spec.ExpiresAt != nil {
		untilExpiry := time.Until(membership.Spec.ExpiresAt.Time)
		if untilExpiry > 0 {
			// For OIDC-sourced memberships, use the shorter of expiry or sync interval
			if membership.Spec.Source == "oidc-group" && oidcSyncRequeueInterval < untilExpiry {
				return ctrl.Result{RequeueAfter: oidcSyncRequeueInterval}, nil
			}
			return ctrl.Result{RequeueAfter: untilExpiry}, nil
		}
	}

	// OIDC-sourced memberships need periodic re-reconciliation
	if membership.Spec.Source == "oidc-group" {
		return ctrl.Result{RequeueAfter: oidcSyncRequeueInterval}, nil
	}

	return ctrl.Result{}, nil
}

// handleDeprovisionedMembership removes RoleBindings when OIDC group membership is lost.
func (r *ChoDomainMembershipReconciler) handleDeprovisionedMembership(ctx context.Context, membership *choristerv1alpha1.ChoDomainMembership) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Delete all RoleBindings owned by this membership
	rbList := &rbacv1.RoleBindingList{}
	if err := r.List(ctx, rbList, client.MatchingLabels{labelMembership: membership.Name}); err != nil {
		return ctrl.Result{}, err
	}
	for i := range rbList.Items {
		if err := r.Delete(ctx, &rbList.Items[i]); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	membership.Status.Phase = "Deprovisioned"
	membership.Status.RoleBindingRef = ""
	setCondition(&membership.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "OIDCGroupRemoved",
		Message:            fmt.Sprintf("Identity %q is no longer a member of OIDC group %q", membership.Spec.Identity, membership.Spec.OIDCGroup),
		ObservedGeneration: membership.Generation,
	})

	if err := r.Status().Update(ctx, membership); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Deprovisioned membership", "identity", membership.Spec.Identity, "oidcGroup", membership.Spec.OIDCGroup)
	return ctrl.Result{RequeueAfter: oidcSyncRequeueInterval}, nil
}

// handleExpiredMembership removes RoleBindings and marks membership as expired.
func (r *ChoDomainMembershipReconciler) handleExpiredMembership(ctx context.Context, membership *choristerv1alpha1.ChoDomainMembership) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Delete all RoleBindings owned by this membership
	rbList := &rbacv1.RoleBindingList{}
	if err := r.List(ctx, rbList, client.MatchingLabels{labelMembership: membership.Name}); err != nil {
		return ctrl.Result{}, err
	}
	for i := range rbList.Items {
		if err := r.Delete(ctx, &rbList.Items[i]); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	membership.Status.Phase = "Expired"
	membership.Status.RoleBindingRef = ""
	setCondition(&membership.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "Expired",
		Message:            "Membership has expired, RoleBindings removed",
		ObservedGeneration: membership.Generation,
	})

	if err := r.Status().Update(ctx, membership); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Expired membership", "identity", membership.Spec.Identity, "role", membership.Spec.Role)
	return ctrl.Result{}, nil
}

// ensureRoleBinding creates or updates a RoleBinding in the given namespace.
func (r *ChoDomainMembershipReconciler) ensureRoleBinding(ctx context.Context, membership *choristerv1alpha1.ChoDomainMembership, name, namespace, clusterRole string) error {
	desired := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				labelApplication: membership.Spec.Application,
				labelDomain:      membership.Spec.Domain,
				labelMembership:  membership.Name,
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     rbacv1.UserKind,
				Name:     membership.Spec.Identity,
				APIGroup: rbacv1.GroupName,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     clusterRole,
		},
	}

	existing := &rbacv1.RoleBinding{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Update if subjects or roleRef changed
	needsUpdate := false
	if len(existing.Subjects) != len(desired.Subjects) ||
		(len(existing.Subjects) > 0 && existing.Subjects[0].Name != desired.Subjects[0].Name) {
		existing.Subjects = desired.Subjects
		needsUpdate = true
	}
	if existing.RoleRef.Name != desired.RoleRef.Name {
		// RoleRef is immutable — delete and recreate
		if err := r.Delete(ctx, existing); err != nil {
			return err
		}
		return r.Create(ctx, desired)
	}
	if needsUpdate {
		return r.Update(ctx, existing)
	}
	return nil
}

// roleToClusterRole maps a membership role to the corresponding Kubernetes ClusterRole.
func roleToClusterRole(role string) string {
	switch role {
	case "org-admin":
		return "admin"
	case "domain-admin":
		return "admin"
	case "developer":
		return "edit"
	case "viewer":
		return "view"
	default:
		return "view"
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChoDomainMembershipReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choristerv1alpha1.ChoDomainMembership{}).
		Named("chodomainmembership").
		Complete(r)
}
