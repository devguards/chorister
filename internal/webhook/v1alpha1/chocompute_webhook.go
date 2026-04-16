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
var chocomputelog = logf.Log.WithName("chocompute-resource")

// SetupChoComputeWebhookWithManager registers the webhook for ChoCompute in the manager.
func SetupChoComputeWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &choristerv1alpha1.ChoCompute{}).
		WithValidator(&ChoComputeCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-chorister-dev-v1alpha1-chocompute,mutating=false,failurePolicy=fail,sideEffects=None,groups=chorister.dev,resources=chocomputes,verbs=create;update,versions=v1alpha1,name=vchocompute-v1alpha1.kb.io,admissionReviewVersions=v1

type ChoComputeCustomValidator struct{}

func (v *ChoComputeCustomValidator) ValidateCreate(_ context.Context, obj *choristerv1alpha1.ChoCompute) (admission.Warnings, error) {
	chocomputelog.Info("Validation for ChoCompute upon creation", "name", obj.GetName())
	return nil, validateChoCompute(obj)
}

func (v *ChoComputeCustomValidator) ValidateUpdate(_ context.Context, _, newObj *choristerv1alpha1.ChoCompute) (admission.Warnings, error) {
	chocomputelog.Info("Validation for ChoCompute upon update", "name", newObj.GetName())
	return nil, validateChoCompute(newObj)
}

func (v *ChoComputeCustomValidator) ValidateDelete(_ context.Context, obj *choristerv1alpha1.ChoCompute) (admission.Warnings, error) {
	chocomputelog.Info("Validation for ChoCompute upon deletion", "name", obj.GetName())
	return nil, nil
}

func validateChoCompute(compute *choristerv1alpha1.ChoCompute) error {
	var allErrs field.ErrorList

	if compute.Spec.Application == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "application"), "application is required"))
	}
	if compute.Spec.Domain == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "domain"), "domain is required"))
	}
	if compute.Spec.Image == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "image"), "image is required"))
	}
	if compute.Spec.Variant == "cronjob" && compute.Spec.Schedule == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "schedule"), "schedule is required for cronjob variant"))
	}
	if compute.Spec.Variant == "gpu" && compute.Spec.GPU == nil {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "gpu"), "gpu configuration is required for gpu variant"))
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "chorister.dev", Kind: "ChoCompute"},
		compute.Name,
		allErrs,
	)
}
