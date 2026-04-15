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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ChoNetwork Webhook", func() {
	var (
		validator ChoNetworkCustomValidator
	)

	BeforeEach(func() {
		validator = ChoNetworkCustomValidator{}
	})

	Context("When creating or updating ChoNetwork under Validating Webhook", func() {
		It("Should admit a valid internal ingress without auth", func() {
			obj := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "internal-ingress"},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: "myapp",
					Domain:      "payments",
					Ingress: &choristerv1alpha1.NetworkIngressSpec{
						From: "internal",
						Port: 8080,
					},
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("Should deny internet ingress without auth", func() {
			obj := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "no-auth-internet"},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: "myapp",
					Domain:      "payments",
					Ingress: &choristerv1alpha1.NetworkIngressSpec{
						From: "internet",
						Port: 443,
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("requires an auth block"))
		})

		It("Should admit internet ingress with JWT auth", func() {
			obj := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "auth-internet"},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: "myapp",
					Domain:      "payments",
					Ingress: &choristerv1alpha1.NetworkIngressSpec{
						From: "internet",
						Port: 443,
						Auth: &choristerv1alpha1.NetworkAuthSpec{
							JWT: &choristerv1alpha1.JWTAuthSpec{
								Issuer:  "https://idp.example.com",
								JWKSUri: "https://idp.example.com/.well-known/jwks.json",
							},
						},
					},
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("Should deny wildcard egress", func() {
			obj := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "wildcard-egress"},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: "myapp",
					Domain:      "payments",
					Egress: &choristerv1alpha1.NetworkEgressSpec{
						Allowlist: []string{"*"},
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("wildcard egress"))
		})

		It("Should admit specific egress destinations", func() {
			obj := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "specific-egress"},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: "myapp",
					Domain:      "payments",
					Egress: &choristerv1alpha1.NetworkEgressSpec{
						Allowlist: []string{"api.stripe.com", "api.twilio.com"},
					},
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("Should validate on update with same rules", func() {
			oldObj := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "valid-network"},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: "myapp",
					Domain:      "payments",
					Egress: &choristerv1alpha1.NetworkEgressSpec{
						Allowlist: []string{"api.stripe.com"},
					},
				},
			}
			newObj := &choristerv1alpha1.ChoNetwork{
				ObjectMeta: metav1.ObjectMeta{Name: "valid-network"},
				Spec: choristerv1alpha1.ChoNetworkSpec{
					Application: "myapp",
					Domain:      "payments",
					Egress: &choristerv1alpha1.NetworkEgressSpec{
						Allowlist: []string{"*"},
					},
				},
			}
			_, err := validator.ValidateUpdate(ctx, oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("wildcard egress"))
		})
	})
})
