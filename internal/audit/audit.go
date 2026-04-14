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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Event represents a structured audit event.
type Event struct {
	Timestamp   time.Time         `json:"timestamp"`
	Actor       string            `json:"actor,omitempty"`
	Action      string            `json:"action"`
	Resource    string            `json:"resource"`
	Namespace   string            `json:"namespace,omitempty"`
	Application string            `json:"application,omitempty"`
	Domain      string            `json:"domain,omitempty"`
	Result      string            `json:"result"`
	Details     map[string]string `json:"details,omitempty"`
}

// Logger defines the interface for audit event logging.
type Logger interface {
	Log(ctx context.Context, event Event) error
}

// --- NoopLogger ---

// NoopLogger discards all audit events. Used when audit logging is not configured.
type NoopLogger struct{}

// NewNoopLogger creates a NoopLogger.
func NewNoopLogger() *NoopLogger {
	return &NoopLogger{}
}

// Log discards the event.
func (l *NoopLogger) Log(_ context.Context, _ Event) error {
	return nil
}

// --- LokiLogger ---

// LokiLogger sends structured audit events to a Loki push API endpoint.
type LokiLogger struct {
	endpoint string
	client   *http.Client
}

// NewLokiLogger creates a LokiLogger targeting the given Loki base URL.
func NewLokiLogger(endpoint string) *LokiLogger {
	return &LokiLogger{
		endpoint: endpoint,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

type lokiPushRequest struct {
	Streams []lokiStream `json:"streams"`
}

type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// Log sends the audit event to Loki synchronously.
func (l *LokiLogger) Log(ctx context.Context, event Event) error {
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}

	ts := event.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	payload := lokiPushRequest{
		Streams: []lokiStream{
			{
				Stream: map[string]string{
					"job":      "chorister-audit",
					"level":    "info",
					"resource": event.Resource,
					"action":   event.Action,
				},
				Values: [][]string{
					{strconv.FormatInt(ts.UnixNano(), 10), string(eventJSON)},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal loki payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.endpoint+"/loki/api/v1/push", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create loki request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return fmt.Errorf("send audit event to Loki: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("loki returned status %d", resp.StatusCode)
	}

	return nil
}

// --- MemoryLogger ---

// MemoryLogger stores audit events in memory. Use in tests.
type MemoryLogger struct {
	mu     sync.Mutex
	Events []Event
}

// NewMemoryLogger creates a MemoryLogger.
func NewMemoryLogger() *MemoryLogger {
	return &MemoryLogger{}
}

// Log stores the event in memory.
func (l *MemoryLogger) Log(_ context.Context, event Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Events = append(l.Events, event)
	return nil
}

// GetEvents returns a copy of all recorded events.
func (l *MemoryLogger) GetEvents() []Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Event, len(l.Events))
	copy(out, l.Events)
	return out
}

// --- FailingLogger ---

// FailingLogger always returns an error. Use in tests to verify audit failure handling.
type FailingLogger struct {
	Err error
}

// NewFailingLogger creates a FailingLogger that always returns the given error.
func NewFailingLogger(err error) *FailingLogger {
	return &FailingLogger{Err: err}
}

// Log always returns the configured error.
func (l *FailingLogger) Log(_ context.Context, _ Event) error {
	return l.Err
}
