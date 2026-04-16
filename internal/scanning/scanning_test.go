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
	"testing"
)

// mockRunner is a test double for CommandRunner.
type mockRunner struct {
	output []byte
	err    error
}

func (m *mockRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return m.output, m.err
}

func TestSignatureScanner_NoVulnerabilities(t *testing.T) {
	scanner := NewDefaultScanner()
	result, err := scanner.ScanImages(context.Background(), []string{
		"myapp/api:v1.0.0",
		"myapp/worker:v2.3.1",
		"nginx:1.25",
	})
	if err != nil {
		t.Fatalf("ScanImages returned error: %v", err)
	}
	if result.CriticalCount != 0 {
		t.Errorf("expected 0 critical findings, got %d", result.CriticalCount)
	}
	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result.Findings))
	}
	if result.Scanner != "signature-scanner" {
		t.Errorf("expected scanner name signature-scanner, got %s", result.Scanner)
	}
}

func TestSignatureScanner_DetectsCriticalImages(t *testing.T) {
	scanner := NewDefaultScanner()
	result, err := scanner.ScanImages(context.Background(), []string{
		"myapp/api:v1.0.0",
		"vulnerable-critical-image:latest",
	})
	if err != nil {
		t.Fatalf("ScanImages returned error: %v", err)
	}
	if result.CriticalCount != 1 {
		t.Errorf("expected 1 critical finding, got %d", result.CriticalCount)
	}
	if len(result.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].Severity != "Critical" {
		t.Errorf("expected severity Critical, got %s", result.Findings[0].Severity)
	}
	if result.Findings[0].Image != "vulnerable-critical-image:latest" {
		t.Errorf("expected image vulnerable-critical-image:latest, got %s", result.Findings[0].Image)
	}
}

func TestSignatureScanner_DetectsHighImages(t *testing.T) {
	scanner := NewDefaultScanner()
	result, err := scanner.ScanImages(context.Background(), []string{
		"some-high-risk-image:v1",
	})
	if err != nil {
		t.Fatalf("ScanImages returned error: %v", err)
	}
	if result.CriticalCount != 0 {
		t.Errorf("expected 0 critical findings for high-only, got %d", result.CriticalCount)
	}
	if len(result.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].Severity != "High" {
		t.Errorf("expected severity High, got %s", result.Findings[0].Severity)
	}
}

func TestSignatureScanner_DetectsCVEInImageName(t *testing.T) {
	scanner := NewDefaultScanner()
	result, err := scanner.ScanImages(context.Background(), []string{
		"registry.example.com/app-with-cve-fix:v1",
	})
	if err != nil {
		t.Fatalf("ScanImages returned error: %v", err)
	}
	if result.CriticalCount != 1 {
		t.Errorf("expected 1 critical (cve in name), got %d", result.CriticalCount)
	}
}

func TestSignatureScanner_MultipleMixedImages(t *testing.T) {
	scanner := NewDefaultScanner()
	result, err := scanner.ScanImages(context.Background(), []string{
		"clean-app:v1.0",
		"critical-vuln-app:latest",
		"high-risk-lib:v2",
		"another-clean:v3",
		"cve-patched:v1.1",
	})
	if err != nil {
		t.Fatalf("ScanImages returned error: %v", err)
	}
	if result.CriticalCount != 2 {
		t.Errorf("expected 2 critical findings, got %d", result.CriticalCount)
	}
	// 2 critical + 1 high
	if len(result.Findings) != 3 {
		t.Errorf("expected 3 total findings, got %d", len(result.Findings))
	}
}

func TestSignatureScanner_EmptyImageList(t *testing.T) {
	scanner := NewDefaultScanner()
	result, err := scanner.ScanImages(context.Background(), []string{})
	if err != nil {
		t.Fatalf("ScanImages returned error: %v", err)
	}
	if result.CriticalCount != 0 {
		t.Errorf("expected 0 critical for empty list, got %d", result.CriticalCount)
	}
	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings for empty list, got %d", len(result.Findings))
	}
}

func TestSignatureScanner_CaseInsensitive(t *testing.T) {
	scanner := NewDefaultScanner()
	result, err := scanner.ScanImages(context.Background(), []string{
		"app-CRITICAL-issue:v1",
		"HIGH-severity-lib:v2",
	})
	if err != nil {
		t.Fatalf("ScanImages returned error: %v", err)
	}
	if result.CriticalCount != 1 {
		t.Errorf("expected 1 critical (case insensitive), got %d", result.CriticalCount)
	}
	if len(result.Findings) != 2 {
		t.Errorf("expected 2 findings (case insensitive), got %d", len(result.Findings))
	}
}

