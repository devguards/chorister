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

package report

import (
	"context"
	"fmt"

	"github.com/chorister-dev/chorister/internal/query"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceUsage holds used/limit pair for a resource dimension.
type ResourceUsage struct {
	Used  string
	Limit string
}

// DomainQuotaUsage holds quota utilization for a domain namespace.
type DomainQuotaUsage struct {
	Name      string
	Namespace string
	CPU       ResourceUsage
	Memory    ResourceUsage
	Storage   ResourceUsage
	PodCount  ResourceUsage
}

// QuotaResult holds quota utilization across all domains.
type QuotaResult struct {
	AppName string
	Domains []DomainQuotaUsage
}

// QuotaReport computes resource quota utilization for each domain in the application.
// It reads ResourceQuota objects from each domain namespace to get used/limit values.
func QuotaReport(ctx context.Context, q *query.Querier, appName string, domainFilter string) (*QuotaResult, error) {
	c := q.Client()
	domains, err := q.ListDomainsByApp(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}

	result := &QuotaResult{AppName: appName}

	for _, d := range domains {
		if domainFilter != "" && d.Name != domainFilter {
			continue
		}
		if d.Namespace == "" {
			result.Domains = append(result.Domains, DomainQuotaUsage{
				Name:      d.Name,
				Namespace: "(not created)",
			})
			continue
		}

		usage, err := namespaceQuotaUsage(ctx, c, d.Name, d.Namespace)
		if err != nil {
			// Non-fatal: include entry with unknown values
			usage = DomainQuotaUsage{
				Name:      d.Name,
				Namespace: d.Namespace,
				CPU:       ResourceUsage{Used: "?", Limit: "?"},
				Memory:    ResourceUsage{Used: "?", Limit: "?"},
				Storage:   ResourceUsage{Used: "?", Limit: "?"},
				PodCount:  ResourceUsage{Used: "?", Limit: "?"},
			}
		}
		result.Domains = append(result.Domains, usage)
	}

	return result, nil
}

// namespaceQuotaUsage reads ResourceQuota objects in a namespace and returns aggregated usage.
func namespaceQuotaUsage(ctx context.Context, c client.Client, domainName, namespace string) (DomainQuotaUsage, error) {
	var quotaList corev1.ResourceQuotaList
	if err := c.List(ctx, &quotaList, client.InNamespace(namespace)); err != nil {
		return DomainQuotaUsage{}, fmt.Errorf("list ResourceQuota in %s: %w", namespace, err)
	}

	usage := DomainQuotaUsage{
		Name:      domainName,
		Namespace: namespace,
	}

	if len(quotaList.Items) == 0 {
		usage.CPU = ResourceUsage{Used: "-", Limit: "(no quota)"}
		usage.Memory = ResourceUsage{Used: "-", Limit: "(no quota)"}
		usage.Storage = ResourceUsage{Used: "-", Limit: "(no quota)"}
		usage.PodCount = ResourceUsage{Used: "-", Limit: "(no quota)"}
		return usage, nil
	}

	// Aggregate from first matching quota (chorister creates one per namespace)
	q := &quotaList.Items[0]
	usage.CPU = ResourceUsage{
		Used:  quantityStr(q.Status.Used, corev1.ResourceCPU),
		Limit: quantityStr(q.Status.Hard, corev1.ResourceLimitsCPU),
	}
	if usage.CPU.Limit == "-" {
		usage.CPU.Limit = quantityStr(q.Status.Hard, corev1.ResourceCPU)
	}
	usage.Memory = ResourceUsage{
		Used:  quantityStr(q.Status.Used, corev1.ResourceMemory),
		Limit: quantityStr(q.Status.Hard, corev1.ResourceLimitsMemory),
	}
	if usage.Memory.Limit == "-" {
		usage.Memory.Limit = quantityStr(q.Status.Hard, corev1.ResourceMemory)
	}
	usage.Storage = ResourceUsage{
		Used:  quantityStr(q.Status.Used, corev1.ResourceRequestsStorage),
		Limit: quantityStr(q.Status.Hard, corev1.ResourceRequestsStorage),
	}
	usage.PodCount = ResourceUsage{
		Used:  quantityStr(q.Status.Used, corev1.ResourcePods),
		Limit: quantityStr(q.Status.Hard, corev1.ResourcePods),
	}

	return usage, nil
}

// quantityStr returns the string representation of a resource quantity, or "-" if absent.
func quantityStr(rl corev1.ResourceList, name corev1.ResourceName) string {
	if v, ok := rl[name]; ok {
		return v.String()
	}
	return "-"
}

// QuotaTableReport produces a quota utilization table.
// Columns: DOMAIN, NAMESPACE, CPU-USED/LIMIT, MEM-USED/LIMIT, STORAGE-USED/LIMIT, PODS-USED/LIMIT
func QuotaTableReport(result *QuotaResult) TableData {
	td := TableData{
		Headers: []string{"DOMAIN", "NAMESPACE", "CPU", "MEMORY", "STORAGE", "PODS"},
		Rows:    make([][]string, 0, len(result.Domains)),
	}
	for _, d := range result.Domains {
		td.Rows = append(td.Rows, []string{
			d.Name,
			d.Namespace,
			d.CPU.Used + "/" + d.CPU.Limit,
			d.Memory.Used + "/" + d.Memory.Limit,
			d.Storage.Used + "/" + d.Storage.Limit,
			d.PodCount.Used + "/" + d.PodCount.Limit,
		})
	}
	return td
}
