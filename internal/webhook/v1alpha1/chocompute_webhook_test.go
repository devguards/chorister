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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var _ = Describe("ChoCompute Webhook", func() {
	var validator ChoComputeCustomValidator

	BeforeEach(func() {
		validator = ChoComputeCustomValidator{}
	})

	Context("When validating ChoCompute", func() {
		It("Should admit a valid long-running compute", func() {
			replicas := int32(2)
			obj := &choristerv1alpha1.ChoCompute{
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "payments",
					Image:       "registry.example.com/api:v1",
					Variant:     "long-running",
					Replicas:    &replicas,
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("Should deny when application is missing", func() {
			obj := &choristerv1alpha1.ChoCompute{
				Spec: choristerv1alpha1.ChoComputeSpec{
					Domain: "payments",
					Image:  "registry.example.com/api:v1",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("application"))
		})

		It("Should deny when domain is missing", func() {
			obj := &choristerv1alpha1.ChoCompute{
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Image:       "registry.example.com/api:v1",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("domain"))
		})

		It("Should deny when image is missing", func() {
			obj := &choristerv1alpha1.ChoCompute{
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "payments",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("image"))
		})

		It("Should deny cronjob variant without schedule", func() {
			obj := &choristerv1alpha1.ChoCompute{
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "payments",
					Image:       "registry.example.com/cron:v1",
					Variant:     "cronjob",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("schedule"))
		})

		It("Should admit cronjob variant with schedule", func() {
			obj := &choristerv1alpha1.ChoCompute{
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "payments",
					Image:       "registry.example.com/cron:v1",
					Variant:     "cronjob",
					Schedule:    "0 3 * * *",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should deny gpu variant without gpu spec", func() {
			obj := &choristerv1alpha1.ChoCompute{
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "ml",
					Image:       "registry.example.com/model:v1",
					Variant:     "gpu",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("gpu"))
		})

		It("Should admit gpu variant with gpu spec", func() {
			obj := &choristerv1alpha1.ChoCompute{
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "ml",
					Image:       "registry.example.com/model:v1",
					Variant:     "gpu",
					GPU:         &choristerv1alpha1.GPUSpec{Count: 1},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should collect multiple errors", func() {
			obj := &choristerv1alpha1.ChoCompute{
				Spec: choristerv1alpha1.ChoComputeSpec{
					Variant: "cronjob",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("application"))
			Expect(err.Error()).To(ContainSubstring("domain"))
			Expect(err.Error()).To(ContainSubstring("image"))
			Expect(err.Error()).To(ContainSubstring("schedule"))
		})

		It("Should apply same rules on update", func() {
			old := &choristerv1alpha1.ChoCompute{
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "payments",
					Image:       "registry.example.com/api:v1",
				},
			}
			updated := &choristerv1alpha1.ChoCompute{
				Spec: choristerv1alpha1.ChoComputeSpec{
					Application: "myapp",
					Domain:      "",
					Image:       "registry.example.com/api:v2",
				},
			}
			_, err := validator.ValidateUpdate(ctx, old, updated)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("domain"))
		})

		It("Should allow delete", func() {
			obj := &choristerv1alpha1.ChoCompute{}
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
