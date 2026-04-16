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
	"encoding/json"
	"fmt"
	"os/exec"
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

// SignatureScanner is a simulated scanner for testing and development.
// It matches vulnerability keywords in image names instead of performing real scans.
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

// CommandRunner abstracts command execution for testing.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// TrivyScanner shells out to the trivy CLI to scan container images.
type TrivyScanner struct {
	// ServerURL is the optional Trivy server URL for client-server mode.
	// If empty, trivy runs in standalone mode.
	ServerURL string
	// Runner abstracts command execution (nil uses real exec).
	Runner CommandRunner
}

// NewTrivyScanner creates a scanner that invokes `trivy image`.
func NewTrivyScanner(serverURL string) *TrivyScanner {
	return &TrivyScanner{ServerURL: serverURL}
}

func (t *TrivyScanner) runner() CommandRunner {
	if t.Runner != nil {
		return t.Runner
	}
	return execRunner{}
}

func (t *TrivyScanner) ScanImages(ctx context.Context, images []string) (ScanResult, error) {
	result := ScanResult{Scanner: "trivy"}
	for _, image := range images {
		findings, err := t.scanOneImage(ctx, image)
		if err != nil {
			return result, fmt.Errorf("trivy scan %s: %w", image, err)
		}
		for i := range findings {
			if findings[i].Severity == "CRITICAL" {
				findings[i].Severity = "Critical"
				result.CriticalCount++
			}
		}
		result.Findings = append(result.Findings, findings...)
	}
	return result, nil
}

func (t *TrivyScanner) scanOneImage(ctx context.Context, image string) ([]choristerv1alpha1.VulnerabilityFinding, error) {
	args := []string{"image", "--format", "json", "--severity", "CRITICAL,HIGH", "--quiet"}
	if t.ServerURL != "" {
		args = append(args, "--server", t.ServerURL)
	}
	args = append(args, image)

	out, err := t.runner().Run(ctx, "trivy", args...)
	if err != nil {
		return nil, fmt.Errorf("exec trivy: %w", err)
	}

	var report trivyReport
	if err := json.Unmarshal(out, &report); err != nil {
		return nil, fmt.Errorf("parse trivy output: %w", err)
	}

	var findings []choristerv1alpha1.VulnerabilityFinding
	for _, r := range report.Results {
		for _, v := range r.Vulnerabilities {
			findings = append(findings, choristerv1alpha1.VulnerabilityFinding{
				Image:        image,
				ID:           v.VulnerabilityID,
				Severity:     v.Severity,
				Package:      v.PkgName,
				FixedVersion: v.FixedVersion,
				Title:        v.Title,
			})
		}
	}
	return findings, nil
}

// trivyReport is the subset of Trivy JSON output we parse.
type trivyReport struct {
	Results []trivyResult `json:"Results"`
}

type trivyResult struct {
	Vulnerabilities []trivyVulnerability `json:"Vulnerabilities"`
}

type trivyVulnerability struct {
	VulnerabilityID string `json:"VulnerabilityID"`
	PkgName         string `json:"PkgName"`
	Severity        string `json:"Severity"`
	Title           string `json:"Title"`
	FixedVersion    string `json:"FixedVersion"`
}
