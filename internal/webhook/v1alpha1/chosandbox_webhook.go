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
	"regexp"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

// nolint:unused
var chosandboxlog = logf.Log.WithName("chosandbox-resource")

var sandboxNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// SetupChoSandboxWebhookWithManager registers the webhook for ChoSandbox in the manager.
func SetupChoSandboxWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &choristerv1alpha1.ChoSandbox{}).
		WithValidator(&ChoSandboxCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-chorister-dev-v1alpha1-chosandbox,mutating=false,failurePolicy=fail,sideEffects=None,groups=chorister.dev,resources=chosandboxes,verbs=create;update,versions=v1alpha1,name=vchosandbox-v1alpha1.kb.io,admissionReviewVersions=v1

type ChoSandboxCustomValidator struct{}

func (v *ChoSandboxCustomValidator) ValidateCreate(_ context.Context, obj *choristerv1alpha1.ChoSandbox) (admission.Warnings, error) {
	chosandboxlog.Info("Validation for ChoSandbox upon creation", "name", obj.GetName())
	return nil, validateChoSandbox(obj)
}

func (v *ChoSandboxCustomValidator) ValidateUpdate(_ context.Context, _, newObj *choristerv1alpha1.ChoSandbox) (admission.Warnings, error) {
	chosandboxlog.Info("Validation for ChoSandbox upon update", "name", newObj.GetName())
	return nil, validateChoSandbox(newObj)
}

func (v *ChoSandboxCustomValidator) ValidateDelete(_ context.Context, obj *choristerv1alpha1.ChoSandbox) (admission.Warnings, error) {
	chosandboxlog.Info("Validation for ChoSandbox upon deletion", "name", obj.GetName())
	return nil, nil
}

func validateChoSandbox(sandbox *choristerv1alpha1.ChoSandbox) error {
	var allErrs field.ErrorList

	if sandbox.Spec.Application == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "application"), "application is required"))
	}
	if sandbox.Spec.Domain == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "domain"), "domain is required"))
	}
	if sandbox.Spec.Name == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "name"), "sandbox name is required"))
	} else if !sandboxNamePattern.MatchString(sandbox.Spec.Name) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "name"), sandbox.Spec.Name,
			"must match pattern ^[a-z][a-z0-9-]*$"))
	}
	if sandbox.Spec.Owner == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "owner"), "owner is required"))
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "chorister.dev", Kind: "ChoSandbox"},
		sandbox.Name,
		allErrs,
	)
}
