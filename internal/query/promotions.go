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
	"strings"
	"time"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

// PromotionFilter defines filtering criteria for promotion requests.
type PromotionFilter struct {
	App    string
	Domain string
	Status string
}

// PromotionInfo provides summary info about a promotion request.
type PromotionInfo struct {
	Name              string
	Domain            string
	Application       string
	Sandbox           string
	RequestedBy       string
	Phase             string
	CreatedAt         time.Time
	ApprovalCount     int
	RequiredApprovals int
	Diff              string
	ExternalRef       string
}

// ListPromotionRequests returns promotion requests matching the given filters.
func (q *Querier) ListPromotionRequests(ctx context.Context, filters PromotionFilter) ([]PromotionInfo, error) {
	var list choristerv1alpha1.ChoPromotionRequestList
	if err := q.list(ctx, &list); err != nil {
		return nil, wrapError("ChoPromotionRequest", "", "", err)
	}

	var result []PromotionInfo
	for _, pr := range list.Items {
		if filters.App != "" && pr.Spec.Application != filters.App {
			continue
		}
		if filters.Domain != "" && pr.Spec.Domain != filters.Domain {
			continue
		}
		phase := pr.Status.Phase
		if phase == "" {
			phase = "Pending"
		}
		if filters.Status != "" && filters.Status != "all" && !strings.EqualFold(phase, filters.Status) {
			continue
		}

		result = append(result, PromotionInfo{
			Name:          pr.Name,
			Domain:        pr.Spec.Domain,
			Application:   pr.Spec.Application,
			Sandbox:       pr.Spec.Sandbox,
			RequestedBy:   pr.Spec.RequestedBy,
			Phase:         phase,
			CreatedAt:     pr.CreationTimestamp.Time,
			ApprovalCount: len(pr.Status.Approvals),
			Diff:          pr.Spec.Diff,
			ExternalRef:   pr.Spec.ExternalRef,
		})
	}
	return result, nil
}
