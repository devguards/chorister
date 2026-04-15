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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AuditFilter defines filtering criteria for audit log queries.
type AuditFilter struct {
	Domain string
	Action string
	Actor  string
	Since  time.Duration
}

// AuditEntry represents a single structured audit event from Loki.
type AuditEntry struct {
	Timestamp   time.Time
	Actor       string
	Action      string
	Resource    string
	Namespace   string
	Application string
	Domain      string
	Result      string
	Details     string
}

// lokiQueryResponse represents the Loki HTTP query_range response.
type lokiQueryResponse struct {
	Data struct {
		Result []struct {
			Values [][]string `json:"values"` // [[timestamp_ns, log_line], ...]
		} `json:"result"`
	} `json:"data"`
}

// QueryAuditLog queries audit events from Loki. lokiURL is the base Loki HTTP URL
// (e.g. "http://loki.cho-monitoring.svc.cluster.local:3100"). If lokiURL is empty
// or Loki is unreachable, an error is returned with remediation instructions.
func (q *Querier) QueryAuditLog(ctx context.Context, lokiURL string, filters AuditFilter) ([]AuditEntry, error) {
	if lokiURL == "" {
		return nil, fmt.Errorf("Loki URL not configured: set CHORISTER_LOKI_URL or ensure ChoCluster observability is enabled. Check: kubectl port-forward -n cho-monitoring svc/loki 3100:3100")
	}

	since := filters.Since
	if since == 0 {
		since = 24 * time.Hour
	}

	// Build LogQL query
	logql := `{job="chorister-audit"}`
	if filters.Domain != "" {
		logql = fmt.Sprintf(`{job="chorister-audit",domain=%q}`, filters.Domain)
	}
	if filters.Actor != "" {
		logql += fmt.Sprintf(` | json | actor=%q`, filters.Actor)
	}
	if filters.Action != "" {
		logql += fmt.Sprintf(` | json | action=%q`, filters.Action)
	}

	start := time.Now().Add(-since).UnixNano()
	end := time.Now().UnixNano()

	endpoint := fmt.Sprintf("%s/loki/api/v1/query_range", lokiURL)
	params := url.Values{
		"query": []string{logql},
		"start": []string{fmt.Sprintf("%d", start)},
		"end":   []string{fmt.Sprintf("%d", end)},
		"limit": []string{"500"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build Loki request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Loki unreachable at %s: %w\nTip: kubectl port-forward -n cho-monitoring svc/loki 3100:3100", lokiURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Loki returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var lokiResp lokiQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&lokiResp); err != nil {
		return nil, fmt.Errorf("decode Loki response: %w", err)
	}

	var entries []AuditEntry
	for _, stream := range lokiResp.Data.Result {
		for _, val := range stream.Values {
			if len(val) < 2 {
				continue
			}
			entry := parseAuditLogLine(val[1])
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// parseAuditLogLine parses a JSON audit log line into an AuditEntry.
// Falls back to a raw entry if the line is not valid JSON.
func parseAuditLogLine(line string) AuditEntry {
	var raw struct {
		Timestamp   string `json:"timestamp"`
		Actor       string `json:"actor"`
		Action      string `json:"action"`
		Resource    string `json:"resource"`
		Namespace   string `json:"namespace"`
		Application string `json:"application"`
		Domain      string `json:"domain"`
		Result      string `json:"result"`
		Details     string `json:"details"`
	}
	entry := AuditEntry{Timestamp: time.Now()}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		entry.Action = line
		return entry
	}
	if t, err := time.Parse(time.RFC3339Nano, raw.Timestamp); err == nil {
		entry.Timestamp = t
	}
	entry.Actor = raw.Actor
	entry.Action = raw.Action
	entry.Resource = raw.Resource
	entry.Namespace = raw.Namespace
	entry.Application = raw.Application
	entry.Domain = raw.Domain
	entry.Result = raw.Result
	entry.Details = raw.Details
	return entry
}

// EventInfo summarises a Kubernetes event.
type EventInfo struct {
	Time           time.Time
	Type           string
	Reason         string
	Message        string
	InvolvedObject string // "Kind/Name"
}

// ListChoristerEvents returns events from the given namespace within the since duration, limited to limit results.
// It tries events.k8s.io/v1 first, then falls back to core/v1.
func (q *Querier) ListChoristerEvents(ctx context.Context, namespace string, since time.Duration, limit int) ([]EventInfo, error) {
	cutoff := time.Now().Add(-since)

	// Try events.k8s.io/v1 first
	var eventList eventsv1.EventList
	opts := []client.ListOption{}
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}
	if err := q.list(ctx, &eventList, opts...); err == nil && len(eventList.Items) > 0 {
		return eventsV1ToInfos(eventList.Items, cutoff, limit), nil
	}

	// Fallback to core/v1
	var coreEventList corev1.EventList
	coreOpts := []client.ListOption{}
	if namespace != "" {
		coreOpts = append(coreOpts, client.InNamespace(namespace))
	}
	if err := q.list(ctx, &coreEventList, coreOpts...); err != nil {
		return nil, wrapError("Event", "", namespace, err)
	}

	return coreEventsToInfos(coreEventList.Items, cutoff, limit), nil
}

func eventsV1ToInfos(events []eventsv1.Event, cutoff time.Time, limit int) []EventInfo {
	var result []EventInfo
	for _, e := range events {
		var eventTime time.Time
		if e.EventTime.Time.IsZero() {
			if e.DeprecatedFirstTimestamp.Time.IsZero() {
				continue
			}
			eventTime = e.DeprecatedFirstTimestamp.Time
		} else {
			eventTime = e.EventTime.Time
		}

		if eventTime.Before(cutoff) {
			continue
		}

		involved := ""
		if e.Regarding.Kind != "" {
			involved = e.Regarding.Kind + "/" + e.Regarding.Name
		}

		result = append(result, EventInfo{
			Time:           eventTime,
			Type:           e.Type,
			Reason:         e.Reason,
			Message:        e.Note,
			InvolvedObject: involved,
		})

		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

func coreEventsToInfos(events []corev1.Event, cutoff time.Time, limit int) []EventInfo {
	var result []EventInfo
	for _, e := range events {
		eventTime := e.LastTimestamp.Time
		if eventTime.IsZero() {
			eventTime = e.FirstTimestamp.Time
		}
		if eventTime.IsZero() {
			eventTime = e.CreationTimestamp.Time
		}

		if eventTime.Before(cutoff) {
			continue
		}

		involved := ""
		if e.InvolvedObject.Kind != "" {
			involved = e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name
		}

		result = append(result, EventInfo{
			Time:           eventTime,
			Type:           e.Type,
			Reason:         e.Reason,
			Message:        e.Message,
			InvolvedObject: involved,
		})

		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}
