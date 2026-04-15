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

// Package loader parses multi-document YAML files containing chorister CRD
// manifests into typed API objects.
package loader

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

// Resources holds the typed objects parsed from a YAML file.
type Resources struct {
	Computes  []choristerv1alpha1.ChoCompute
	Databases []choristerv1alpha1.ChoDatabase
	Networks  []choristerv1alpha1.ChoNetwork
	Caches    []choristerv1alpha1.ChoCache
	Queues    []choristerv1alpha1.ChoQueue
	Storages  []choristerv1alpha1.ChoStorage
}

// typeMeta is used to peek at apiVersion and kind before full deserialization.
type typeMeta struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

var expectedGV = choristerv1alpha1.GroupVersion.String() // "chorister.dev/v1alpha1"

// LoadFile parses multi-document YAML from raw bytes and returns typed resources.
func LoadFile(data []byte) (*Resources, error) {
	docs, err := splitYAMLDocuments(data)
	if err != nil {
		return nil, fmt.Errorf("splitting YAML documents: %w", err)
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no YAML documents found in input")
	}

	res := &Resources{}
	for i, doc := range docs {
		if err := parseDocument(res, doc, i); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func parseDocument(res *Resources, doc []byte, index int) error {
	// Convert YAML to JSON for consistent unmarshalling.
	jsonData, err := yaml.YAMLToJSON(doc)
	if err != nil {
		return fmt.Errorf("document %d: invalid YAML: %w", index, err)
	}

	var meta typeMeta
	if err := json.Unmarshal(jsonData, &meta); err != nil {
		return fmt.Errorf("document %d: cannot read apiVersion/kind: %w", index, err)
	}

	gv, err := schema.ParseGroupVersion(meta.APIVersion)
	if err != nil {
		return fmt.Errorf("document %d: invalid apiVersion %q: %w", index, meta.APIVersion, err)
	}
	if gv.String() != expectedGV {
		return fmt.Errorf("document %d: unsupported apiVersion %q (expected %s)", index, meta.APIVersion, expectedGV)
	}

	switch meta.Kind {
	case "ChoCompute":
		obj := choristerv1alpha1.ChoCompute{}
		if err := unmarshalInto(jsonData, &obj); err != nil {
			return fmt.Errorf("document %d (ChoCompute): %w", index, err)
		}
		res.Computes = append(res.Computes, obj)
	case "ChoDatabase":
		obj := choristerv1alpha1.ChoDatabase{}
		if err := unmarshalInto(jsonData, &obj); err != nil {
			return fmt.Errorf("document %d (ChoDatabase): %w", index, err)
		}
		res.Databases = append(res.Databases, obj)
	case "ChoNetwork":
		obj := choristerv1alpha1.ChoNetwork{}
		if err := unmarshalInto(jsonData, &obj); err != nil {
			return fmt.Errorf("document %d (ChoNetwork): %w", index, err)
		}
		res.Networks = append(res.Networks, obj)
	case "ChoCache":
		obj := choristerv1alpha1.ChoCache{}
		if err := unmarshalInto(jsonData, &obj); err != nil {
			return fmt.Errorf("document %d (ChoCache): %w", index, err)
		}
		res.Caches = append(res.Caches, obj)
	case "ChoQueue":
		obj := choristerv1alpha1.ChoQueue{}
		if err := unmarshalInto(jsonData, &obj); err != nil {
			return fmt.Errorf("document %d (ChoQueue): %w", index, err)
		}
		res.Queues = append(res.Queues, obj)
	case "ChoStorage":
		obj := choristerv1alpha1.ChoStorage{}
		if err := unmarshalInto(jsonData, &obj); err != nil {
			return fmt.Errorf("document %d (ChoStorage): %w", index, err)
		}
		res.Storages = append(res.Storages, obj)
	default:
		return fmt.Errorf("document %d: unsupported kind %q (expected one of: ChoCompute, ChoDatabase, ChoNetwork, ChoCache, ChoQueue, ChoStorage)", index, meta.Kind)
	}
	return nil
}

func unmarshalInto(jsonData []byte, obj runtime.Object) error {
	return json.Unmarshal(jsonData, obj)
}

// splitYAMLDocuments splits a multi-document YAML stream on "---" separators,
// returning non-empty document bodies.
func splitYAMLDocuments(data []byte) ([][]byte, error) {
	var docs [][]byte
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // up to 1 MB per line
	var current bytes.Buffer

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			if doc := bytes.TrimSpace(current.Bytes()); len(doc) > 0 {
				cp := make([]byte, len(doc))
				copy(cp, doc)
				docs = append(docs, cp)
			}
			current.Reset()
			continue
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return nil, err
	}
	if doc := bytes.TrimSpace(current.Bytes()); len(doc) > 0 {
		docs = append(docs, doc)
	}
	return docs, nil
}

// Total returns the number of resources loaded.
func (r *Resources) Total() int {
	return len(r.Computes) + len(r.Databases) + len(r.Networks) +
		len(r.Caches) + len(r.Queues) + len(r.Storages)
}
