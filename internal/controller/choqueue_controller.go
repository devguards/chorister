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

const queueFinalizerName = "chorister.dev/queue-archive"

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
	Scheme             *runtime.Scheme
	ControllerRevision string
}

// +kubebuilder:rbac:groups=chorister.dev,resources=choqueues,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=choqueues/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=choqueues/finalizers,verbs=update
// +kubebuilder:rbac:groups=chorister.dev,resources=choclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

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

	// Controller revision labeling — skip if namespace revision doesn't match
	if r.ControllerRevision != "" {
		if skip, err := ShouldSkipForRevision(ctx, r.Client, r.ControllerRevision, queue.Namespace); err != nil {
			return ctrl.Result{}, err
		} else if skip {
			log.Info("Skipping reconciliation: revision mismatch", "namespace", queue.Namespace, "controllerRevision", r.ControllerRevision)
			return ctrl.Result{}, nil
		}
	}

	// Handle deletion via finalizer
	if !queue.DeletionTimestamp.IsZero() {
		return r.handleQueueDeletion(ctx, queue)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(queue, queueFinalizerName) {
		controllerutil.AddFinalizer(queue, queueFinalizerName)
		if err := r.Update(ctx, queue); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Handle archived/deletable lifecycle states
	if queue.Status.Lifecycle == "Archived" {
		return r.handleQueueArchived(ctx, queue)
	}
	if queue.Status.Lifecycle == "Deletable" {
		return ctrl.Result{}, nil
	}

	// ---- Normal Active reconciliation ----

	// Branch on queue type: streaming variant uses a different backing system
	if queue.Spec.Type == "streaming" {
		return r.reconcileStreamingQueue(ctx, queue)
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

// reconcileStreamingQueue handles the streaming variant (AutoMQ / Strimzi).
// The streaming variant does NOT deploy NATS resources. It sets a condition
// indicating that an external streaming operator (AutoMQ / Strimzi) is
// required and not yet installed. This keeps the controller idempotent
// while making the unsupported state explicit.
func (r *ChoQueueReconciler) reconcileStreamingQueue(ctx context.Context, queue *choristerv1alpha1.ChoQueue) (ctrl.Result, error) {
	if err := r.Get(ctx, types.NamespacedName{Name: queue.Name, Namespace: queue.Namespace}, queue); err != nil {
		return ctrl.Result{}, err
	}
	setCondition(&queue.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "StreamingOperatorNotInstalled",
		Message:            "Streaming queue variant requires AutoMQ or Strimzi operator, which is not yet installed",
		ObservedGeneration: queue.Generation,
	})
	queue.Status.Ready = false
	// Lifecycle stays empty until the streaming operator is installed.
	// Only set it to Active once reconciliation can fully proceed.
	if err := r.Status().Update(ctx, queue); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, fmt.Errorf("streaming queue variant not supported: AutoMQ/Strimzi operator not installed")
}

// handleQueueDeletion processes the finalizer on deletion.
func (r *ChoQueueReconciler) handleQueueDeletion(ctx context.Context, queue *choristerv1alpha1.ChoQueue) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(queue, queueFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Check if in sandbox namespace — immediate deletion
	ns := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: queue.Namespace}, ns); err == nil {
		if _, ok := ns.Labels[labelSandbox]; ok {
			log.Info("Deleting sandbox ChoQueue immediately", "name", queue.Name)
			controllerutil.RemoveFinalizer(queue, queueFinalizerName)
			return ctrl.Result{}, r.Update(ctx, queue)
		}
	}

	// Production: remove finalizer to allow deletion
	controllerutil.RemoveFinalizer(queue, queueFinalizerName)
	return ctrl.Result{}, r.Update(ctx, queue)
}

