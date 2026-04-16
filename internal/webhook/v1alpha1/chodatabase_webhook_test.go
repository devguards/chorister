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
	corev1 "k8s.io/api/core/v1"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var _ = Describe("ChoDatabase Webhook", func() {
	var validator ChoDatabaseCustomValidator

	BeforeEach(func() {
		validator = ChoDatabaseCustomValidator{}
	})

	Context("When validating ChoDatabase", func() {
		It("Should admit a valid database with size", func() {
			obj := &choristerv1alpha1.ChoDatabase{
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Domain:      "payments",
					Engine:      "postgres",
					Size:        "small",
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("Should admit a valid database with resources", func() {
			obj := &choristerv1alpha1.ChoDatabase{
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Domain:      "payments",
					Engine:      "postgres",
					Resources:   &corev1.ResourceRequirements{},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should deny when application is missing", func() {
			obj := &choristerv1alpha1.ChoDatabase{
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Domain: "payments",
					Size:   "small",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("application"))
		})

		It("Should deny when domain is missing", func() {
			obj := &choristerv1alpha1.ChoDatabase{
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Size:        "small",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("domain"))
		})

		It("Should deny when neither size nor resources is set", func() {
			obj := &choristerv1alpha1.ChoDatabase{
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Domain:      "payments",
					Engine:      "postgres",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("size or resources"))
		})

		It("Should collect multiple errors", func() {
			obj := &choristerv1alpha1.ChoDatabase{
				Spec: choristerv1alpha1.ChoDatabaseSpec{},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("application"))
			Expect(err.Error()).To(ContainSubstring("domain"))
		})

		It("Should apply same rules on update", func() {
			old := &choristerv1alpha1.ChoDatabase{
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Domain:      "payments",
					Size:        "small",
				},
			}
			updated := &choristerv1alpha1.ChoDatabase{
				Spec: choristerv1alpha1.ChoDatabaseSpec{
					Application: "myapp",
					Domain:      "",
				},
			}
			_, err := validator.ValidateUpdate(ctx, old, updated)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("domain"))
		})

		It("Should allow delete", func() {
			obj := &choristerv1alpha1.ChoDatabase{}
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
