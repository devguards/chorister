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
	"strings"
	"time"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DomainResources holds all chorister resources in a domain namespace.
type DomainResources struct {
	Computes  []choristerv1alpha1.ChoCompute
	Databases []choristerv1alpha1.ChoDatabase
	Queues    []choristerv1alpha1.ChoQueue
	Caches    []choristerv1alpha1.ChoCache
	Storages  []choristerv1alpha1.ChoStorage
	Networks  []choristerv1alpha1.ChoNetwork
}

// TotalCount returns the total number of resources.
func (dr *DomainResources) TotalCount() int {
	return len(dr.Computes) + len(dr.Databases) + len(dr.Queues) + len(dr.Caches) + len(dr.Storages) + len(dr.Networks)
}

// ListDomainResources returns all chorister resources in the given namespace.
func (q *Querier) ListDomainResources(ctx context.Context, namespace string) (*DomainResources, error) {
	inNs := client.InNamespace(namespace)
	r := &DomainResources{}

	var computes choristerv1alpha1.ChoComputeList
	if err := q.list(ctx, &computes, inNs); err != nil {
		return nil, wrapError("ChoCompute", "", namespace, err)
	}
	r.Computes = computes.Items

	var databases choristerv1alpha1.ChoDatabaseList
	if err := q.list(ctx, &databases, inNs); err != nil {
		return nil, wrapError("ChoDatabase", "", namespace, err)
	}
	r.Databases = databases.Items

	var queues choristerv1alpha1.ChoQueueList
	if err := q.list(ctx, &queues, inNs); err != nil {
		return nil, wrapError("ChoQueue", "", namespace, err)
	}
	r.Queues = queues.Items

	var caches choristerv1alpha1.ChoCacheList
	if err := q.list(ctx, &caches, inNs); err != nil {
		return nil, wrapError("ChoCache", "", namespace, err)
	}
	r.Caches = caches.Items

	var storages choristerv1alpha1.ChoStorageList
	if err := q.list(ctx, &storages, inNs); err != nil {
		return nil, wrapError("ChoStorage", "", namespace, err)
	}
	r.Storages = storages.Items

	var networks choristerv1alpha1.ChoNetworkList
	if err := q.list(ctx, &networks, inNs); err != nil {
		return nil, wrapError("ChoNetwork", "", namespace, err)
	}
	r.Networks = networks.Items

	return r, nil
}

// GetResource fetches a single chorister resource by kind and name.
// Supported kinds: compute, database, queue, cache, storage, network, sandbox, promotion.
func (q *Querier) GetResource(ctx context.Context, kind, name, namespace string) (client.Object, error) {
	key := client.ObjectKey{Name: name, Namespace: namespace}
	switch strings.ToLower(kind) {
	case "compute":
		obj := &choristerv1alpha1.ChoCompute{}
		return obj, q.get(ctx, key, obj)
	case "database":
		obj := &choristerv1alpha1.ChoDatabase{}
		return obj, q.get(ctx, key, obj)
	case "queue":
		obj := &choristerv1alpha1.ChoQueue{}
		return obj, q.get(ctx, key, obj)
	case "cache":
		obj := &choristerv1alpha1.ChoCache{}
		return obj, q.get(ctx, key, obj)
	case "storage":
		obj := &choristerv1alpha1.ChoStorage{}
		return obj, q.get(ctx, key, obj)
	case "network":
		obj := &choristerv1alpha1.ChoNetwork{}
		return obj, q.get(ctx, key, obj)
	case "sandbox":
		var list choristerv1alpha1.ChoSandboxList
		if err := q.list(ctx, &list); err != nil {
			return nil, wrapError("ChoSandbox", name, "", err)
		}
		for i := range list.Items {
			if list.Items[i].Name == name || list.Items[i].Spec.Name == name {
				return &list.Items[i], nil
			}
		}
		return nil, fmt.Errorf("ChoSandbox %q not found", name)
	case "promotion":
		obj := &choristerv1alpha1.ChoPromotionRequest{}
		if err := q.get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, obj); err != nil {
			return nil, wrapError("ChoPromotionRequest", name, namespace, err)
		}
		return obj, nil
	default:
		return nil, fmt.Errorf("unknown resource type %q: supported types are compute, database, queue, cache, storage, network, sandbox, promotion", kind)
	}
}

// WaitForCondition polls until the named resource has the specified condition type
// set to True, or until the timeout is reached.
// Returns nil when the condition is met, or an error on timeout or failure.
func (q *Querier) WaitForCondition(ctx context.Context, kind, name, namespace, conditionType string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s %s/%s condition %q after %s", kind, namespace, name, conditionType, timeout)
		case <-ticker.C:
			met, err := q.checkCondition(ctx, kind, name, namespace, conditionType)
			if err != nil {
				return err
			}
			if met {
				return nil
			}
		}
	}
}

// checkCondition checks whether a named condition is True on the given resource.
func (q *Querier) checkCondition(ctx context.Context, kind, name, namespace, conditionType string) (bool, error) {
	key := client.ObjectKey{Name: name, Namespace: namespace}

	switch strings.ToLower(kind) {
	case "compute":
		var obj choristerv1alpha1.ChoCompute
		if err := q.get(ctx, key, &obj); err != nil {
			return false, nil // not found yet, keep waiting
		}
		return conditionMetav1(obj.Status.Conditions, conditionType), nil
	case "database":
		var obj choristerv1alpha1.ChoDatabase
		if err := q.get(ctx, key, &obj); err != nil {
			return false, nil
		}
		return conditionMetav1(obj.Status.Conditions, conditionType), nil
	case "queue":
		var obj choristerv1alpha1.ChoQueue
		if err := q.get(ctx, key, &obj); err != nil {
			return false, nil
		}
		return conditionMetav1(obj.Status.Conditions, conditionType), nil
	case "sandbox":
		var list choristerv1alpha1.ChoSandboxList
		if err := q.list(ctx, &list); err != nil {
			return false, nil
		}
		for _, sb := range list.Items {
			if sb.Name == name || sb.Spec.Name == name {
				if strings.EqualFold(sb.Status.Phase, conditionType) {
					return true, nil
				}
				return conditionMetav1(sb.Status.Conditions, conditionType), nil
			}
		}
		return false, nil
	case "promotion":
		var obj choristerv1alpha1.ChoPromotionRequest
		if err := q.get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, &obj); err != nil {
			return false, nil
		}
		return strings.EqualFold(obj.Status.Phase, conditionType), nil
	default:
		return false, fmt.Errorf("unknown resource type %q for condition check", kind)
	}
}

// conditionMetav1 returns true if a metav1.Condition of the given type has Status=True.
func conditionMetav1(conditions []metav1.Condition, condType string) bool {
	for _, c := range conditions {
		if c.Type == condType && c.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}