// handleQueueArchived checks retention and transitions Archived → Deletable.
func (r *ChoQueueReconciler) handleQueueArchived(ctx context.Context, queue *choristerv1alpha1.ChoQueue) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	queue.Status.Ready = false
	setCondition(&queue.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "Archived",
		Message:            "ChoQueue is archived; data preserved but not actively managed",
		ObservedGeneration: queue.Generation,
	})

	if queue.Status.DeletableAfter != nil && time.Now().After(queue.Status.DeletableAfter.Time) {
		queue.Status.Lifecycle = "Deletable"
		setCondition(&queue.Status.Conditions, metav1.Condition{
			Type:               "Deletable",
			Status:             metav1.ConditionTrue,
			Reason:             "RetentionExpired",
			Message:            "Archive retention period expired; resource eligible for explicit deletion",
			ObservedGeneration: queue.Generation,
		})
		if err := r.Status().Update(ctx, queue); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("ChoQueue transitioned to Deletable", "name", queue.Name)
		return ctrl.Result{}, nil
	}

	if err := r.Status().Update(ctx, queue); err != nil {
		return ctrl.Result{}, err
	}

	if queue.Status.DeletableAfter != nil {
		requeueAfter := time.Until(queue.Status.DeletableAfter.Time)
		if requeueAfter > 0 {
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
	}
	return ctrl.Result{RequeueAfter: time.Hour}, nil
}

func (r *ChoQueueReconciler) reconcileStatefulSet(ctx context.Context, queue *choristerv1alpha1.ChoQueue) error {
	labels := queueLabels(queue)
	resources := queueResources(queue)
	replicas := int32(1)

	// Build NATS args: enable JetStream with persistent data directory.
	natsArgs := []string{"-js", "-sd", "/data"}

	// Build JetStream config file content if custom settings are provided.
	var configVolumeSource *corev1.VolumeSource
	var configVolumeMount *corev1.VolumeMount
	if queue.Spec.JetStream != nil {
		jsConfig := buildJetStreamConfig(queue.Spec.JetStream)
		if jsConfig != "" {
			natsArgs = append(natsArgs, "-c", "/etc/nats/jetstream.conf")
			configVolumeSource = &corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: queue.Name + "-js-config",
					},
				},
			}
			configVolumeMount = &corev1.VolumeMount{
				Name: "js-config", MountPath: "/etc/nats",
			}
			// Ensure the ConfigMap exists.
			if err := r.ensureJetStreamConfigMap(ctx, queue, jsConfig); err != nil {
				return err
			}
		}
	}

	// Calculate PVC storage size — default 1Gi.
	storageSize := "1Gi"
	if queue.Spec.StorageSize != "" {
		storageSize = queue.Spec.StorageSize
	}
	storageParsed := resource.MustParse(storageSize)

	volumeMounts := []corev1.VolumeMount{
		{Name: "data", MountPath: "/data"},
	}
	if configVolumeMount != nil {
		volumeMounts = append(volumeMounts, *configVolumeMount)
	}

	volumes := []corev1.Volume{}
	if configVolumeSource != nil {
		volumes = append(volumes, corev1.Volume{
			Name:         "js-config",
			VolumeSource: *configVolumeSource,
		})
	}

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
							Args:      natsArgs,
							Resources: resources,
							Ports: []corev1.ContainerPort{
								{Name: "client", ContainerPort: 4222, Protocol: corev1.ProtocolTCP},
								{Name: "cluster", ContainerPort: 6222, Protocol: corev1.ProtocolTCP},
								{Name: "monitor", ContainerPort: 8222, Protocol: corev1.ProtocolTCP},
							},
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "data",
						Labels: labels,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: storageParsed,
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

// buildJetStreamConfig generates a NATS JetStream configuration snippet.
func buildJetStreamConfig(js *choristerv1alpha1.JetStreamConfig) string {
	if js == nil {
		return ""
	}
	lines := []string{"jetstream {", "  store_dir: /data"}
	if js.MaxBytes != "" {
		lines = append(lines, fmt.Sprintf("  max_mem_store: %s", js.MaxBytes))
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n")
}

// ensureJetStreamConfigMap creates or updates the ConfigMap for JetStream configuration.
func (r *ChoQueueReconciler) ensureJetStreamConfigMap(ctx context.Context, queue *choristerv1alpha1.ChoQueue, config string) error {
	cmName := queue.Name + "-js-config"
	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: queue.Namespace}, existing)
	if errors.IsNotFound(err) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmName,
				Namespace: queue.Namespace,
				Labels:    queueLabels(queue),
			},
			Data: map[string]string{
				"jetstream.conf": config,
			},
		}
		if setErr := controllerutil.SetControllerReference(queue, cm, r.Scheme); setErr != nil {
			return setErr
		}
		return r.Create(ctx, cm)
	}
	if err != nil {
		return err
	}
	if existing.Data["jetstream.conf"] != config {
		existing.Data["jetstream.conf"] = config
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
		Owns(&corev1.ConfigMap{}).
		Named("choqueue").
		Complete(r)
}
