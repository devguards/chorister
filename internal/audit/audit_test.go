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

package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNoopLogger_DiscardEvents(t *testing.T) {
	logger := NewNoopLogger()
	err := logger.Log(context.Background(), Event{
		Action:   "Reconcile",
		Resource: "ChoCompute/test",
		Result:   "success",
	})
	if err != nil {
		t.Fatalf("NoopLogger.Log() returned error: %v", err)
	}
}

func TestMemoryLogger_StoresEvents(t *testing.T) {
	logger := NewMemoryLogger()

	events := []Event{
		{Action: "Reconcile", Resource: "ChoCompute/api", Result: "success"},
		{Action: "Create", Resource: "ChoDatabase/users", Result: "success"},
		{Action: "Delete", Resource: "ChoCache/sessions", Result: "failed"},
	}

	for _, e := range events {
		if err := logger.Log(context.Background(), e); err != nil {
			t.Fatalf("MemoryLogger.Log() returned error: %v", err)
		}
	}

	got := logger.GetEvents()
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d", len(got))
	}

	if got[0].Resource != "ChoCompute/api" {
		t.Errorf("expected first event resource ChoCompute/api, got %s", got[0].Resource)
	}
	if got[2].Result != "failed" {
		t.Errorf("expected third event result failed, got %s", got[2].Result)
	}
}

func TestMemoryLogger_GetEventsReturnsCopy(t *testing.T) {
	logger := NewMemoryLogger()
	_ = logger.Log(context.Background(), Event{Action: "test", Resource: "r", Result: "ok"})

	events1 := logger.GetEvents()
	events2 := logger.GetEvents()

	// Modifying one should not affect the other
	events1[0].Action = "modified"
	if events2[0].Action == "modified" {
		t.Error("GetEvents() should return a copy, not a reference")
	}
}

func TestFailingLogger_AlwaysReturnsError(t *testing.T) {
	expectedErr := fmt.Errorf("audit sink unavailable")
	logger := NewFailingLogger(expectedErr)

	err := logger.Log(context.Background(), Event{
		Action:   "Reconcile",
		Resource: "ChoCluster/main",
		Result:   "started",
	})
	if err == nil {
		t.Fatal("FailingLogger.Log() should return error")
	}
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestLokiLogger_SendsEventToEndpoint(t *testing.T) {
	var receivedBody []byte
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/push" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		receivedContentType = r.Header.Get("Content-Type")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := NewLokiLogger(server.URL)
	event := Event{
		Timestamp:   time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
		Actor:       "controller",
		Action:      "Reconcile",
		Resource:    "ChoCompute/api",
		Namespace:   "payments",
		Application: "saas-product",
		Domain:      "payments",
		Result:      "success",
	}

	err := logger.Log(context.Background(), event)
	if err != nil {
		t.Fatalf("LokiLogger.Log() returned error: %v", err)
	}

	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", receivedContentType)
	}

	// Verify the payload structure
	var payload lokiPushRequest
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("failed to parse Loki payload: %v", err)
	}

	if len(payload.Streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(payload.Streams))
	}

	stream := payload.Streams[0]
	if stream.Stream["job"] != "chorister-audit" {
		t.Errorf("expected job=chorister-audit, got %s", stream.Stream["job"])
	}
	if stream.Stream["resource"] != "ChoCompute/api" {
		t.Errorf("expected resource=ChoCompute/api, got %s", stream.Stream["resource"])
	}

	if len(stream.Values) != 1 {
		t.Fatalf("expected 1 value entry, got %d", len(stream.Values))
	}

	// The value should contain JSON of the event
	var embeddedEvent Event
	if err := json.Unmarshal([]byte(stream.Values[0][1]), &embeddedEvent); err != nil {
		t.Fatalf("failed to parse embedded event JSON: %v", err)
	}
	if embeddedEvent.Action != "Reconcile" {
		t.Errorf("expected Action=Reconcile, got %s", embeddedEvent.Action)
	}
	if embeddedEvent.Application != "saas-product" {
		t.Errorf("expected Application=saas-product, got %s", embeddedEvent.Application)
	}
}

func TestLokiLogger_ReturnsErrorOnHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	logger := NewLokiLogger(server.URL)
	err := logger.Log(context.Background(), Event{
		Action:   "Reconcile",
		Resource: "ChoCluster/main",
		Result:   "started",
	})
	if err == nil {
		t.Fatal("expected error for HTTP 500 response")
	}
}

func TestLokiLogger_ReturnsErrorOnConnectionRefused(t *testing.T) {
	// Use a port that's definitely not listening
	logger := NewLokiLogger("http://127.0.0.1:1")
	err := logger.Log(context.Background(), Event{
		Action:   "Reconcile",
		Resource: "ChoCluster/main",
		Result:   "started",
	})
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}

func TestLokiLogger_Endpoint(t *testing.T) {
	logger := NewLokiLogger("http://loki.monitoring:3100")
	if logger.Endpoint() != "http://loki.monitoring:3100" {
		t.Errorf("expected endpoint http://loki.monitoring:3100, got %s", logger.Endpoint())
	}
}

func TestLokiLogger_RespectsContext(t *testing.T) {
	// Server that delays response longer than client context timeout
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			http.Error(w, "cancelled", http.StatusServiceUnavailable)
		case <-done:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer func() {
		close(done)
		server.Close()
	}()

	logger := NewLokiLogger(server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := logger.Log(ctx, Event{
		Action:   "Reconcile",
		Resource: "test",
		Result:   "started",
	})
	// Should fail due to context deadline or client timeout
	if err == nil {
		t.Fatal("expected error due to context timeout")
	}
}

func TestMemoryLogger_ConcurrentAccess(t *testing.T) {
	logger := NewMemoryLogger()
	done := make(chan struct{})

	// Write concurrently
	for i := range 100 {
		go func(n int) {
			_ = logger.Log(context.Background(), Event{
				Action:   "test",
				Resource: fmt.Sprintf("resource-%d", n),
				Result:   "ok",
			})
			done <- struct{}{}
		}(i)
	}

	for range 100 {
		<-done
	}

	events := logger.GetEvents()
	if len(events) != 100 {
		t.Errorf("expected 100 events, got %d", len(events))
	}
}
