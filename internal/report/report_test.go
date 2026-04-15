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

package report

import (
	"testing"
	"time"
)

func TestFormatAge_Days(t *testing.T) {
	got := FormatAge(time.Now().Add(-48 * time.Hour))
	if got != "2d" {
		t.Fatalf("Expected '2d', got %q", got)
	}
}

func TestFormatAge_Hours(t *testing.T) {
	got := FormatAge(time.Now().Add(-3 * time.Hour))
	if got != "3h" {
		t.Fatalf("Expected '3h', got %q", got)
	}
}

func TestFormatAge_Minutes(t *testing.T) {
	got := FormatAge(time.Now().Add(-5 * time.Minute))
	if got != "5m" {
		t.Fatalf("Expected '5m', got %q", got)
	}
}

func TestFormatAge_LessThanMinute(t *testing.T) {
	got := FormatAge(time.Now().Add(-10 * time.Second))
	if got != "<1m" {
		t.Fatalf("Expected '<1m', got %q", got)
	}
}

func TestFormatAge_Zero(t *testing.T) {
	got := FormatAge(time.Time{})
	if got != "<unknown>" {
		t.Fatalf("Expected '<unknown>', got %q", got)
	}
}

func TestFormatAge_Future(t *testing.T) {
	got := FormatAge(time.Now().Add(10 * time.Minute))
	if got != "<future>" {
		t.Fatalf("Expected '<future>', got %q", got)
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "-"},
		{"12.50", "$12.50/mo"},
		{"$12.50", "$12.50/mo"},
		{"0.00", "$0.00/mo"},
	}
	for _, tc := range tests {
		got := FormatCost(tc.input)
		if got != tc.want {
			t.Errorf("FormatCost(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
