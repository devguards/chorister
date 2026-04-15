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
var chonetworklog = logf.Log.WithName("chonetwork-resource")

// SetupChoNetworkWebhookWithManager registers the webhook for ChoNetwork in the manager.
func SetupChoNetworkWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &choristerv1alpha1.ChoNetwork{}).
		WithValidator(&ChoNetworkCustomValidator{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-chorister-chorister-dev-v1alpha1-chonetwork,mutating=false,failurePolicy=fail,sideEffects=None,groups=chorister.chorister.dev,resources=chonetworks,verbs=create;update,versions=v1alpha1,name=vchonetwork-v1alpha1.kb.io,admissionReviewVersions=v1

// ChoNetworkCustomValidator struct is responsible for validating the ChoNetwork resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ChoNetworkCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ChoNetwork.
func (v *ChoNetworkCustomValidator) ValidateCreate(_ context.Context, obj *choristerv1alpha1.ChoNetwork) (admission.Warnings, error) {
	chonetworklog.Info("Validation for ChoNetwork upon creation", "name", obj.GetName())
	return nil, validateChoNetwork(obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ChoNetwork.
func (v *ChoNetworkCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *choristerv1alpha1.ChoNetwork) (admission.Warnings, error) {
	chonetworklog.Info("Validation for ChoNetwork upon update", "name", newObj.GetName())
	return nil, validateChoNetwork(newObj)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ChoNetwork.
func (v *ChoNetworkCustomValidator) ValidateDelete(_ context.Context, obj *choristerv1alpha1.ChoNetwork) (admission.Warnings, error) {
	chonetworklog.Info("Validation for ChoNetwork upon deletion", "name", obj.GetName())
	return nil, nil
}

func validateChoNetwork(network *choristerv1alpha1.ChoNetwork) error {
	var allErrs field.ErrorList

	// Ingress auth required for internet-facing
	if errs := validation.ValidateIngressAuth(network); len(errs) > 0 {
		for _, e := range errs {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "ingress"), nil, e))
		}
	}

	// Egress wildcard prohibition
	if errs := validation.ValidateEgressWildcard(network); len(errs) > 0 {
		for _, e := range errs {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "egress", "allowlist"), nil, e))
		}
	}

	if len(allErrs) == 0 {
		return nil
	}

	gvk := schema.GroupVersionKind{
		Group:   "chorister.chorister.dev",
		Version: "v1alpha1",
		Kind:    "ChoNetwork",
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind},
		network.Name,
		allErrs,
	)
}
