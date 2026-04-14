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
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
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

// ChoComputeReconciler reconciles a ChoCompute object
type ChoComputeReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=chorister.dev,resources=chocomputes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chocomputes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=chocomputes/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete

// Reconcile moves the cluster state to match the desired ChoCompute spec.
func (r *ChoComputeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	compute := &choristerv1alpha1.ChoCompute{}
	if err := r.Get(ctx, req.NamespacedName, compute); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	variant := compute.Spec.Variant
	if variant == "" {
		variant = "long-running"
	}

	switch variant {
	case "long-running", "gpu":
		if err := r.reconcileDeployment(ctx, compute); err != nil {
			return ctrl.Result{}, err
		}
		if compute.Spec.Port != nil {
			if err := r.reconcileService(ctx, compute); err != nil {
				return ctrl.Result{}, err
			}
		}
		if err := r.reconcileHPA(ctx, compute); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.reconcilePDB(ctx, compute); err != nil {
			return ctrl.Result{}, err
		}
	case "job":
		if err := r.reconcileJob(ctx, compute); err != nil {
			return ctrl.Result{}, err
		}
	case "cronjob":
		if err := r.reconcileCronJob(ctx, compute); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Update status
	if err := r.updateStatus(ctx, compute, variant); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciled ChoCompute", "name", compute.Name, "variant", variant)
	return ctrl.Result{}, nil
}

func (r *ChoComputeReconciler) reconcileDeployment(ctx context.Context, compute *choristerv1alpha1.ChoCompute) error {
	labels := computeLabels(compute)
	replicas := int32(1)
	if compute.Spec.Replicas != nil {
		replicas = *compute.Spec.Replicas
	}

	container := corev1.Container{
		Name:      compute.Name,
		Image:     compute.Spec.Image,
		Env:       compute.Spec.Env,
		Command:   compute.Spec.Command,
		Args:      compute.Spec.Args,
		Resources: computeResources(compute),
	}

	if compute.Spec.Port != nil {
		container.Ports = []corev1.ContainerPort{{
			ContainerPort: *compute.Spec.Port,
			Protocol:      corev1.ProtocolTCP,
		}}
	}

	// GPU support
	if compute.Spec.GPU != nil {
		gpuType := compute.Spec.GPU.Type
		if gpuType == "" {
			gpuType = "nvidia.com/gpu"
		}
		if container.Resources.Limits == nil {
			container.Resources.Limits = corev1.ResourceList{}
		}
		gpuQty := resource.MustParse(fmt.Sprintf("%d", compute.Spec.GPU.Count))
		container.Resources.Limits[corev1.ResourceName(gpuType)] = gpuQty
	}

	desired := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      compute.Name,
			Namespace: compute.Namespace,
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
					Containers: []corev1.Container{container},
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(compute, desired, r.Scheme); err != nil {
		return err
	}

	existing := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Update if spec changed
	if !equality.Semantic.DeepEqual(existing.Spec.Template, desired.Spec.Template) ||
		!equality.Semantic.DeepEqual(existing.Spec.Replicas, desired.Spec.Replicas) {
		existing.Spec.Template = desired.Spec.Template
		existing.Spec.Replicas = desired.Spec.Replicas
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *ChoComputeReconciler) reconcileService(ctx context.Context, compute *choristerv1alpha1.ChoCompute) error {
	labels := computeLabels(compute)
	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      compute.Name,
			Namespace: compute.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{{
				Port:       *compute.Spec.Port,
				TargetPort: intstr.FromInt32(*compute.Spec.Port),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
	if err := controllerutil.SetControllerReference(compute, desired, r.Scheme); err != nil {
		return err
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Update ports if changed
	if !equality.Semantic.DeepEqual(existing.Spec.Ports, desired.Spec.Ports) ||
		!equality.Semantic.DeepEqual(existing.Spec.Selector, desired.Spec.Selector) {
		existing.Spec.Ports = desired.Spec.Ports
		existing.Spec.Selector = desired.Spec.Selector
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *ChoComputeReconciler) reconcileJob(ctx context.Context, compute *choristerv1alpha1.ChoCompute) error {
	labels := computeLabels(compute)
	container := corev1.Container{
		Name:      compute.Name,
		Image:     compute.Spec.Image,
		Env:       compute.Spec.Env,
		Command:   compute.Spec.Command,
		Args:      compute.Spec.Args,
		Resources: computeResources(compute),
	}

	desired := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      compute.Name,
			Namespace: compute.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers:    []corev1.Container{container},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(compute, desired, r.Scheme); err != nil {
		return err
	}

	existing := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	// Jobs are immutable after creation — no update
	return err
}

func (r *ChoComputeReconciler) reconcileCronJob(ctx context.Context, compute *choristerv1alpha1.ChoCompute) error {
	labels := computeLabels(compute)
	container := corev1.Container{
		Name:      compute.Name,
		Image:     compute.Spec.Image,
		Env:       compute.Spec.Env,
		Command:   compute.Spec.Command,
		Args:      compute.Spec.Args,
		Resources: computeResources(compute),
	}

	desired := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      compute.Name,
			Namespace: compute.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: compute.Spec.Schedule,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: labels},
						Spec: corev1.PodSpec{
							Containers:    []corev1.Container{container},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			},
		},
	}
	if err := controllerutil.SetControllerReference(compute, desired, r.Scheme); err != nil {
		return err
	}

	existing := &batchv1.CronJob{}
	err := r.Get(ctx, types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	if !equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		existing.Spec = desired.Spec
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *ChoComputeReconciler) reconcileHPA(ctx context.Context, compute *choristerv1alpha1.ChoCompute) error {
	if compute.Spec.Autoscaling == nil {
		return nil
	}

	as := compute.Spec.Autoscaling
	hpaName := fmt.Sprintf("%s-hpa", compute.Name)
	labels := computeLabels(compute)

	metrics := []autoscalingv2.MetricSpec{}
	if as.TargetCPUPercent != nil {
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: as.TargetCPUPercent,
				},
			},
		})
	}
	if as.TargetMemoryPercent != nil {
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceMemory,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: as.TargetMemoryPercent,
				},
			},
		})
	}

	desired := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hpaName,
			Namespace: compute.Namespace,
			Labels:    labels,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       compute.Name,
			},
			MinReplicas: &as.MinReplicas,
			MaxReplicas: as.MaxReplicas,
			Metrics:     metrics,
		},
	}
	if err := controllerutil.SetControllerReference(compute, desired, r.Scheme); err != nil {
		return err
	}

	existing := &autoscalingv2.HorizontalPodAutoscaler{}
	err := r.Get(ctx, types.NamespacedName{Name: hpaName, Namespace: compute.Namespace}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	if !equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		existing.Spec = desired.Spec
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *ChoComputeReconciler) reconcilePDB(ctx context.Context, compute *choristerv1alpha1.ChoCompute) error {
	replicas := int32(1)
	if compute.Spec.Replicas != nil {
		replicas = *compute.Spec.Replicas
	}

	// Only create PDB when replicas > 1
	if replicas <= 1 {
		return nil
	}

	pdbName := fmt.Sprintf("%s-pdb", compute.Name)
	labels := computeLabels(compute)
	minAvailable := intstr.FromInt32(replicas - 1)

	desired := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pdbName,
			Namespace: compute.Namespace,
			Labels:    labels,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector:     &metav1.LabelSelector{MatchLabels: labels},
		},
	}
	if err := controllerutil.SetControllerReference(compute, desired, r.Scheme); err != nil {
		return err
	}

	existing := &policyv1.PodDisruptionBudget{}
	err := r.Get(ctx, types.NamespacedName{Name: pdbName, Namespace: compute.Namespace}, existing)
	if errors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	if !equality.Semantic.DeepEqual(existing.Spec.MinAvailable, desired.Spec.MinAvailable) {
		existing.Spec.MinAvailable = desired.Spec.MinAvailable
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *ChoComputeReconciler) updateStatus(ctx context.Context, compute *choristerv1alpha1.ChoCompute, variant string) error {
	// Re-fetch to avoid conflicts
	if err := r.Get(ctx, types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace}, compute); err != nil {
		return err
	}

	switch variant {
	case "long-running", "gpu":
		deploy := &appsv1.Deployment{}
		if err := r.Get(ctx, types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace}, deploy); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
		} else {
			compute.Status.ReadyReplicas = deploy.Status.ReadyReplicas
			compute.Status.Ready = deploy.Status.ReadyReplicas > 0 &&
				deploy.Status.ReadyReplicas == deploy.Status.Replicas
		}
	case "job":
		job := &batchv1.Job{}
		if err := r.Get(ctx, types.NamespacedName{Name: compute.Name, Namespace: compute.Namespace}, job); err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
		} else {
			compute.Status.Ready = job.Status.Succeeded > 0
		}
	case "cronjob":
		compute.Status.Ready = true // CronJobs are "ready" once created
	}

	setCondition(&compute.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             conditionStatus(compute.Status.Ready),
		Reason:             "Reconciled",
		Message:            fmt.Sprintf("ChoCompute %s reconciled as %s", compute.Name, variant),
		ObservedGeneration: compute.Generation,
	})

	return r.Status().Update(ctx, compute)
}

func computeLabels(compute *choristerv1alpha1.ChoCompute) map[string]string {
	return map[string]string{
		labelApplication: compute.Spec.Application,
		labelDomain:      compute.Spec.Domain,
		"app":            compute.Name,
	}
}

func computeResources(compute *choristerv1alpha1.ChoCompute) corev1.ResourceRequirements {
	if compute.Spec.Resources != nil {
		return *compute.Spec.Resources
	}
	return corev1.ResourceRequirements{}
}

func conditionStatus(ready bool) metav1.ConditionStatus {
	if ready {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChoComputeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choristerv1alpha1.ChoCompute{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&batchv1.Job{}).
		Owns(&batchv1.CronJob{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Named("chocompute").
		Complete(r)
}
