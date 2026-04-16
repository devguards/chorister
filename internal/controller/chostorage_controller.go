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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

const storageFinalizerName = "chorister.dev/storage-archive"

// ChoStorageReconciler reconciles a ChoStorage object
type ChoStorageReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	ControllerRevision string
}

// +kubebuilder:rbac:groups=chorister.dev,resources=chostorages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chostorages/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=chostorages/finalizers,verbs=update
// +kubebuilder:rbac:groups=chorister.dev,resources=choclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// Reconcile moves the cluster state to match the desired ChoStorage spec.
func (r *ChoStorageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	storage := &choristerv1alpha1.ChoStorage{}
	if err := r.Get(ctx, req.NamespacedName, storage); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Controller revision labeling — skip if namespace revision doesn't match
	if r.ControllerRevision != "" {
		if skip, err := ShouldSkipForRevision(ctx, r.Client, r.ControllerRevision, storage.Namespace); err != nil {
			return ctrl.Result{}, err
		} else if skip {
			log.Info("Skipping reconciliation: revision mismatch", "namespace", storage.Namespace, "controllerRevision", r.ControllerRevision)
			return ctrl.Result{}, nil
		}
	}

	// Handle deletion via finalizer
	if !storage.DeletionTimestamp.IsZero() {
		return r.handleStorageDeletion(ctx, storage)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(storage, storageFinalizerName) {
		controllerutil.AddFinalizer(storage, storageFinalizerName)
		if err := r.Update(ctx, storage); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Handle archived/deletable lifecycle states
	if storage.Status.Lifecycle == "Archived" {
		return r.handleStorageArchived(ctx, storage)
	}
	if storage.Status.Lifecycle == "Deletable" {
		return ctrl.Result{}, nil
	}

	// ---- Normal Active reconciliation ----

	// Reconcile PVC for block/file variants
	if storage.Spec.Variant == "block" || storage.Spec.Variant == "file" {
		if err := r.reconcilePVC(ctx, storage); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile kro Claim for object variant
	if storage.Spec.Variant == "object" {
		if err := r.reconcileObjectStorageClaim(ctx, storage); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Update status
	if err := r.Get(ctx, req.NamespacedName, storage); err != nil {
		return ctrl.Result{}, err
	}
	storage.Status.Ready = true
	if storage.Status.Lifecycle == "" {
		storage.Status.Lifecycle = "Active"
	}

	// Set object-variant-specific status fields.
	if storage.Spec.Variant == "object" {
		storage.Status.BucketName = fmt.Sprintf("%s-%s-%s", storage.Spec.Application, storage.Spec.Domain, storage.Name)
		storage.Status.CredentialsSecretRef = fmt.Sprintf("%s-%s-%s-credentials", storage.Spec.Application, storage.Spec.Domain, storage.Name)
	}

	setCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            fmt.Sprintf("ChoStorage %s reconciled (variant=%s)", storage.Name, storage.Spec.Variant),
		ObservedGeneration: storage.Generation,
	})

	if err := r.Status().Update(ctx, storage); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciled ChoStorage", "name", storage.Name, "variant", storage.Spec.Variant)
	return ctrl.Result{}, nil
}

func (r *ChoStorageReconciler) reconcilePVC(ctx context.Context, storage *choristerv1alpha1.ChoStorage) error {
	pvcName := storage.Name
	accessMode := corev1.ReadWriteOnce
	switch storage.Spec.AccessMode {
	case "ReadWriteMany":
		accessMode = corev1.ReadWriteMany
	case "ReadOnlyMany":
		accessMode = corev1.ReadOnlyMany
	}

	existing := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: storage.Namespace}, existing)
	if errors.IsNotFound(err) {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: storage.Namespace,
				Labels: map[string]string{
					labelApplication: storage.Spec.Application,
					labelDomain:      storage.Spec.Domain,
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{},
				},
			},
		}
		if storage.Spec.Size != nil {
			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = *storage.Spec.Size
		}
		if storage.Spec.StorageClass != "" {
			pvc.Spec.StorageClassName = &storage.Spec.StorageClass
		}
		if err := controllerutil.SetControllerReference(storage, pvc, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, pvc)
	}
	return err
}

