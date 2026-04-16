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

var _ = Describe("ChoPromotionRequest Webhook", func() {
	Context("When creating ChoPromotionRequest under Validating Webhook", func() {
		It("Should reject when application is empty", func() {
			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr-no-app",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Domain:      "payments",
					Sandbox:     "alice",
					RequestedBy: "dev@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).Error().To(HaveOccurred())
		})

		It("Should reject when domain is empty", func() {
			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr-no-domain",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: "myapp",
					Sandbox:     "alice",
					RequestedBy: "dev@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).Error().To(HaveOccurred())
		})

		It("Should reject when sandbox is empty", func() {
			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr-no-sandbox",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: "myapp",
					Domain:      "payments",
					RequestedBy: "dev@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).Error().To(HaveOccurred())
		})

		It("Should reject when requestedBy is empty", func() {
			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr-no-requester",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: "myapp",
					Domain:      "payments",
					Sandbox:     "alice",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).Error().To(HaveOccurred())
		})

		It("Should accept when all required fields are present", func() {
			pr := &choristerv1alpha1.ChoPromotionRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pr-valid",
					Namespace: "default",
				},
				Spec: choristerv1alpha1.ChoPromotionRequestSpec{
					Application: "myapp",
					Domain:      "payments",
					Sandbox:     "alice",
					RequestedBy: "dev@example.com",
				},
			}
			Expect(k8sClient.Create(ctx, pr)).To(Succeed())
		})
	})
})
