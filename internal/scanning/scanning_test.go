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
	"testing"
)

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
