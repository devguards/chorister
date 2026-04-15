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
)

// VulnFilter defines filtering criteria for vulnerability reports.
type VulnFilter struct {
	App         string
	Domain      string
	MinSeverity string
}

// VulnReportInfo provides summary info about a vulnerability report.
type VulnReportInfo struct {
	Name          string
	Domain        string
	Application   string
	Namespace     string
	Scanner       string
	CriticalCount int
	HighCount     int
	ScannedAt     *time.Time
	Phase         string
}

// VulnReportDetail extends VulnReportInfo with individual findings.
type VulnReportDetail struct {
	VulnReportInfo
	Findings []choristerv1alpha1.VulnerabilityFinding
}

// ListVulnerabilityReports returns vulnerability reports matching the given filters.
func (q *Querier) ListVulnerabilityReports(ctx context.Context, filters VulnFilter) ([]VulnReportInfo, error) {
	var list choristerv1alpha1.ChoVulnerabilityReportList
	if err := q.list(ctx, &list); err != nil {
		return nil, wrapError("ChoVulnerabilityReport", "", "", err)
	}

	var result []VulnReportInfo
	for _, vr := range list.Items {
		if filters.App != "" && vr.Spec.Application != filters.App {
			continue
		}
		if filters.Domain != "" && vr.Spec.Domain != filters.Domain {
			continue
		}

		info := vulnInfoFromCR(&vr)

		// Filter by minimum severity
		if filters.MinSeverity == "critical" && info.CriticalCount == 0 {
			continue
		}

		result = append(result, info)
	}
	return result, nil
}

// GetVulnerabilityReport returns a detailed vulnerability report for a given domain namespace.
func (q *Querier) GetVulnerabilityReport(ctx context.Context, appName, domainName string) (*VulnReportDetail, error) {
	var list choristerv1alpha1.ChoVulnerabilityReportList
	if err := q.list(ctx, &list); err != nil {
		return nil, wrapError("ChoVulnerabilityReport", "", "", err)
	}

	for _, vr := range list.Items {
		if vr.Spec.Application == appName && vr.Spec.Domain == domainName {
			info := vulnInfoFromCR(&vr)
			return &VulnReportDetail{
				VulnReportInfo: info,
				Findings:       vr.Status.Findings,
			}, nil
		}
	}

	return nil, fmt.Errorf("ChoVulnerabilityReport: not found for app=%s domain=%s", appName, domainName)
}

func vulnInfoFromCR(vr *choristerv1alpha1.ChoVulnerabilityReport) VulnReportInfo {
	info := VulnReportInfo{
		Name:          vr.Name,
		Domain:        vr.Spec.Domain,
		Application:   vr.Spec.Application,
		Namespace:     vr.Namespace,
		Scanner:       vr.Status.Scanner,
		CriticalCount: vr.Status.CriticalCount,
		Phase:         vr.Status.Phase,
	}
	if info.Phase == "" {
		info.Phase = "Pending"
	}
	if vr.Status.ScannedAt != nil {
		t := vr.Status.ScannedAt.Time
		info.ScannedAt = &t
	}

	// Count high severity findings
	for _, f := range vr.Status.Findings {
		if f.Severity == "High" {
			info.HighCount++
		}
	}

	return info
}
