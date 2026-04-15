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

func TestGetCluster(t *testing.T) {
	s := newScheme()
	cluster := &choristerv1alpha1.ChoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "chorister"},
		Spec: choristerv1alpha1.ChoClusterSpec{
			ControllerRevision: "v1.0.0",
			Operators: &choristerv1alpha1.OperatorVersions{
				Kro:         "v0.2.0",
				StackGres:   "1.12.0",
				CertManager: "v1.14.0",
			},
		},
		Status: choristerv1alpha1.ChoClusterStatus{
			Phase:              "Ready",
			ObservabilityReady: true,
			CISBenchmark:       "Pass",
			OperatorStatus: map[string]string{
				"kro":          "Installed",
				"stackgres":    "Installed",
				"cert-manager": "Degraded",
			},
		},
	}

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(cluster).Build()
	q := NewQuerier(fc)

	got, err := q.GetCluster(context.Background())
	if err != nil {
		t.Fatalf("GetCluster error: %v", err)
	}
	if got.Name != "chorister" {
		t.Errorf("Expected name 'chorister', got %q", got.Name)
	}
	if got.Status.Phase != "Ready" {
		t.Errorf("Expected phase 'Ready', got %q", got.Status.Phase)
	}
}

func TestGetCluster_NotFound(t *testing.T) {
	s := newScheme()
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	q := NewQuerier(fc)

	_, err := q.GetCluster(context.Background())
	if err == nil {
		t.Fatal("Expected error when no ChoCluster exists")
	}
}

func TestGetOperatorDetails(t *testing.T) {
	s := newScheme()
	cluster := &choristerv1alpha1.ChoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "chorister"},
		Spec: choristerv1alpha1.ChoClusterSpec{
			Operators: &choristerv1alpha1.OperatorVersions{
				Kro:         "v0.2.0",
				StackGres:   "1.12.0",
				CertManager: "v1.14.0",
			},
		},
		Status: choristerv1alpha1.ChoClusterStatus{
			Phase: "Ready",
			OperatorStatus: map[string]string{
				"kro":          "Installed",
				"stackgres":    "Degraded",
				"cert-manager": "Installed",
			},
		},
	}

	fc := fake.NewClientBuilder().WithScheme(s).WithObjects(cluster).Build()
	q := NewQuerier(fc)

	infos, err := q.GetOperatorDetails(context.Background())
	if err != nil {
		t.Fatalf("GetOperatorDetails error: %v", err)
	}
	if len(infos) != 3 {
		t.Fatalf("Expected 3 operators, got %d", len(infos))
	}

	// Verify we have degraded operator
	found := false
	for _, info := range infos {
		if info.Name == "stackgres" && info.Status == "Degraded" {
			found = true
		}
	}
	if !found {
		t.Error("Expected to find degraded stackgres operator")
	}
}
