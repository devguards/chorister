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
	"k8s.io/apimachinery/pkg/api/resource"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

var _ = Describe("ChoStorage Webhook", func() {
	var validator ChoStorageCustomValidator

	BeforeEach(func() {
		validator = ChoStorageCustomValidator{}
	})

	Context("When validating ChoStorage", func() {
		It("Should admit a valid object storage with objectBackend", func() {
			size := resource.MustParse("50Gi")
			obj := &choristerv1alpha1.ChoStorage{
				Spec: choristerv1alpha1.ChoStorageSpec{
					Application:   "myapp",
					Domain:        "media",
					Variant:       "object",
					ObjectBackend: "s3",
					Size:          &size,
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("Should admit a valid block storage with size", func() {
			size := resource.MustParse("10Gi")
			obj := &choristerv1alpha1.ChoStorage{
				Spec: choristerv1alpha1.ChoStorageSpec{
					Application: "myapp",
					Domain:      "data",
					Variant:     "block",
					Size:        &size,
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should admit a valid file storage with size", func() {
			size := resource.MustParse("20Gi")
			obj := &choristerv1alpha1.ChoStorage{
				Spec: choristerv1alpha1.ChoStorageSpec{
					Application: "myapp",
					Domain:      "data",
					Variant:     "file",
					Size:        &size,
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should deny when application is missing", func() {
			obj := &choristerv1alpha1.ChoStorage{
				Spec: choristerv1alpha1.ChoStorageSpec{
					Domain:  "media",
					Variant: "block",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("application"))
		})

		It("Should deny when domain is missing", func() {
			obj := &choristerv1alpha1.ChoStorage{
				Spec: choristerv1alpha1.ChoStorageSpec{
					Application: "myapp",
					Variant:     "block",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("domain"))
		})

		It("Should deny object variant without objectBackend", func() {
			obj := &choristerv1alpha1.ChoStorage{
				Spec: choristerv1alpha1.ChoStorageSpec{
					Application: "myapp",
					Domain:      "media",
					Variant:     "object",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("objectBackend"))
		})

		It("Should deny block variant without size", func() {
			obj := &choristerv1alpha1.ChoStorage{
				Spec: choristerv1alpha1.ChoStorageSpec{
					Application: "myapp",
					Domain:      "data",
					Variant:     "block",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("size"))
		})

		It("Should deny file variant without size", func() {
			obj := &choristerv1alpha1.ChoStorage{
				Spec: choristerv1alpha1.ChoStorageSpec{
					Application: "myapp",
					Domain:      "data",
					Variant:     "file",
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("size"))
		})

		It("Should apply same rules on update", func() {
			size := resource.MustParse("10Gi")
			old := &choristerv1alpha1.ChoStorage{
				Spec: choristerv1alpha1.ChoStorageSpec{
					Application: "myapp",
					Domain:      "data",
					Variant:     "block",
					Size:        &size,
				},
			}
			updated := &choristerv1alpha1.ChoStorage{
				Spec: choristerv1alpha1.ChoStorageSpec{
					Application: "",
					Domain:      "data",
					Variant:     "block",
					Size:        &size,
				},
			}
			_, err := validator.ValidateUpdate(ctx, old, updated)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("application"))
		})

		It("Should allow delete", func() {
			obj := &choristerv1alpha1.ChoStorage{}
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
