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

var _ = Describe("ChoCluster Webhook", func() {
	var validator ChoClusterCustomValidator

	BeforeEach(func() {
		validator = ChoClusterCustomValidator{}
	})

	Context("When validating ChoCluster", func() {
		It("Should admit a minimal cluster with no optional fields", func() {
			obj := &choristerv1alpha1.ChoCluster{
				Spec: choristerv1alpha1.ChoClusterSpec{},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("Should admit a cluster with valid cloud provider", func() {
			obj := &choristerv1alpha1.ChoCluster{
				Spec: choristerv1alpha1.ChoClusterSpec{
					CloudProvider: &choristerv1alpha1.CloudProviderSpec{
						Provider: "aws",
						Region:   "us-west-2",
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should deny cloud provider without provider name", func() {
			obj := &choristerv1alpha1.ChoCluster{
				Spec: choristerv1alpha1.ChoClusterSpec{
					CloudProvider: &choristerv1alpha1.CloudProviderSpec{
						Region: "us-west-2",
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("provider"))
		})

		It("Should admit a cluster with valid external secret backend", func() {
			obj := &choristerv1alpha1.ChoCluster{
				Spec: choristerv1alpha1.ChoClusterSpec{
					ExternalSecretBackend: &choristerv1alpha1.ExternalSecretBackendSpec{
						Provider:       "aws",
						SecretStoreRef: "my-store",
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should deny external secret backend without provider", func() {
			obj := &choristerv1alpha1.ChoCluster{
				Spec: choristerv1alpha1.ChoClusterSpec{
					ExternalSecretBackend: &choristerv1alpha1.ExternalSecretBackendSpec{
						SecretStoreRef: "my-store",
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("provider"))
		})

		It("Should deny external secret backend without secretStoreRef", func() {
			obj := &choristerv1alpha1.ChoCluster{
				Spec: choristerv1alpha1.ChoClusterSpec{
					ExternalSecretBackend: &choristerv1alpha1.ExternalSecretBackendSpec{
						Provider: "aws",
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("secretStoreRef"))
		})

		It("Should collect multiple errors from both cloud provider and external secret backend", func() {
			obj := &choristerv1alpha1.ChoCluster{
				Spec: choristerv1alpha1.ChoClusterSpec{
					CloudProvider:         &choristerv1alpha1.CloudProviderSpec{},
					ExternalSecretBackend: &choristerv1alpha1.ExternalSecretBackendSpec{},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cloudProvider"))
			Expect(err.Error()).To(ContainSubstring("externalSecretBackend"))
		})

		It("Should apply same rules on update", func() {
			old := &choristerv1alpha1.ChoCluster{
				Spec: choristerv1alpha1.ChoClusterSpec{
					CloudProvider: &choristerv1alpha1.CloudProviderSpec{
						Provider: "aws",
					},
				},
			}
			updated := &choristerv1alpha1.ChoCluster{
				Spec: choristerv1alpha1.ChoClusterSpec{
					CloudProvider: &choristerv1alpha1.CloudProviderSpec{},
				},
			}
			_, err := validator.ValidateUpdate(ctx, old, updated)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("provider"))
		})

		It("Should allow delete", func() {
			obj := &choristerv1alpha1.ChoCluster{}
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
