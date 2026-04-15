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
	"testing"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestListVulnerabilityReports(t *testing.T) {
	s := newScheme()
	now := metav1.Now()

	vr1 := &choristerv1alpha1.ChoVulnerabilityReport{
		ObjectMeta: metav1.ObjectMeta{Name: "vr-payments", Namespace: "default"},
		Spec:       choristerv1alpha1.ChoVulnerabilityReportSpec{Application: "myproduct", Domain: "payments"},
		Status: choristerv1alpha1.ChoVulnerabilityReportStatus{
			Phase: "Completed", Scanner: "trivy", CriticalCount: 2, ScannedAt: &now,
			Findings: []choristerv1alpha1.VulnerabilityFinding{
				{ID: "CVE-2024-001", Severity: "Critical", Image: "api:v1"},
				{ID: "CVE-2024-002", Severity: "Critical", Image: "api:v1"},
				{ID: "CVE-2024-003", Severity: "High", Image: "api:v1"},
			},
		},
	}
	vr2 := &choristerv1alpha1.ChoVulnerabilityReport{
		ObjectMeta: metav1.ObjectMeta{Name: "vr-auth", Namespace: "default"},
		Spec:       choristerv1alpha1.ChoVulnerabilityReportSpec{Application: "myproduct", Domain: "auth"},
		Status: choristerv1alpha1.ChoVulnerabilityReportStatus{
			Phase: "Completed", Scanner: "trivy", CriticalCount: 0, ScannedAt: &now,
			Findings: []choristerv1alpha1.VulnerabilityFinding{
				{ID: "CVE-2024-010", Severity: "High", Image: "auth:v1"},
			},
		},
	}
	vr3 := &choristerv1alpha1.ChoVulnerabilityReport{
		ObjectMeta: metav1.ObjectMeta{Name: "vr-other", Namespace: "default"},
		Spec:       choristerv1alpha1.ChoVulnerabilityReportSpec{Application: "other-app", Domain: "billing"},
		Status: choristerv1alpha1.ChoVulnerabilityReportStatus{
			Phase: "Completed", Scanner: "trivy", CriticalCount: 1, ScannedAt: &now,
		},
	}

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(vr1, vr2, vr3).Build()
	q := NewQuerier(fc)

	// No filter
	all, err := q.ListVulnerabilityReports(context.Background(), VulnFilter{})
	if err != nil {
		t.Fatalf("ListVulnerabilityReports error: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("Expected 3, got %d", len(all))
	}

	// Filter by domain
	payments, err := q.ListVulnerabilityReports(context.Background(), VulnFilter{Domain: "payments"})
	if err != nil {
		t.Fatalf("ListVulnerabilityReports error: %v", err)
	}
	if len(payments) != 1 {
		t.Fatalf("Expected 1 for payments, got %d", len(payments))
	}
	if payments[0].HighCount != 1 {
		t.Errorf("Expected HighCount=1, got %d", payments[0].HighCount)
	}

	// Filter by severity=critical (only reports with critical > 0)
	critical, err := q.ListVulnerabilityReports(context.Background(), VulnFilter{MinSeverity: "critical"})
	if err != nil {
		t.Fatalf("ListVulnerabilityReports error: %v", err)
	}
	if len(critical) != 2 {
		t.Fatalf("Expected 2 with critical findings, got %d", len(critical))
	}
}

func TestGetVulnerabilityReport(t *testing.T) {
	s := newScheme()
	now := metav1.Now()
	vr := &choristerv1alpha1.ChoVulnerabilityReport{
		ObjectMeta: metav1.ObjectMeta{Name: "vr-payments", Namespace: "default"},
		Spec:       choristerv1alpha1.ChoVulnerabilityReportSpec{Application: "myproduct", Domain: "payments"},
		Status: choristerv1alpha1.ChoVulnerabilityReportStatus{
			Phase: "Completed", Scanner: "trivy", CriticalCount: 1, ScannedAt: &now,
			Findings: []choristerv1alpha1.VulnerabilityFinding{
				{ID: "CVE-2024-001", Severity: "Critical", Image: "api:v1", Package: "openssl", FixedVersion: "3.1.1", Title: "Buffer overflow"},
				{ID: "CVE-2024-002", Severity: "High", Image: "api:v1", Package: "curl", FixedVersion: "8.0.1", Title: "SSRF"},
				{ID: "CVE-2024-003", Severity: "High", Image: "worker:v1", Package: "libc", Title: "Memory leak"},
			},
		},
	}

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(vr).Build()
	q := NewQuerier(fc)

	detail, err := q.GetVulnerabilityReport(context.Background(), "myproduct", "payments")
	if err != nil {
		t.Fatalf("GetVulnerabilityReport error: %v", err)
	}
	if len(detail.Findings) != 3 {
		t.Fatalf("Expected 3 findings, got %d", len(detail.Findings))
	}
}

func TestGetVulnerabilityReport_NotFound(t *testing.T) {
	s := newScheme()
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	q := NewQuerier(fc)

	_, err := q.GetVulnerabilityReport(context.Background(), "myproduct", "nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent report")
	}
}
