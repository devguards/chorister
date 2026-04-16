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

package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// sseEventPayload is the JSON shape sent over the SSE stream.
// The client-side JavaScript renders this into a table row.
type sseEventPayload struct {
	Time           string `json:"time"`
	Type           string `json:"type"` // Normal | Warning
	Reason         string `json:"reason"`
	Message        string `json:"message"`
	InvolvedObject string `json:"object"`
	Namespace      string `json:"namespace"`
}

// handleSSE streams Kubernetes events from the chorister control-plane namespace
// as Server-Sent Events. The client receives JSON payloads it turns into table rows.
//
// Implementation note: uses poll-then-push (5-second interval) against the K8s
// List API rather than the Watch API. A production upgrade would use client-go's
// Watch + Informer mechanism for lower latency, but the List approach requires no
// additional setup and works against any standard kubeconfig.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported by this server", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Prevent nginx/proxies from buffering the stream.
	w.Header().Set("X-Accel-Buffering", "no")

	// Namespace to watch; query param ?ns= allows overriding.
	ns := r.URL.Query().Get("ns")
	if ns == "" {
		ns = controlPlaneNamespace
	}

	// Send an initial ping so the client knows the connection is live.
	fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
	flusher.Flush()

	// lastSeen tracks which events we have already sent to avoid duplicates.
	seen := make(map[string]struct{})
	cutoff := time.Now().Add(-5 * time.Minute) // hydrate with recent history first

	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()

	// Helper: send one SSE event frame.
	send := func(p sseEventPayload) {
		data, _ := json.Marshal(p)
		fmt.Fprintf(w, "event: event\ndata: %s\n\n", data)
	}

	// Poll loop.
	for {
		events, err := s.querier.ListChoristerEvents(r.Context(), ns, time.Since(cutoff)+time.Minute, 100)
		if err == nil {
			for _, ev := range events {
				key := fmt.Sprintf("%s/%s", ev.Reason, ev.Time.Format(time.RFC3339))
				if _, alreadySent := seen[key]; alreadySent {
					continue
				}
				seen[key] = struct{}{}
				send(sseEventPayload{
					Time:           ev.Time.Format("2006-01-02 15:04:05"),
					Type:           ev.Type,
					Reason:         ev.Reason,
					Message:        ev.Message,
					InvolvedObject: ev.InvolvedObject,
					Namespace:      ns,
				})
			}
		}
		cutoff = time.Now() // next poll only looks at new events
		flusher.Flush()

		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			// Keep-alive ping every tick even if no new events.
			fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
			flusher.Flush()
		}
	}
}
