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
var chodatabaselog = logf.Log.WithName("chodatabase-resource")

// SetupChoDatabaseWebhookWithManager registers the webhook for ChoDatabase in the manager.
func SetupChoDatabaseWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &choristerv1alpha1.ChoDatabase{}).
		WithValidator(&ChoDatabaseCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-chorister-dev-v1alpha1-chodatabase,mutating=false,failurePolicy=fail,sideEffects=None,groups=chorister.dev,resources=chodatabases,verbs=create;update,versions=v1alpha1,name=vchodatabase-v1alpha1.kb.io,admissionReviewVersions=v1

type ChoDatabaseCustomValidator struct{}

func (v *ChoDatabaseCustomValidator) ValidateCreate(_ context.Context, obj *choristerv1alpha1.ChoDatabase) (admission.Warnings, error) {
	chodatabaselog.Info("Validation for ChoDatabase upon creation", "name", obj.GetName())
	return nil, validateChoDatabase(obj)
}

func (v *ChoDatabaseCustomValidator) ValidateUpdate(_ context.Context, _, newObj *choristerv1alpha1.ChoDatabase) (admission.Warnings, error) {
	chodatabaselog.Info("Validation for ChoDatabase upon update", "name", newObj.GetName())
	return nil, validateChoDatabase(newObj)
}

func (v *ChoDatabaseCustomValidator) ValidateDelete(_ context.Context, obj *choristerv1alpha1.ChoDatabase) (admission.Warnings, error) {
	chodatabaselog.Info("Validation for ChoDatabase upon deletion", "name", obj.GetName())
	return nil, nil
}

func validateChoDatabase(db *choristerv1alpha1.ChoDatabase) error {
	var allErrs field.ErrorList

	if db.Spec.Application == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "application"), "application is required"))
	}
	if db.Spec.Domain == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "domain"), "domain is required"))
	}
	if db.Spec.Size == "" && db.Spec.Resources == nil {
		allErrs = append(allErrs, field.Required(field.NewPath("spec"), "either size or resources must be specified"))
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "chorister.dev", Kind: "ChoDatabase"},
		db.Name,
		allErrs,
	)
}
