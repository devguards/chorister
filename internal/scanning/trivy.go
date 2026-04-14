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

package scanning

import (
	"context"
	"strings"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

type Scanner interface {
	ScanImages(ctx context.Context, images []string) (ScanResult, error)
}

type ScanResult struct {
	Scanner       string
	CriticalCount int
	Findings      []choristerv1alpha1.VulnerabilityFinding
}

type SignatureScanner struct{}

func NewDefaultScanner() Scanner {
	return SignatureScanner{}
}

func (SignatureScanner) ScanImages(_ context.Context, images []string) (ScanResult, error) {
	result := ScanResult{Scanner: "signature-scanner"}
	for _, image := range images {
		lower := strings.ToLower(image)
		switch {
		case strings.Contains(lower, "critical"), strings.Contains(lower, "cve"), strings.Contains(lower, "vuln"):
			result.CriticalCount++
			result.Findings = append(result.Findings, choristerv1alpha1.VulnerabilityFinding{
				Image:    image,
				ID:       "SIMULATED-CRITICAL-CVE",
				Severity: "Critical",
				Package:  "application",
				Title:    "Image matched critical vulnerability signature",
			})
		case strings.Contains(lower, "high"):
			result.Findings = append(result.Findings, choristerv1alpha1.VulnerabilityFinding{
				Image:    image,
				ID:       "SIMULATED-HIGH-CVE",
				Severity: "High",
				Package:  "application",
				Title:    "Image matched high vulnerability signature",
			})
		}
	}
	return result, nil
}
