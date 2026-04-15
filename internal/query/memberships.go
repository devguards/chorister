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

package query

import (
	"context"
	"time"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MemberFilter defines filtering criteria for domain membership queries.
type MemberFilter struct {
	App            string
	Domain         string
	Role           string
	IncludeExpired bool
}

// MemberInfo provides summary info about a domain membership.
type MemberInfo struct {
	Name            string
	Identity        string
	Role            string
	Domain          string
	Application     string
	Source          string
	ExpiresAt       *metav1.Time
	Phase           string
	DaysUntilExpiry int // negative means already expired
}

// ListMemberships returns memberships matching the given filters.
func (q *Querier) ListMemberships(ctx context.Context, filters MemberFilter) ([]MemberInfo, error) {
	var list choristerv1alpha1.ChoDomainMembershipList
	if err := q.list(ctx, &list); err != nil {
		return nil, wrapError("ChoDomainMembership", "", "", err)
	}

	now := time.Now()
	var result []MemberInfo
	for _, m := range list.Items {
		if filters.App != "" && m.Spec.Application != filters.App {
			continue
		}
		if filters.Domain != "" && m.Spec.Domain != filters.Domain {
			continue
		}
		if filters.Role != "" && m.Spec.Role != filters.Role {
			continue
		}

		info := memberInfoFromCR(&m, now)

		// Skip expired unless requested
		if !filters.IncludeExpired && info.Phase == "Expired" {
			continue
		}

		result = append(result, info)
	}
	return result, nil
}

func memberInfoFromCR(m *choristerv1alpha1.ChoDomainMembership, now time.Time) MemberInfo {
	info := MemberInfo{
		Name:        m.Name,
		Identity:    m.Spec.Identity,
		Role:        m.Spec.Role,
		Domain:      m.Spec.Domain,
		Application: m.Spec.Application,
		Source:      m.Spec.Source,
		ExpiresAt:   m.Spec.ExpiresAt,
		Phase:       m.Status.Phase,
	}
	if info.Phase == "" {
		info.Phase = "Active"
	}
	if info.Source == "" {
		info.Source = "manual"
	}

	if m.Spec.ExpiresAt != nil {
		days := int(m.Spec.ExpiresAt.Time.Sub(now).Hours() / 24)
		info.DaysUntilExpiry = days
		if days < 0 {
			info.Phase = "Expired"
		}
	}

	return info
}
