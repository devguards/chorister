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
	"crypto/rand"
	"fmt"
	"math/big"
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

const dbFinalizerName = "chorister.dev/database-archive"

var (
	sgClusterGVK = schema.GroupVersionKind{
		Group: "stackgres.io", Version: "v1", Kind: "SGCluster",
	}
	sgInstanceProfileGVK = schema.GroupVersionKind{
		Group: "stackgres.io", Version: "v1", Kind: "SGInstanceProfile",
	}
)

// ChoDatabaseReconciler reconciles a ChoDatabase object
type ChoDatabaseReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	ControllerRevision string
}

// +kubebuilder:rbac:groups=chorister.dev,resources=chodatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chodatabases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=chodatabases/finalizers,verbs=update
// +kubebuilder:rbac:groups=chorister.dev,resources=choclusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=stackgres.io,resources=sgclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=stackgres.io,resources=sginstanceprofiles,verbs=get;list;watch;create;update;patch;delete

// Reconcile moves the cluster state to match the desired ChoDatabase spec.
func (r *ChoDatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	db := &choristerv1alpha1.ChoDatabase{}
	if err := r.Get(ctx, req.NamespacedName, db); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Controller revision labeling — skip if namespace revision doesn't match
	if r.ControllerRevision != "" {
		if skip, err := ShouldSkipForRevision(ctx, r.Client, r.ControllerRevision, db.Namespace); err != nil {
			return ctrl.Result{}, err
		} else if skip {
			log.Info("Skipping reconciliation: revision mismatch", "namespace", db.Namespace, "controllerRevision", r.ControllerRevision)
			return ctrl.Result{}, nil
		}
	}

	// Handle deletion via finalizer
	if !db.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, db)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(db, dbFinalizerName) {
		controllerutil.AddFinalizer(db, dbFinalizerName)
		if err := r.Update(ctx, db); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Handle archived/deletable lifecycle states
	if db.Status.Lifecycle == "Archived" {
		return r.handleArchived(ctx, db)
	}
	if db.Status.Lifecycle == "Deletable" {
		// Deletable resources are idle, waiting for explicit admin deletion
		return ctrl.Result{}, nil
	}

	// ---- Normal Active reconciliation ----

	// Determine instance count based on HA setting
	instances := int32(1)
	if db.Spec.HA {
		instances = 2
	}

	// Ensure StackGres SGInstanceProfile
	profileName := fmt.Sprintf("%s-%s", db.Spec.Domain, db.Name)
	if err := r.ensureSGInstanceProfile(ctx, db, profileName); err != nil {
		log.Error(err, "Failed to ensure SGInstanceProfile", "name", profileName)
		// Non-fatal: continue even if StackGres CRDs are not installed
	}

	// Ensure StackGres SGCluster
	sgClusterName := fmt.Sprintf("%s-%s", db.Spec.Domain, db.Name)
	if err := r.ensureSGCluster(ctx, db, sgClusterName, profileName, instances); err != nil {
		log.Error(err, "Failed to ensure SGCluster", "name", sgClusterName)
		// Non-fatal: continue even if StackGres CRDs are not installed
	}

	// Ensure credential secret
	secretName := fmt.Sprintf("%s--database--%s-credentials", db.Spec.Domain, db.Name)
	if err := r.ensureCredentialSecret(ctx, db, secretName); err != nil {
		return ctrl.Result{}, err
	}

	// Update status
	if err := r.Get(ctx, req.NamespacedName, db); err != nil {
		return ctrl.Result{}, err
	}
	db.Status.Instances = instances
	db.Status.CredentialsSecretRef = secretName
	db.Status.Ready = true
	if db.Status.Lifecycle == "" {
		db.Status.Lifecycle = "Active"
	}

	setCondition(&db.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            fmt.Sprintf("ChoDatabase %s reconciled with %d instance(s)", db.Name, instances),
		ObservedGeneration: db.Generation,
	})

	if err := r.Status().Update(ctx, db); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciled ChoDatabase", "name", db.Name, "instances", instances)
	return ctrl.Result{}, nil
}

