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

// queueSizeMap maps size names to resource requirements for NATS.
var queueSizeMap = map[string]corev1.ResourceRequirements{
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

// ChoQueueReconciler reconciles a ChoQueue object
type ChoQueueReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=chorister.dev,resources=choqueues,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=choqueues/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=choqueues/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile moves the cluster state to match the desired ChoQueue spec.
func (r *ChoQueueReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	queue := &choristerv1alpha1.ChoQueue{}
	if err := r.Get(ctx, req.NamespacedName, queue); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Reconcile the NATS StatefulSet
	if err := r.reconcileStatefulSet(ctx, queue); err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile the headless Service for the StatefulSet
	if err := r.reconcileService(ctx, queue); err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile the client Service
	if err := r.reconcileClientService(ctx, queue); err != nil {
		return ctrl.Result{}, err
	}

	// Ensure credential secret
	secretName := fmt.Sprintf("%s--queue--%s-credentials", queue.Spec.Domain, queue.Name)
	if err := r.ensureCredentialSecret(ctx, queue, secretName); err != nil {
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.Get(ctx, req.NamespacedName, queue); err != nil {
		return ctrl.Result{}, err
	}
	queue.Status.CredentialsSecretRef = secretName
	queue.Status.Ready = true
	if queue.Status.Lifecycle == "" {
		queue.Status.Lifecycle = "Active"
	}

	setCondition(&queue.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            fmt.Sprintf("ChoQueue %s reconciled", queue.Name),
		ObservedGeneration: queue.Generation,
	})

	if err := r.Status().Update(ctx, queue); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciled ChoQueue", "name", queue.Name, "type", queue.Spec.Type)
	return ctrl.Result{}, nil
}

func (r *ChoQueueReconciler) reconcileStatefulSet(ctx context.Context, queue *choristerv1alpha1.ChoQueue) error {
	labels := queueLabels(queue)
	resources := queueResources(queue)
	replicas := int32(1)

	desired := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      queue.Name,
			Namespace: queue.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: queue.Name + "-headless",
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:      "nats",
							Image:     "nats:2-alpine",
							Args:      []string{"-js", "-sd", "/data"},
							Resources: resources,
							Ports: []corev1.ContainerPort{
								{Name: "client", ContainerPort: 4222, Protocol: corev1.ProtocolTCP},
								{Name: "cluster", ContainerPort: 6222, Protocol: corev1.ProtocolTCP},
								{Name: "monitor", ContainerPort: 8222, Protocol: corev1.ProtocolTCP},
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "data", MountPath: "/data"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(queue, desired, r.Scheme); err != nil {
		return err
	}

	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace}, existing)
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

func (r *ChoQueueReconciler) reconcileService(ctx context.Context, queue *choristerv1alpha1.ChoQueue) error {
	labels := queueLabels(queue)
	svcName := queue.Name + "-headless"
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: queue.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "None",
			Selector:  labels,
			Ports: []corev1.ServicePort{
				{Name: "client", Port: 4222, TargetPort: intstr.FromInt32(4222), Protocol: corev1.ProtocolTCP},
				{Name: "cluster", Port: 6222, TargetPort: intstr.FromInt32(6222), Protocol: corev1.ProtocolTCP},
			},
		},
	}
	if err := controllerutil.SetControllerReference(queue, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: svcName, Namespace: queue.Namespace}, existing)
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

func (r *ChoQueueReconciler) reconcileClientService(ctx context.Context, queue *choristerv1alpha1.ChoQueue) error {
	labels := queueLabels(queue)
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      queue.Name,
			Namespace: queue.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{Name: "client", Port: 4222, TargetPort: intstr.FromInt32(4222), Protocol: corev1.ProtocolTCP},
			},
		},
	}
	if err := controllerutil.SetControllerReference(queue, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace}, existing)
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

func (r *ChoQueueReconciler) ensureCredentialSecret(ctx context.Context, queue *choristerv1alpha1.ChoQueue, secretName string) error {
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: queue.Namespace}, existing)
	if err == nil {
		if !hasOwnerRef(existing, queue) {
			if err := controllerutil.SetControllerReference(queue, existing, r.Scheme); err != nil {
				return err
			}
			return r.Update(ctx, existing)
		}
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	host := fmt.Sprintf("%s.%s.svc.cluster.local", queue.Name, queue.Namespace)
	port := "4222"
	uri := fmt.Sprintf("nats://%s:%s", host, port)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: queue.Namespace,
			Labels: map[string]string{
				labelApplication: queue.Spec.Application,
				labelDomain:      queue.Spec.Domain,
			},
		},
		Data: map[string][]byte{
			"host": []byte(host),
			"port": []byte(port),
			"uri":  []byte(uri),
		},
	}
	if err := controllerutil.SetControllerReference(queue, secret, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, secret)
}

func queueLabels(queue *choristerv1alpha1.ChoQueue) map[string]string {
	return map[string]string{
		labelApplication: queue.Spec.Application,
		labelDomain:      queue.Spec.Domain,
		"app":            queue.Name,
	}
}

func queueResources(queue *choristerv1alpha1.ChoQueue) corev1.ResourceRequirements {
	if queue.Spec.Resources != nil {
		return *queue.Spec.Resources
	}
	if r, ok := queueSizeMap[queue.Spec.Size]; ok {
		return r
	}
	return queueSizeMap["small"]
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChoQueueReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choristerv1alpha1.ChoQueue{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Named("choqueue").
		Complete(r)
}
