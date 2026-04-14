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
)

// ChoSandboxReconciler reconciles a ChoSandbox object
type ChoSandboxReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=chorister.dev,resources=chosandboxes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chosandboxes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=chosandboxes/finalizers,verbs=update

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

	// Ensure sandbox namespace exists
	if err := r.ensureSandboxNamespace(ctx, sandbox, nsName); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure default-deny NetworkPolicy in sandbox namespace
	if err := r.ensureSandboxDenyPolicy(ctx, sandbox, nsName); err != nil {
		return ctrl.Result{}, err
	}

	// Update status
	sandbox.Status.Namespace = nsName
	sandbox.Status.Phase = "Active"
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
	return ctrl.Result{}, nil
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

// SetupWithManager sets up the controller with the Manager.
func (r *ChoSandboxReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choristerv1alpha1.ChoSandbox{}).
		Named("chosandbox").
		Complete(r)
}
