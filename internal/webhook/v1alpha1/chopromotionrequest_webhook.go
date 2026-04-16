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
var chopromotionrequestlog = logf.Log.WithName("chopromotionrequest-resource")

// SetupChoPromotionRequestWebhookWithManager registers the webhook for ChoPromotionRequest in the manager.
func SetupChoPromotionRequestWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &choristerv1alpha1.ChoPromotionRequest{}).
		WithValidator(&ChoPromotionRequestCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-chorister-dev-v1alpha1-chopromotionrequest,mutating=false,failurePolicy=fail,sideEffects=None,groups=chorister.dev,resources=chopromotionrequests,verbs=create;update,versions=v1alpha1,name=vchopromotionrequest-v1alpha1.kb.io,admissionReviewVersions=v1

// ChoPromotionRequestCustomValidator validates ChoPromotionRequest resources.
// +kubebuilder:object:generate=false
type ChoPromotionRequestCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator.
func (v *ChoPromotionRequestCustomValidator) ValidateCreate(_ context.Context, obj *choristerv1alpha1.ChoPromotionRequest) (admission.Warnings, error) {
	chopromotionrequestlog.Info("Validation for ChoPromotionRequest upon creation", "name", obj.GetName())
	return nil, validateChoPromotionRequest(obj)
}

// ValidateUpdate implements webhook.CustomValidator.
func (v *ChoPromotionRequestCustomValidator) ValidateUpdate(_ context.Context, _, newObj *choristerv1alpha1.ChoPromotionRequest) (admission.Warnings, error) {
	chopromotionrequestlog.Info("Validation for ChoPromotionRequest upon update", "name", newObj.GetName())
	return nil, validateChoPromotionRequest(newObj)
}

// ValidateDelete implements webhook.CustomValidator.
func (v *ChoPromotionRequestCustomValidator) ValidateDelete(_ context.Context, obj *choristerv1alpha1.ChoPromotionRequest) (admission.Warnings, error) {
	chopromotionrequestlog.Info("Validation for ChoPromotionRequest upon deletion", "name", obj.GetName())
	return nil, nil
}

func validateChoPromotionRequest(pr *choristerv1alpha1.ChoPromotionRequest) error {
	var allErrs field.ErrorList

	if pr.Spec.Application == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "application"), "application is required"))
	}
	if pr.Spec.Domain == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "domain"), "domain is required"))
	}
	if pr.Spec.Sandbox == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "sandbox"), "sandbox is required"))
	}
	if pr.Spec.RequestedBy == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "requestedBy"), "requestedBy is required"))
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "chorister.dev", Kind: "ChoPromotionRequest"},
		pr.Name,
		allErrs,
	)
}
