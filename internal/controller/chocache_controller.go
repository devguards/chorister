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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

// cacheSizeMap maps size names to resource requirements for Dragonfly.
var cacheSizeMap = map[string]corev1.ResourceRequirements{
	"small": {
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("250m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
	},
	"medium": {
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("250m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
	},
	"large": {
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	},
}

// ChoCacheReconciler reconciles a ChoCache object
type ChoCacheReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	ControllerRevision string
}

// +kubebuilder:rbac:groups=chorister.dev,resources=chocaches,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chocaches/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=chocaches/finalizers,verbs=update
// +kubebuilder:rbac:groups=chorister.dev,resources=choclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// Reconcile moves the cluster state to match the desired ChoCache spec.
func (r *ChoCacheReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	cache := &choristerv1alpha1.ChoCache{}
	if err := r.Get(ctx, req.NamespacedName, cache); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Controller revision labeling — skip if namespace revision doesn't match
	if r.ControllerRevision != "" {
		if skip, err := ShouldSkipForRevision(ctx, r.Client, r.ControllerRevision, cache.Namespace); err != nil {
			return ctrl.Result{}, err
		} else if skip {
			log.Info("Skipping reconciliation: revision mismatch", "namespace", cache.Namespace, "controllerRevision", r.ControllerRevision)
			return ctrl.Result{}, nil
		}
	}

	// Reconcile the Dragonfly Deployment
	if err := r.reconcileDeployment(ctx, cache); err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile the Service
	if err := r.reconcileService(ctx, cache); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure credential secret
	secretName := fmt.Sprintf("%s--cache--%s-credentials", cache.Spec.Domain, cache.Name)
	if err := r.ensureCredentialSecret(ctx, cache, secretName); err != nil {
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.Get(ctx, req.NamespacedName, cache); err != nil {
		return ctrl.Result{}, err
	}
	cache.Status.CredentialsSecretRef = secretName
	cache.Status.Ready = true

	setCondition(&cache.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            fmt.Sprintf("ChoCache %s reconciled with size %s", cache.Name, cache.Spec.Size),
		ObservedGeneration: cache.Generation,
	})

	if err := r.Status().Update(ctx, cache); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciled ChoCache", "name", cache.Name, "size", cache.Spec.Size)
	return ctrl.Result{}, nil
}

func (r *ChoCacheReconciler) reconcileDeployment(ctx context.Context, cache *choristerv1alpha1.ChoCache) error {
	labels := cacheLabels(cache)
	resources := cacheResources(cache)
	replicas := int32(1)

	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cache.Name,
			Namespace: cache.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:      "dragonfly",
							Image:     "docker.dragonflydb.io/dragonflydb/dragonfly:latest",
							Resources: resources,
							Ports: []corev1.ContainerPort{
								{Name: "redis", ContainerPort: 6379, Protocol: corev1.ProtocolTCP},
							},
						},
					},
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(cache, desired, r.Scheme); err != nil {
		return err
	}

	existing := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	if !equality.Semantic.DeepEqual(existing.Spec.Template, desired.Spec.Template) ||
		!equality.Semantic.DeepEqual(existing.Spec.Replicas, desired.Spec.Replicas) {
		existing.Spec.Template = desired.Spec.Template
		existing.Spec.Replicas = desired.Spec.Replicas
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *ChoCacheReconciler) reconcileService(ctx context.Context, cache *choristerv1alpha1.ChoCache) error {
	labels := cacheLabels(cache)
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cache.Name,
			Namespace: cache.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{Name: "redis", Port: 6379, TargetPort: intstr.FromInt32(6379), Protocol: corev1.ProtocolTCP},
			},
		},
	}
	if err := controllerutil.SetControllerReference(cache, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: cache.Name, Namespace: cache.Namespace}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	if !equality.Semantic.DeepEqual(existing.Spec.Ports, desired.Spec.Ports) ||
		!equality.Semantic.DeepEqual(existing.Spec.Selector, desired.Spec.Selector) {
		existing.Spec.Ports = desired.Spec.Ports
		existing.Spec.Selector = desired.Spec.Selector
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *ChoCacheReconciler) ensureCredentialSecret(ctx context.Context, cache *choristerv1alpha1.ChoCache, secretName string) error {
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cache.Namespace}, existing)
	if err == nil {
		if !hasOwnerRef(existing, cache) {
			if err := controllerutil.SetControllerReference(cache, existing, r.Scheme); err != nil {
				return err
			}
			return r.Update(ctx, existing)
		}
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	host := fmt.Sprintf("%s.%s.svc.cluster.local", cache.Name, cache.Namespace)
	port := "6379"
	uri := fmt.Sprintf("redis://%s:%s", host, port)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cache.Namespace,
			Labels: map[string]string{
				labelApplication: cache.Spec.Application,
				labelDomain:      cache.Spec.Domain,
			},
		},
		Data: map[string][]byte{
			"host": []byte(host),
			"port": []byte(port),
			"uri":  []byte(uri),
		},
	}
	if err := controllerutil.SetControllerReference(cache, secret, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, secret)
}

func cacheLabels(cache *choristerv1alpha1.ChoCache) map[string]string {
	return map[string]string{
		labelApplication: cache.Spec.Application,
		labelDomain:      cache.Spec.Domain,
		"app":            cache.Name,
	}
}

func cacheResources(cache *choristerv1alpha1.ChoCache) corev1.ResourceRequirements {
	if cache.Spec.Resources != nil {
		return *cache.Spec.Resources
	}
	if r, ok := cacheSizeMap[cache.Spec.Size]; ok {
		return r
	}
	return cacheSizeMap["small"]
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChoCacheReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choristerv1alpha1.ChoCache{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Named("chocache").
		Complete(r)
}
