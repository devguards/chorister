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
	"fmt"
	"time"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SandboxInfo provides summary info about a sandbox.
type SandboxInfo struct {
	Name                 string
	Owner                string
	Domain               string
	Application          string
	Namespace            string
	Phase                string
	Age                  time.Time
	LastApplyTime        *metav1.Time
	EstimatedMonthlyCost string
	IdleWarning          bool
}

// SandboxDetail extends SandboxInfo with full resource details.
type SandboxDetail struct {
	SandboxInfo
	Resources  *DomainResources
	Conditions []metav1.Condition
}

// ListSandboxesByDomain returns sandboxes for a given app + domain.
func (q *Querier) ListSandboxesByDomain(ctx context.Context, appName, domainName string) ([]SandboxInfo, error) {
	var list choristerv1alpha1.ChoSandboxList
	if err := q.list(ctx, &list); err != nil {
		return nil, wrapError("ChoSandbox", "", "", err)
	}

	var result []SandboxInfo
	for _, sb := range list.Items {
		if sb.Spec.Application != appName || sb.Spec.Domain != domainName {
			continue
		}
		result = append(result, sandboxInfoFromCR(&sb))
	}
	return result, nil
}

// ListAllSandboxes returns all sandboxes, optionally filtered by app.
func (q *Querier) ListAllSandboxes(ctx context.Context, appFilter string) ([]SandboxInfo, error) {
	var list choristerv1alpha1.ChoSandboxList
	opts := []client.ListOption{}
	if err := q.list(ctx, &list, opts...); err != nil {
		return nil, wrapError("ChoSandbox", "", "", err)
	}

	var result []SandboxInfo
	for _, sb := range list.Items {
		if appFilter != "" && sb.Spec.Application != appFilter {
			continue
		}
		result = append(result, sandboxInfoFromCR(&sb))
	}
	return result, nil
}

// GetSandbox returns detailed sandbox info including resources.
func (q *Querier) GetSandbox(ctx context.Context, appName, domainName, sandboxName string) (*SandboxDetail, error) {
	var list choristerv1alpha1.ChoSandboxList
	if err := q.list(ctx, &list); err != nil {
		return nil, wrapError("ChoSandbox", sandboxName, "", err)
	}

	for _, sb := range list.Items {
		if sb.Spec.Application == appName && sb.Spec.Domain == domainName && sb.Spec.Name == sandboxName {
			info := sandboxInfoFromCR(&sb)
			detail := &SandboxDetail{
				SandboxInfo: info,
				Conditions:  sb.Status.Conditions,
			}

			if sb.Status.Namespace != "" {
				resources, err := q.ListDomainResources(ctx, sb.Status.Namespace)
				if err != nil {
					return nil, err
				}
				detail.Resources = resources
			} else {
				detail.Resources = &DomainResources{}
			}

			return detail, nil
		}
	}

	return nil, wrapError("ChoSandbox", sandboxName, "", fmt.Errorf("not found in app=%s domain=%s", appName, domainName))
}

func sandboxInfoFromCR(sb *choristerv1alpha1.ChoSandbox) SandboxInfo {
	info := SandboxInfo{
		Name:                 sb.Spec.Name,
		Owner:                sb.Spec.Owner,
		Domain:               sb.Spec.Domain,
		Application:          sb.Spec.Application,
		Namespace:            sb.Status.Namespace,
		Phase:                sb.Status.Phase,
		Age:                  sb.CreationTimestamp.Time,
		LastApplyTime:        sb.Status.LastApplyTime,
		EstimatedMonthlyCost: sb.Status.EstimatedMonthlyCost,
	}

	if info.Phase == "" {
		info.Phase = "Pending"
	}

	// Mark as idle if no apply in 7 days
	if sb.Status.LastApplyTime != nil {
		if time.Since(sb.Status.LastApplyTime.Time) > 7*24*time.Hour {
			info.IdleWarning = true
		}
	}

	return info
}