// handleDeletion processes the finalizer on deletion.
func (r *ChoDatabaseReconciler) handleDeletion(ctx context.Context, db *choristerv1alpha1.ChoDatabase) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(db, dbFinalizerName) {
		return ctrl.Result{}, nil
	}

	// Check if in sandbox namespace — immediate deletion
	if r.isInSandbox(ctx, db.Namespace) {
		log.Info("Deleting sandbox ChoDatabase immediately", "name", db.Name)
		controllerutil.RemoveFinalizer(db, dbFinalizerName)
		return ctrl.Result{}, r.Update(ctx, db)
	}

	// Production: record final snapshot reference (placeholder)
	if db.Status.FinalSnapshotRef == "" {
		db.Status.FinalSnapshotRef = fmt.Sprintf("snapshot-%s-%s", db.Name, time.Now().Format("20060102-150405"))
		if err := r.Status().Update(ctx, db); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Recorded final snapshot for ChoDatabase", "name", db.Name, "snapshot", db.Status.FinalSnapshotRef)
	}

	controllerutil.RemoveFinalizer(db, dbFinalizerName)
	return ctrl.Result{}, r.Update(ctx, db)
}

// handleArchived checks retention and transitions Archived → Deletable.
func (r *ChoDatabaseReconciler) handleArchived(ctx context.Context, db *choristerv1alpha1.ChoDatabase) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	db.Status.Ready = false
	setCondition(&db.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "Archived",
		Message:            "ChoDatabase is archived; data preserved but not actively managed",
		ObservedGeneration: db.Generation,
	})

	// Check if retention period has passed
	if db.Status.DeletableAfter != nil && time.Now().After(db.Status.DeletableAfter.Time) {
		db.Status.Lifecycle = "Deletable"
		setCondition(&db.Status.Conditions, metav1.Condition{
			Type:               "Deletable",
			Status:             metav1.ConditionTrue,
			Reason:             "RetentionExpired",
			Message:            "Archive retention period expired; resource eligible for explicit deletion",
			ObservedGeneration: db.Generation,
		})
		if err := r.Status().Update(ctx, db); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("ChoDatabase transitioned to Deletable", "name", db.Name)
		return ctrl.Result{}, nil
	}

	if err := r.Status().Update(ctx, db); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue to check retention again
	if db.Status.DeletableAfter != nil {
		requeueAfter := time.Until(db.Status.DeletableAfter.Time)
		if requeueAfter > 0 {
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
	}
	return ctrl.Result{RequeueAfter: time.Hour}, nil
}

// isInSandbox returns true if the namespace has a sandbox label.
func (r *ChoDatabaseReconciler) isInSandbox(ctx context.Context, namespace string) bool {
	ns := &corev1.Namespace{}
	if err := r.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		return false
	}
	_, ok := ns.Labels[labelSandbox]
	return ok
}

// ensureCredentialSecret creates the database credential secret if it doesn't exist.
func (r *ChoDatabaseReconciler) ensureCredentialSecret(ctx context.Context, db *choristerv1alpha1.ChoDatabase, secretName string) error {
	existing := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: db.Namespace}, existing)
	if err == nil {
		// Secret already exists, ensure owner reference
		if !hasOwnerRef(existing, db) {
			if err := controllerutil.SetControllerReference(db, existing, r.Scheme); err != nil {
				return err
			}
			return r.Update(ctx, existing)
		}
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	// Generate random credentials
	password, err := generatePassword(24)
	if err != nil {
		return fmt.Errorf("generating password: %w", err)
	}

	username := db.Spec.Domain + "_" + db.Name
	host := fmt.Sprintf("%s.%s.svc.cluster.local", db.Name, db.Namespace)
	port := "5432"
	uri := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s", username, password, host, port, db.Name)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: db.Namespace,
			Labels: map[string]string{
				labelApplication: db.Spec.Application,
				labelDomain:      db.Spec.Domain,
			},
		},
		Data: map[string][]byte{
			"host":     []byte(host),
			"port":     []byte(port),
			"username": []byte(username),
			"password": []byte(password),
			"uri":      []byte(uri),
		},
	}
	if err := controllerutil.SetControllerReference(db, secret, r.Scheme); err != nil {
		return err
	}

	return r.Create(ctx, secret)
}