func TestNewDefaultScanner_ImplementsInterface(t *testing.T) {
	var _ Scanner = NewDefaultScanner()
}

// ---------------------------------------------------------------------------
// TrivyScanner tests (using mockRunner)
// ---------------------------------------------------------------------------

func trivyJSON(results []trivyResult) []byte {
	report := trivyReport{Results: results}
	b, _ := json.Marshal(report)
	return b
}

func TestTrivyScanner_NoVulnerabilities(t *testing.T) {
	scanner := &TrivyScanner{Runner: &mockRunner{output: trivyJSON(nil)}}
	result, err := scanner.ScanImages(context.Background(), []string{"clean:v1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Scanner != "trivy" {
		t.Errorf("expected scanner name trivy, got %s", result.Scanner)
	}
	if result.CriticalCount != 0 {
		t.Errorf("expected 0 critical, got %d", result.CriticalCount)
	}
	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result.Findings))
	}
}

func TestTrivyScanner_ParsesCriticalAndHigh(t *testing.T) {
	out := trivyJSON([]trivyResult{{
		Vulnerabilities: []trivyVulnerability{
			{VulnerabilityID: "CVE-2024-1234", PkgName: "libssl", Severity: "CRITICAL", Title: "OpenSSL RCE", FixedVersion: "3.0.14"},
			{VulnerabilityID: "CVE-2024-5678", PkgName: "curl", Severity: "HIGH", Title: "Curl HSTS bypass"},
		},
	}})
	scanner := &TrivyScanner{Runner: &mockRunner{output: out}}
	result, err := scanner.ScanImages(context.Background(), []string{"myapp:v1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CriticalCount != 1 {
		t.Errorf("expected 1 critical, got %d", result.CriticalCount)
	}
	if len(result.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(result.Findings))
	}
	if result.Findings[0].ID != "CVE-2024-1234" {
		t.Errorf("expected CVE-2024-1234, got %s", result.Findings[0].ID)
	}
	if result.Findings[0].Severity != "Critical" {
		t.Errorf("expected normalized Critical, got %s", result.Findings[0].Severity)
	}
	if result.Findings[0].FixedVersion != "3.0.14" {
		t.Errorf("expected fixed version 3.0.14, got %s", result.Findings[0].FixedVersion)
	}
	if result.Findings[1].Severity != "HIGH" {
		t.Errorf("expected HIGH, got %s", result.Findings[1].Severity)
	}
}

func TestTrivyScanner_MultipleImages(t *testing.T) {
	out := trivyJSON([]trivyResult{{
		Vulnerabilities: []trivyVulnerability{
			{VulnerabilityID: "CVE-2024-0001", PkgName: "zlib", Severity: "CRITICAL", Title: "zlib overflow"},
		},
	}})
	scanner := &TrivyScanner{Runner: &mockRunner{output: out}}
	result, err := scanner.ScanImages(context.Background(), []string{"img1:v1", "img2:v2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Each image returns the same mock output: 1 critical each
	if result.CriticalCount != 2 {
		t.Errorf("expected 2 critical across 2 images, got %d", result.CriticalCount)
	}
	if len(result.Findings) != 2 {
		t.Errorf("expected 2 findings, got %d", len(result.Findings))
	}
	if result.Findings[0].Image != "img1:v1" {
		t.Errorf("expected image img1:v1, got %s", result.Findings[0].Image)
	}
	if result.Findings[1].Image != "img2:v2" {
		t.Errorf("expected image img2:v2, got %s", result.Findings[1].Image)
	}
}

func TestTrivyScanner_ExecError(t *testing.T) {
	scanner := &TrivyScanner{Runner: &mockRunner{err: fmt.Errorf("trivy not found")}}
	_, err := scanner.ScanImages(context.Background(), []string{"myapp:v1"})
	if err == nil {
		t.Fatal("expected error when trivy exec fails")
	}
}

func TestTrivyScanner_InvalidJSON(t *testing.T) {
	scanner := &TrivyScanner{Runner: &mockRunner{output: []byte("not json")}}
	_, err := scanner.ScanImages(context.Background(), []string{"myapp:v1"})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestTrivyScanner_ServerMode(t *testing.T) {
	// Verify that server URL is accepted (we can't verify the args without
	// inspecting the mock, but we ensure it doesn't error).
	out := trivyJSON(nil)
	scanner := NewTrivyScanner("http://trivy.internal:4954")
	scanner.Runner = &mockRunner{output: out}
	result, err := scanner.ScanImages(context.Background(), []string{"clean:v1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CriticalCount != 0 {
		t.Errorf("expected 0 critical, got %d", result.CriticalCount)
	}
}

func TestTrivyScanner_ImplementsInterface(t *testing.T) {
	var _ Scanner = &TrivyScanner{}
}
