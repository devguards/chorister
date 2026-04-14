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

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"github.com/chorister-dev/chorister/internal/compiler"
	"github.com/chorister-dev/chorister/internal/validation"
)

// ChoNetworkReconciler reconciles a ChoNetwork object
type ChoNetworkReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=chorister.dev,resources=chonetworks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chonetworks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=chonetworks/finalizers,verbs=update
// +kubebuilder:rbac:groups=chorister.dev,resources=choapplications,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cilium.io,resources=ciliumnetworkpolicies,verbs=get;list;watch;create;update;patch;delete

// Reconcile moves the cluster state to match the desired ChoNetwork spec.
func (r *ChoNetworkReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	network := &choristerv1alpha1.ChoNetwork{}
	if err := r.Get(ctx, req.NamespacedName, network); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Compile-time guardrails: validate ingress auth and egress wildcard
	var validationErrors []string

	validationErrors = append(validationErrors, validation.ValidateIngressAuth(network)...)
	validationErrors = append(validationErrors, validation.ValidateEgressWildcard(network)...)

	// Validate against application policy (allowed IdPs)
	app, appFound := r.lookupApplication(ctx, network)
	if appFound {
		validationErrors = append(validationErrors, validation.ValidateIngressAllowedIdP(network, app.Spec.Policy)...)
		validationErrors = append(validationErrors, validation.ValidateEgressAllowedDestinations(network, app.Spec.Policy)...)
	}

	if len(validationErrors) > 0 {
		// Re-fetch before status update
		if err := r.Get(ctx, req.NamespacedName, network); err != nil {
			return ctrl.Result{}, err
		}
		network.Status.Ready = false
		setCondition(&network.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "ValidationFailed",
			Message:            strings.Join(validationErrors, "; "),
			ObservedGeneration: network.Generation,
		})
		if err := r.Status().Update(ctx, network); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("ChoNetwork validation failed", "name", network.Name, "errors", validationErrors)
		return ctrl.Result{}, nil
	}

	// Reconcile ingress NetworkPolicy if ingress spec is defined
	if network.Spec.Ingress != nil {
		if err := r.reconcileIngressPolicy(ctx, network); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.reconcileHTTPRoute(ctx, network); err != nil {
			return ctrl.Result{}, err
		}
	}

	if network.Spec.Egress != nil {
		if err := r.reconcileEgressPolicy(ctx, network); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Update status
	if err := r.Get(ctx, req.NamespacedName, network); err != nil {
		return ctrl.Result{}, err
	}
	network.Status.Ready = true

	setCondition(&network.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            fmt.Sprintf("ChoNetwork %s reconciled", network.Name),
		ObservedGeneration: network.Generation,
	})

	if err := r.Status().Update(ctx, network); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciled ChoNetwork", "name", network.Name)
	return ctrl.Result{}, nil
}

// reconcileIngressPolicy creates a NetworkPolicy allowing ingress on the specified port.
func (r *ChoNetworkReconciler) reconcileIngressPolicy(ctx context.Context, network *choristerv1alpha1.ChoNetwork) error {
	ingress := network.Spec.Ingress
	policyName := fmt.Sprintf("%s-ingress", network.Name)

	tcp := corev1.ProtocolTCP
	port := intstr.FromInt32(int32(ingress.Port))

	desired := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: network.Namespace,
			Labels: map[string]string{
				labelApplication: network.Spec.Application,
				labelDomain:      network.Spec.Domain,
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{Protocol: &tcp, Port: &port},
					},
				},
			},
		},
	}

	existing := &networkingv1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: policyName, Namespace: network.Namespace}, existing)
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

func (r *ChoNetworkReconciler) reconcileHTTPRoute(ctx context.Context, network *choristerv1alpha1.ChoNetwork) error {
	route := compiler.CompileIngressHTTPRoute(network)
	return ensureUnstructured(ctx, r.Client, route)
}

func (r *ChoNetworkReconciler) reconcileEgressPolicy(ctx context.Context, network *choristerv1alpha1.ChoNetwork) error {
	policy := compiler.CompileEgressCiliumPolicy(network)
	return ensureUnstructured(ctx, r.Client, policy)
}

func (r *ChoNetworkReconciler) lookupApplication(ctx context.Context, network *choristerv1alpha1.ChoNetwork) (*choristerv1alpha1.ChoApplication, bool) {
	for _, namespace := range []string{network.Namespace, "default"} {
		app := &choristerv1alpha1.ChoApplication{}
		if err := r.Get(ctx, types.NamespacedName{Name: network.Spec.Application, Namespace: namespace}, app); err == nil {
			return app, true
		}
	}
	return nil, false
}

func getUnstructuredSpec(obj *unstructured.Unstructured) map[string]interface{} {
	if spec, ok := obj.Object["spec"].(map[string]interface{}); ok {
		return spec
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChoNetworkReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choristerv1alpha1.ChoNetwork{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Named("chonetwork").
		Complete(r)
}
