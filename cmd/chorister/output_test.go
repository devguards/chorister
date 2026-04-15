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

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/chorister-dev/chorister/internal/report"
)

func TestOutputTable(t *testing.T) {
	td := &report.TableData{
		Headers: []string{"NAME", "STATUS", "AGE"},
		Rows: [][]string{
			{"payments", "Ready", "2d"},
			{"auth", "Pending", "5m"},
		},
	}

	var buf bytes.Buffer
	renderTable(&buf, td)
	out := buf.String()

	// Check header present
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "STATUS") || !strings.Contains(out, "AGE") {
		t.Fatalf("Table missing headers:\n%s", out)
	}

	// Check rows present
	if !strings.Contains(out, "payments") || !strings.Contains(out, "auth") {
		t.Fatalf("Table missing rows:\n%s", out)
	}

	// Check alignment: all lines should have same column structure
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("Expected 3 lines (header + 2 rows), got %d", len(lines))
	}
}

func TestOutputJSON(t *testing.T) {
	data := map[string]interface{}{
		"name":    "myapp",
		"phase":   "Ready",
		"domains": 2,
	}

	var buf bytes.Buffer
	if err := renderJSON(&buf, data); err != nil {
		t.Fatalf("renderJSON error: %v", err)
	}

	// Validate JSON
	var parsed map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("Output is not valid JSON: %v\n%s", err, buf.String())
	}
	if parsed["name"] != "myapp" {
		t.Errorf("Expected name 'myapp', got %v", parsed["name"])
	}
}

func TestOutputYAML(t *testing.T) {
	data := map[string]interface{}{
		"name":  "myapp",
		"phase": "Ready",
	}

	var buf bytes.Buffer
	if err := renderYAML(&buf, data); err != nil {
		t.Fatalf("renderYAML error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "name: myapp") {
		t.Fatalf("YAML output missing expected content:\n%s", out)
	}
	if !strings.Contains(out, "phase: Ready") {
		t.Fatalf("YAML output missing expected content:\n%s", out)
	}
}

func TestRenderOutput_Dispatch(t *testing.T) {
	td := &report.TableData{
		Headers: []string{"NAME"},
		Rows:    [][]string{{"test"}},
	}
	data := map[string]string{"name": "test"}

	// Table format
	var buf bytes.Buffer
	if err := renderOutput(&buf, "table", data, td); err != nil {
		t.Fatalf("renderOutput table error: %v", err)
	}
	if !strings.Contains(buf.String(), "NAME") {
		t.Fatalf("Expected table output, got:\n%s", buf.String())
	}

	// JSON format
	buf.Reset()
	if err := renderOutput(&buf, "json", data, td); err != nil {
		t.Fatalf("renderOutput json error: %v", err)
	}
	if !strings.Contains(buf.String(), `"name"`) {
		t.Fatalf("Expected JSON output, got:\n%s", buf.String())
	}

	// YAML format
	buf.Reset()
	if err := renderOutput(&buf, "yaml", data, td); err != nil {
		t.Fatalf("renderOutput yaml error: %v", err)
	}
	if !strings.Contains(buf.String(), "name: test") {
		t.Fatalf("Expected YAML output, got:\n%s", buf.String())
	}
}
