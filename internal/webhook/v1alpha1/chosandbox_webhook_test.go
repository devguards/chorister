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

var _ = Describe("ChoSandbox Webhook", func() {
	var validator ChoSandboxCustomValidator

	BeforeEach(func() {
		validator = ChoSandboxCustomValidator{}
	})

	Context("When validating ChoSandbox", func() {
		It("Should admit a valid sandbox", func() {
			obj := &choristerv1alpha1.ChoSandbox{
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "myapp",
					Domain:      "payments",
					Name:        "dev",
					Owner:       "developer@example.com",
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("Should admit a sandbox name with hyphens and numbers", func() {
			obj := &choristerv1alpha1.ChoSandbox{
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "myapp",
					Domain:      "payments",
					Name:        "feature-123-test",
					Owner:       "dev@example.com",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should deny when application is missing", func() {
			obj := &choristerv1alpha1.ChoSandbox{
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Domain: "payments",
					Name:   "dev",
					Owner:  "dev@example.com",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("application"))
		})

		It("Should deny when domain is missing", func() {
			obj := &choristerv1alpha1.ChoSandbox{
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "myapp",
					Name:        "dev",
					Owner:       "dev@example.com",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("domain"))
		})

		It("Should deny when name is missing", func() {
			obj := &choristerv1alpha1.ChoSandbox{
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "myapp",
					Domain:      "payments",
					Owner:       "dev@example.com",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name"))
		})

		It("Should deny when owner is missing", func() {
			obj := &choristerv1alpha1.ChoSandbox{
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "myapp",
					Domain:      "payments",
					Name:        "dev",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("owner"))
		})

		It("Should deny name starting with a number", func() {
			obj := &choristerv1alpha1.ChoSandbox{
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "myapp",
					Domain:      "payments",
					Name:        "123-invalid",
					Owner:       "dev@example.com",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("pattern"))
		})

		It("Should deny name with uppercase letters", func() {
			obj := &choristerv1alpha1.ChoSandbox{
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "myapp",
					Domain:      "payments",
					Name:        "MyFeature",
					Owner:       "dev@example.com",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("pattern"))
		})

		It("Should deny name with underscores", func() {
			obj := &choristerv1alpha1.ChoSandbox{
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "myapp",
					Domain:      "payments",
					Name:        "my_feature",
					Owner:       "dev@example.com",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("pattern"))
		})

		It("Should apply same rules on update", func() {
			old := &choristerv1alpha1.ChoSandbox{
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "myapp",
					Domain:      "payments",
					Name:        "dev",
					Owner:       "dev@example.com",
				},
			}
			updated := &choristerv1alpha1.ChoSandbox{
				Spec: choristerv1alpha1.ChoSandboxSpec{
					Application: "myapp",
					Domain:      "payments",
					Name:        "INVALID",
					Owner:       "dev@example.com",
				},
			}
			_, err := validator.ValidateUpdate(ctx, old, updated)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("pattern"))
		})

		It("Should allow delete", func() {
			obj := &choristerv1alpha1.ChoSandbox{}
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
