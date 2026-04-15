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
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Querier wraps a controller-runtime client for read-only data retrieval.
// All methods take context.Context as the first parameter and wrap K8s errors
// with chorister-specific context (resource kind, name, namespace).
type Querier struct {
	client client.Client
}

// NewQuerier creates a new Querier with the given client.
func NewQuerier(c client.Client) *Querier {
	return &Querier{client: c}
}

// Client returns the underlying controller-runtime client.
// Use this when a report function needs to query non-chorister resources
// (e.g. corev1.ResourceQuota).
func (q *Querier) Client() client.Client {
	return q.client
}

// wrapError wraps a K8s API error with chorister context.
func wrapError(kind, name, namespace string, err error) error {
	if err == nil {
		return nil
	}
	if namespace != "" {
		return fmt.Errorf("%s %s/%s: %w", kind, namespace, name, err)
	}
	return fmt.Errorf("%s %s: %w", kind, name, err)
}

// get fetches a single object by name/namespace.
func (q *Querier) get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return q.client.Get(ctx, key, obj)
}

// list fetches a list of objects with optional list options.
func (q *Querier) list(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return q.client.List(ctx, list, opts...)
}
