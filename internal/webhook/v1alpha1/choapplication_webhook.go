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
	"github.com/chorister-dev/chorister/internal/validation"
)

// nolint:unused
// log is for logging in this package.
var choapplicationlog = logf.Log.WithName("choapplication-resource")

// SetupChoApplicationWebhookWithManager registers the webhook for ChoApplication in the manager.
func SetupChoApplicationWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &choristerv1alpha1.ChoApplication{}).
		WithValidator(&ChoApplicationCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-chorister-dev-v1alpha1-choapplication,mutating=false,failurePolicy=fail,sideEffects=None,groups=chorister.dev,resources=choapplications,verbs=create;update,versions=v1alpha1,name=vchoapplication-v1alpha1.kb.io,admissionReviewVersions=v1

// ChoApplicationCustomValidator struct is responsible for validating the ChoApplication resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ChoApplicationCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ChoApplication.
func (v *ChoApplicationCustomValidator) ValidateCreate(_ context.Context, obj *choristerv1alpha1.ChoApplication) (admission.Warnings, error) {
	choapplicationlog.Info("Validation for ChoApplication upon creation", "name", obj.GetName())
	return nil, validateChoApplication(obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ChoApplication.
func (v *ChoApplicationCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *choristerv1alpha1.ChoApplication) (admission.Warnings, error) {
	choapplicationlog.Info("Validation for ChoApplication upon update", "name", newObj.GetName())
	return nil, validateChoApplication(newObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ChoApplication.
func (v *ChoApplicationCustomValidator) ValidateDelete(_ context.Context, obj *choristerv1alpha1.ChoApplication) (admission.Warnings, error) {
	choapplicationlog.Info("Validation for ChoApplication upon deletion", "name", obj.GetName())
	return nil, nil
}

func validateChoApplication(app *choristerv1alpha1.ChoApplication) error {
	var allErrs field.ErrorList

	// Consumes/supplies consistency
	if errs := validation.ValidateConsumesSupplies(app); len(errs) > 0 {
		for _, e := range errs {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "domains"), nil, e))
		}
	}

	// Cycle detection
	if err := validation.ValidateCycleDetection(app); err != nil {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "domains"), nil, err.Error()))
	}

	// Compliance escalation
	if errs := validation.ValidateComplianceEscalation(app); len(errs) > 0 {
		for _, e := range errs {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "domains"), nil, e))
		}
	}

	// Archive retention minimum
	if errs := validation.ValidateArchiveRetentionMinimum(app); len(errs) > 0 {
		for _, e := range errs {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "policy", "archiveRetention"), app.Spec.Policy.ArchiveRetention, e))
		}
	}

	if len(allErrs) == 0 {
		return nil
	}

	gvk := schema.GroupVersionKind{
		Group:   "chorister.dev",
		Version: "v1alpha1",
		Kind:    "ChoApplication",
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
		app.Name,
		allErrs,
	)
}
