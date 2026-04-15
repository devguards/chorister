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

var _ = Describe("ChoApplication Webhook", func() {
	var (
		validator ChoApplicationCustomValidator
	)

	BeforeEach(func() {
		validator = ChoApplicationCustomValidator{}
	})

	Context("When creating or updating ChoApplication under Validating Webhook", func() {
		It("Should admit a valid ChoApplication", func() {
			obj := &choristerv1alpha1.ChoApplication{
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance:       "essential",
						ArchiveRetention: "90d",
						Promotion: choristerv1alpha1.PromotionPolicy{
							RequiredApprovers: 1,
							AllowedRoles:      []string{"developer"},
						},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: "payments", Sensitivity: "internal"},
					},
				},
			}
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeNil())
		})

		It("Should deny creation when consumes reference is unresolved", func() {
			obj := &choristerv1alpha1.ChoApplication{
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"dev"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{
							Name: "payments",
							Consumes: []choristerv1alpha1.ConsumeRef{
								{Domain: "auth", Services: []string{"auth-svc"}, Port: 8080},
							},
						},
						{Name: "auth"}, // no supplies
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not declare supplies"))
		})

		It("Should deny creation when dependency cycle exists", func() {
			obj := &choristerv1alpha1.ChoApplication{
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"dev"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{
							Name:     "a",
							Consumes: []choristerv1alpha1.ConsumeRef{{Domain: "b", Port: 8080}},
							Supplies: &choristerv1alpha1.SupplySpec{Port: 8080},
						},
						{
							Name:     "b",
							Consumes: []choristerv1alpha1.ConsumeRef{{Domain: "a", Port: 8080}},
							Supplies: &choristerv1alpha1.SupplySpec{Port: 8080},
						},
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cycle"))
		})

		It("Should deny creation when compliance escalation is violated", func() {
			obj := &choristerv1alpha1.ChoApplication{
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "regulated",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 2, AllowedRoles: []string{"admin"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: "payments", Sensitivity: "public"},
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("weaker than application compliance"))
		})

		It("Should deny creation when archive retention is below 30 days", func() {
			obj := &choristerv1alpha1.ChoApplication{
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance:       "essential",
						ArchiveRetention: "7d",
						Promotion:        choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"dev"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: "payments"},
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("below minimum 30 days"))
		})

		It("Should validate on update with same rules", func() {
			oldObj := &choristerv1alpha1.ChoApplication{
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "essential",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 1, AllowedRoles: []string{"dev"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{{Name: "payments"}},
				},
			}
			newObj := &choristerv1alpha1.ChoApplication{
				Spec: choristerv1alpha1.ChoApplicationSpec{
					Owners: []string{"owner@example.com"},
					Policy: choristerv1alpha1.ApplicationPolicy{
						Compliance: "regulated",
						Promotion:  choristerv1alpha1.PromotionPolicy{RequiredApprovers: 2, AllowedRoles: []string{"admin"}},
					},
					Domains: []choristerv1alpha1.DomainSpec{
						{Name: "payments", Sensitivity: "public"},
					},
				},
			}
			_, err := validator.ValidateUpdate(ctx, oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("weaker than application compliance"))
		})
	})
})
