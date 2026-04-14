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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

// ChoDatabaseReconciler reconciles a ChoDatabase object
type ChoDatabaseReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=chorister.dev,resources=chodatabases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=chorister.dev,resources=chodatabases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=chorister.dev,resources=chodatabases/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

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

	// Determine instance count based on HA setting
	instances := int32(1)
	if db.Spec.HA {
		instances = 2
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
