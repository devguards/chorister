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
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

// nolint:unused
var chostoragelog = logf.Log.WithName("chostorage-resource")

// SetupChoStorageWebhookWithManager registers the webhook for ChoStorage in the manager.
func SetupChoStorageWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &choristerv1alpha1.ChoStorage{}).
		WithValidator(&ChoStorageCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-chorister-dev-v1alpha1-chostorage,mutating=false,failurePolicy=fail,sideEffects=None,groups=chorister.dev,resources=chostorages,verbs=create;update,versions=v1alpha1,name=vchostorage-v1alpha1.kb.io,admissionReviewVersions=v1

type ChoStorageCustomValidator struct{}

func (v *ChoStorageCustomValidator) ValidateCreate(_ context.Context, obj *choristerv1alpha1.ChoStorage) (admission.Warnings, error) {
	chostoragelog.Info("Validation for ChoStorage upon creation", "name", obj.GetName())
	return nil, validateChoStorage(obj)
}

func (v *ChoStorageCustomValidator) ValidateUpdate(_ context.Context, _, newObj *choristerv1alpha1.ChoStorage) (admission.Warnings, error) {
	chostoragelog.Info("Validation for ChoStorage upon update", "name", newObj.GetName())
	return nil, validateChoStorage(newObj)
}

func (v *ChoStorageCustomValidator) ValidateDelete(_ context.Context, obj *choristerv1alpha1.ChoStorage) (admission.Warnings, error) {
	chostoragelog.Info("Validation for ChoStorage upon deletion", "name", obj.GetName())
	return nil, nil
}

func validateChoStorage(storage *choristerv1alpha1.ChoStorage) error {
	var allErrs field.ErrorList

	if storage.Spec.Application == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "application"), "application is required"))
	}
	if storage.Spec.Domain == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "domain"), "domain is required"))
	}
	if storage.Spec.Variant == "object" && storage.Spec.ObjectBackend == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "objectBackend"),
			"objectBackend is required for object variant"))
	}
	if (storage.Spec.Variant == "block" || storage.Spec.Variant == "file") && storage.Spec.Size == nil {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "size"),
			fmt.Sprintf("size is required for %s variant", storage.Spec.Variant)))
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "chorister.dev", Kind: "ChoStorage"},
		storage.Name,
		allErrs,
	)
}