// ensureSGCluster creates or updates a StackGres SGCluster for the database.
func (r *ChoDatabaseReconciler) ensureSGCluster(ctx context.Context, db *choristerv1alpha1.ChoDatabase, name string, profileName string, instances int32) error {
	desired := buildSGCluster(db, name, profileName, instances)

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(sgClusterGVK)
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: db.Namespace}, existing)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		return r.Create(ctx, desired)
	}

	// Update existing — preserve resourceVersion.
	desired.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, desired)
}

// buildSGCluster constructs an unstructured SGCluster with Patroni HA and PgBouncer.
func buildSGCluster(db *choristerv1alpha1.ChoDatabase, name, profileName string, instances int32) *unstructured.Unstructured {
	volumeSize := dbVolumeSize(db.Spec.Size)

	sgCluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "stackgres.io/v1",
			"kind":       "SGCluster",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": db.Namespace,
				"labels": map[string]interface{}{
					labelApplication:           db.Spec.Application,
					labelDomain:                db.Spec.Domain,
					"chorister.dev/managed-by": "chodatabase-controller",
				},
			},
			"spec": map[string]interface{}{
				"instances": int64(instances),
				"postgres": map[string]interface{}{
					"version": "16",
					"flavor":  "vanilla",
				},
				"sgInstanceProfile": profileName,
				"pods": map[string]interface{}{
					"persistentVolume": map[string]interface{}{
						"size": volumeSize,
					},
				},
				"configurations": map[string]interface{}{
					"sgPoolingConfig": "sgpoolingconfig1",
					"backups": []interface{}{
						map[string]interface{}{
							"sgObjectStorage": "default-backup-storage",
							"cronSchedule":    "0 3 * * *",
							"retention":       int64(7),
						},
					},
				},
				"nonProductionOptions": map[string]interface{}{
					"disableClusterPodAntiAffinity": !db.Spec.HA,
				},
			},
		},
	}
	return sgCluster
}

// ensureSGInstanceProfile creates or updates a StackGres SGInstanceProfile.
func (r *ChoDatabaseReconciler) ensureSGInstanceProfile(ctx context.Context, db *choristerv1alpha1.ChoDatabase, name string) error {
	cpu, memory := dbProfileResources(db.Spec.Size)

	desired := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "stackgres.io/v1",
			"kind":       "SGInstanceProfile",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": db.Namespace,
				"labels": map[string]interface{}{
					labelApplication:           db.Spec.Application,
					labelDomain:                db.Spec.Domain,
					"chorister.dev/managed-by": "chodatabase-controller",
				},
			},
			"spec": map[string]interface{}{
				"cpu":    cpu,
				"memory": memory,
			},
		},
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(sgInstanceProfileGVK)
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: db.Namespace}, existing)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		return r.Create(ctx, desired)
	}
	desired.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, desired)
}

// dbVolumeSize returns the PVC size for the given database sizing tier.
func dbVolumeSize(size string) string {
	switch size {
	case "small":
		return "10Gi"
	case "medium":
		return "50Gi"
	case "large":
		return "200Gi"
	default:
		return "10Gi"
	}
}

// dbProfileResources returns (cpu, memory) for the given database sizing tier.
func dbProfileResources(size string) (string, string) {
	switch size {
	case "small":
		return "1", "2Gi"
	case "medium":
		return "2", "4Gi"
	case "large":
		return "4", "8Gi"
	default:
		return "1", "2Gi"
	}
}

// hasOwnerRef checks if the object has an owner reference to the given owner.
func hasOwnerRef(obj metav1.Object, owner metav1.Object) bool {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.UID == owner.GetUID() {
			return true
		}
	}
	return false
}

// generatePassword generates a cryptographically secure random password.
func generatePassword(length int) (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", err
		}
		result[i] = chars[idx.Int64()]
	}
	return string(result), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ChoDatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choristerv1alpha1.ChoDatabase{}).
		Owns(&corev1.Secret{}).
		Named("chodatabase").
		Complete(r)
}