// handleStorageDeletion processes the finalizer on deletion.
func (r *ChoStorageReconciler) handleStorageDeletion(ctx context.Context, storage *choristerv1alpha1.ChoStorage) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(storage, storageFinalizerName) {
		return ctrl.Result{}, nil
	}

	ns := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: storage.Namespace}, ns); err == nil {
		if _, ok := ns.Labels[labelSandbox]; ok {
			log.Info("Deleting sandbox ChoStorage immediately", "name", storage.Name)
			controllerutil.RemoveFinalizer(storage, storageFinalizerName)
			return ctrl.Result{}, r.Update(ctx, storage)
		}
	}

	controllerutil.RemoveFinalizer(storage, storageFinalizerName)
	return ctrl.Result{}, r.Update(ctx, storage)
}

// handleStorageArchived checks retention and transitions Archived → Deletable.
func (r *ChoStorageReconciler) handleStorageArchived(ctx context.Context, storage *choristerv1alpha1.ChoStorage) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	storage.Status.Ready = false
	setCondition(&storage.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "Archived",
		Message:            "ChoStorage is archived; data preserved but not actively managed",
		ObservedGeneration: storage.Generation,
	})

	if storage.Status.DeletableAfter != nil && time.Now().After(storage.Status.DeletableAfter.Time) {
		storage.Status.Lifecycle = "Deletable"
		setCondition(&storage.Status.Conditions, metav1.Condition{
			Type:               "Deletable",
			Status:             metav1.ConditionTrue,
			Reason:             "RetentionExpired",
			Message:            "Archive retention period expired; resource eligible for explicit deletion",
			ObservedGeneration: storage.Generation,
		})
		if err := r.Status().Update(ctx, storage); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("ChoStorage transitioned to Deletable", "name", storage.Name)
		return ctrl.Result{}, nil
	}

	if err := r.Status().Update(ctx, storage); err != nil {
		return ctrl.Result{}, err
	}

	if storage.Status.DeletableAfter != nil {
		requeueAfter := time.Until(storage.Status.DeletableAfter.Time)
		if requeueAfter > 0 {
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
	}
	return ctrl.Result{RequeueAfter: time.Hour}, nil
}

// reconcileObjectStorageClaim creates or updates a kro ResourceGraphDefinition Claim
// for cloud object storage (S3/GCS/Azure Blob).
func (r *ChoStorageReconciler) reconcileObjectStorageClaim(ctx context.Context, storage *choristerv1alpha1.ChoStorage) error {
	log := logf.FromContext(ctx)
	claimName := storage.Name + "-object-claim"
	bucketName := fmt.Sprintf("%s-%s-%s", storage.Spec.Application, storage.Spec.Domain, storage.Name)

	claimGVK := schema.GroupVersionKind{
		Group:   "kro.run",
		Version: "v1alpha1",
		Kind:    "ObjectStorageClaim",
	}

	backend := storage.Spec.ObjectBackend
	if backend == "" {
		backend = "s3"
	}

	// Resolve region from ChoCluster cloud provider config
	region := r.resolveCloudProviderRegion(ctx)

	desired := &unstructured.Unstructured{}
	desired.SetGroupVersionKind(claimGVK)
	desired.SetName(claimName)
	desired.SetNamespace(storage.Namespace)
	desired.SetLabels(map[string]string{
		labelApplication: storage.Spec.Application,
		labelDomain:      storage.Spec.Domain,
	})

	spec := map[string]any{
		"bucketName": bucketName,
		"backend":    backend,
		"region":     region,
	}
	if storage.Spec.Size != nil {
		spec["maxSize"] = storage.Spec.Size.String()
	}
	desired.Object["spec"] = spec

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(claimGVK)
	err := r.Get(ctx, types.NamespacedName{Name: claimName, Namespace: storage.Namespace}, existing)
	if errors.IsNotFound(err) {
		log.Info("Creating kro ObjectStorageClaim", "name", claimName, "backend", backend)
		if err := controllerutil.SetControllerReference(storage, desired, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// Update existing claim spec if changed.
	existing.Object["spec"] = spec
	return r.Update(ctx, existing)
}

// resolveCloudProviderRegion looks up the ChoCluster to find the cloud provider region.
func (r *ChoStorageReconciler) resolveCloudProviderRegion(ctx context.Context) string {
	clusterList := &choristerv1alpha1.ChoClusterList{}
	if err := r.List(ctx, clusterList); err != nil {
		return "us-east-1"
	}
	for _, cluster := range clusterList.Items {
		if cluster.Spec.CloudProvider != nil && cluster.Spec.CloudProvider.Region != "" {
			return cluster.Spec.CloudProvider.Region
		}
	}
	return "us-east-1"
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChoStorageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choristerv1alpha1.ChoStorage{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Named("chostorage").
		Complete(r)
}
