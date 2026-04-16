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
var chocachelog = logf.Log.WithName("chocache-resource")

// SetupChoCacheWebhookWithManager registers the webhook for ChoCache in the manager.
func SetupChoCacheWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &choristerv1alpha1.ChoCache{}).
		WithValidator(&ChoCacheCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-chorister-dev-v1alpha1-chocache,mutating=false,failurePolicy=fail,sideEffects=None,groups=chorister.dev,resources=chocaches,verbs=create;update,versions=v1alpha1,name=vchocache-v1alpha1.kb.io,admissionReviewVersions=v1

type ChoCacheCustomValidator struct{}

func (v *ChoCacheCustomValidator) ValidateCreate(_ context.Context, obj *choristerv1alpha1.ChoCache) (admission.Warnings, error) {
	chocachelog.Info("Validation for ChoCache upon creation", "name", obj.GetName())
	return nil, validateChoCache(obj)
}

func (v *ChoCacheCustomValidator) ValidateUpdate(_ context.Context, _, newObj *choristerv1alpha1.ChoCache) (admission.Warnings, error) {
	chocachelog.Info("Validation for ChoCache upon update", "name", newObj.GetName())
	return nil, validateChoCache(newObj)
}

func (v *ChoCacheCustomValidator) ValidateDelete(_ context.Context, obj *choristerv1alpha1.ChoCache) (admission.Warnings, error) {
	chocachelog.Info("Validation for ChoCache upon deletion", "name", obj.GetName())
	return nil, nil
}

func validateChoCache(cache *choristerv1alpha1.ChoCache) error {
	var allErrs field.ErrorList

	if cache.Spec.Application == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "application"), "application is required"))
	}
	if cache.Spec.Domain == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("spec", "domain"), "domain is required"))
	}
	if cache.Spec.Size == "" && cache.Spec.Resources == nil {
		allErrs = append(allErrs, field.Required(field.NewPath("spec"), "either size or resources must be specified"))
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "chorister.dev", Kind: "ChoCache"},
		cache.Name,
		allErrs,
	)
}
