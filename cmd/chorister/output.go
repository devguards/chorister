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
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/chorister-dev/chorister/internal/report"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

// addOutputFlag adds the --output flag to a command.
func addOutputFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("output", "o", "table", "Output format: table, json, yaml")
}

// getOutputFormat reads the --output flag value.
func getOutputFormat(cmd *cobra.Command) string {
	f, _ := cmd.Flags().GetString("output")
	return f
}

// renderOutput dispatches to the appropriate formatter based on --output flag.
// data is the structured object (for json/yaml), tableData is for table output.
func renderOutput(w io.Writer, format string, data any, tableData *report.TableData) error {
	switch format {
	case "json":
		return renderJSON(w, data)
	case "yaml":
		return renderYAML(w, data)
	default:
		if tableData != nil {
			renderTable(w, tableData)
			return nil
		}
		// Fall back to JSON for non-table data
		return renderJSON(w, data)
	}
}

// renderJSON marshals data as indented JSON.
func renderJSON(w io.Writer, data any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// renderYAML marshals data as YAML.
func renderYAML(w io.Writer, data any) error {
	b, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// renderTable renders TableData as aligned ASCII columns.
func renderTable(w io.Writer, td *report.TableData) {
	if td == nil || len(td.Headers) == 0 {
		return
	}

	// Calculate column widths
	widths := make([]int, len(td.Headers))
	for i, h := range td.Headers {
		widths[i] = len(h)
	}
	for _, row := range td.Rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print header
	var headerParts []string
	for i, h := range td.Headers {
		headerParts = append(headerParts, padRight(h, widths[i]))
	}
	fmt.Fprintln(w, strings.Join(headerParts, "  "))

	// Print rows
	for _, row := range td.Rows {
		var parts []string
		for i := range td.Headers {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			parts = append(parts, padRight(cell, widths[i]))
		}
		fmt.Fprintln(w, strings.Join(parts, "  "))
	}
}

// renderStatusSummary renders a StatusSummary as key-value pairs for table mode.
func renderStatusSummary(w io.Writer, ss *report.StatusSummary) {
	fmt.Fprintf(w, "Name:  %s\n", ss.Name)
	fmt.Fprintf(w, "Phase: %s\n", ss.Phase)

	if len(ss.Details) > 0 {
		fmt.Fprintln(w, "")
		// Sort keys for deterministic output
		keys := sortedKeys(ss.Details)
		maxKeyLen := 0
		for _, k := range keys {
			if len(k) > maxKeyLen {
				maxKeyLen = len(k)
			}
		}
		for _, k := range keys {
			fmt.Fprintf(w, "  %s  %s\n", padRight(k+":", maxKeyLen+1), ss.Details[k])
		}
	}

	if len(ss.Conditions) > 0 {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Conditions:")
		td := &report.TableData{
			Headers: []string{"TYPE", "STATUS", "REASON", "MESSAGE"},
		}
		for _, c := range ss.Conditions {
			td.Rows = append(td.Rows, []string{c.Type, c.Status, c.Reason, c.Message})
		}
		renderTable(w, td)
	}
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple insertion sort for small maps
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
