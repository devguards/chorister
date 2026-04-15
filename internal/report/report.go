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
	"fmt"
	"math"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TableData represents tabular output for CLI rendering.
type TableData struct {
	Headers []string
	Rows    [][]string
}

// ConditionSummary is a simplified view of a metav1.Condition.
type ConditionSummary struct {
	Type    string
	Status  string
	Reason  string
	Message string
}

// StatusSummary provides a detailed view of a single resource or aggregate.
type StatusSummary struct {
	Name       string
	Phase      string
	Conditions []ConditionSummary
	Details    map[string]string
}

// HealthRollup aggregates health across multiple items.
type HealthRollup struct {
	Healthy  int
	Degraded int
	Unknown  int
	Items    []StatusSummary
}

// FormatAge returns a human-readable age string: "2d", "3h", "5m", "<1m".
func FormatAge(t time.Time) string {
	if t.IsZero() {
		return "<unknown>"
	}
	d := time.Since(t)
	if d < 0 {
		return "<future>"
	}
	switch {
	case d >= 24*time.Hour:
		days := int(math.Floor(d.Hours() / 24))
		return fmt.Sprintf("%dd", days)
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return "<1m"
	}
}

// FormatCost formats a cost string with dollar sign (e.g. "$12.50/mo").
func FormatCost(cost string) string {
	if cost == "" {
		return "-"
	}
	if strings.HasPrefix(cost, "$") {
		return cost + "/mo"
	}
	return "$" + cost + "/mo"
}

// ConditionsFromMeta converts metav1.Conditions to ConditionSummary slice.
func ConditionsFromMeta(conditions []metav1.Condition) []ConditionSummary {
	result := make([]ConditionSummary, len(conditions))
	for i, c := range conditions {
		result[i] = ConditionSummary{
			Type:    c.Type,
			Status:  string(c.Status),
			Reason:  c.Reason,
			Message: c.Message,
		}
	}
	return result
}
