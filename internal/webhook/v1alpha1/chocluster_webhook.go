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

package v1alpha1

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

// nolint:unused
var choclusterlog = logf.Log.WithName("chocluster-resource")

// SetupChoClusterWebhookWithManager registers the webhook for ChoCluster in the manager.
func SetupChoClusterWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &choristerv1alpha1.ChoCluster{}).
		WithValidator(&ChoClusterCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-chorister-dev-v1alpha1-chocluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=chorister.dev,resources=choclusters,verbs=create;update,versions=v1alpha1,name=vchocluster-v1alpha1.kb.io,admissionReviewVersions=v1

type ChoClusterCustomValidator struct{}

func (v *ChoClusterCustomValidator) ValidateCreate(_ context.Context, obj *choristerv1alpha1.ChoCluster) (admission.Warnings, error) {
	choclusterlog.Info("Validation for ChoCluster upon creation", "name", obj.GetName())
	return nil, validateChoCluster(obj)
}

func (v *ChoClusterCustomValidator) ValidateUpdate(_ context.Context, _, newObj *choristerv1alpha1.ChoCluster) (admission.Warnings, error) {
	choclusterlog.Info("Validation for ChoCluster upon update", "name", newObj.GetName())
	return nil, validateChoCluster(newObj)
}

func (v *ChoClusterCustomValidator) ValidateDelete(_ context.Context, obj *choristerv1alpha1.ChoCluster) (admission.Warnings, error) {
	choclusterlog.Info("Validation for ChoCluster upon deletion", "name", obj.GetName())
	return nil, nil
}

func validateChoCluster(cluster *choristerv1alpha1.ChoCluster) error {
	var allErrs field.ErrorList

	if cluster.Spec.CloudProvider != nil {
		cp := cluster.Spec.CloudProvider
		if cp.Provider == "" {
			allErrs = append(allErrs, field.Required(field.NewPath("spec", "cloudProvider", "provider"), "cloud provider is required"))
		}
	}

	if cluster.Spec.ExternalSecretBackend != nil {
		esb := cluster.Spec.ExternalSecretBackend
		if esb.Provider == "" {
			allErrs = append(allErrs, field.Required(field.NewPath("spec", "externalSecretBackend", "provider"), "provider is required"))
		}
		if esb.SecretStoreRef == "" {
			allErrs = append(allErrs, field.Required(field.NewPath("spec", "externalSecretBackend", "secretStoreRef"), "secretStoreRef is required"))
		}
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "chorister.dev", Kind: "ChoCluster"},
		cluster.Name,
		allErrs,
	)
}
