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
var chodomainmembershiplog = logf.Log.WithName("chodomainmembership-resource")

// SetupChoDomainMembershipWebhookWithManager registers the webhook for ChoDomainMembership in the manager.
func SetupChoDomainMembershipWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &choristerv1alpha1.ChoDomainMembership{}).
		WithValidator(&ChoDomainMembershipCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-chorister-dev-v1alpha1-chodomainmembership,mutating=false,failurePolicy=fail,sideEffects=None,groups=chorister.dev,resources=chodomainmemberships,verbs=create;update,versions=v1alpha1,name=vchodomainmembership-v1alpha1.kb.io,admissionReviewVersions=v1

// ChoDomainMembershipCustomValidator validates ChoDomainMembership resources.
// +kubebuilder:object:generate=false
type ChoDomainMembershipCustomValidator struct{}

// ValidateCreate implements webhook.CustomValidator.
func (v *ChoDomainMembershipCustomValidator) ValidateCreate(_ context.Context, obj *choristerv1alpha1.ChoDomainMembership) (admission.Warnings, error) {
	chodomainmembershiplog.Info("Validation for ChoDomainMembership upon creation", "name", obj.GetName())
	return nil, validateChoDomainMembership(obj)
}

// ValidateUpdate implements webhook.CustomValidator.
func (v *ChoDomainMembershipCustomValidator) ValidateUpdate(_ context.Context, _, newObj *choristerv1alpha1.ChoDomainMembership) (admission.Warnings, error) {
	chodomainmembershiplog.Info("Validation for ChoDomainMembership upon update", "name", newObj.GetName())
	return nil, validateChoDomainMembership(newObj)
}

// ValidateDelete implements webhook.CustomValidator.
func (v *ChoDomainMembershipCustomValidator) ValidateDelete(_ context.Context, obj *choristerv1alpha1.ChoDomainMembership) (admission.Warnings, error) {
	chodomainmembershiplog.Info("Validation for ChoDomainMembership upon deletion", "name", obj.GetName())
	return nil, nil
}

func validateChoDomainMembership(m *choristerv1alpha1.ChoDomainMembership) error {
	var allErrs field.ErrorList

	if m.Spec.Application == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "application"), "application is required"))
	}
	if m.Spec.Domain == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "domain"), "domain is required"))
	}
	if m.Spec.Identity == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "identity"), "identity is required"))
	}
	if m.Spec.Role == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "role"), "role is required"))
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "chorister.dev", Kind: "ChoDomainMembership"},
		m.Name,
		allErrs,
	)
}
