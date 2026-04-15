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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestListChoristerEvents(t *testing.T) {
	s := newScheme()
	// Register core/v1 types
	_ = corev1.AddToScheme(s)

	now := metav1.Now()
	old := metav1.NewTime(time.Now().Add(-2 * time.Hour))

	events := []runtime.Object{
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "evt-1", Namespace: "myproduct-payments"},
			InvolvedObject: corev1.ObjectReference{Kind: "ChoCompute", Name: "api"},
			Reason:         "Reconciled",
			Message:        "Successfully reconciled",
			Type:           "Normal",
			LastTimestamp:  now,
		},
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "evt-2", Namespace: "myproduct-payments"},
			InvolvedObject: corev1.ObjectReference{Kind: "ChoDatabase", Name: "ledger"},
			Reason:         "FailedCreate",
			Message:        "Failed to create pod",
			Type:           "Warning",
			LastTimestamp:  now,
		},
		&corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: "evt-3", Namespace: "myproduct-payments"},
			InvolvedObject: corev1.ObjectReference{Kind: "ChoCompute", Name: "worker"},
			Reason:         "Reconciled",
			Message:        "Old event",
			Type:           "Normal",
			LastTimestamp:  old,
		},
	}

	fc := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(events...).Build()
	q := NewQuerier(fc)

	// Events from last hour
	results, err := q.ListChoristerEvents(context.Background(), "myproduct-payments", 1*time.Hour, 100)
	if err != nil {
		t.Fatalf("ListChoristerEvents error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Expected 2 events within last hour, got %d", len(results))
	}

	// With limit
	limited, err := q.ListChoristerEvents(context.Background(), "myproduct-payments", 24*time.Hour, 1)
	if err != nil {
		t.Fatalf("ListChoristerEvents error: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("Expected 1 event with limit=1, got %d", len(limited))
	}
}
