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

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

// OperatorInfo provides detailed information about a managed operator.
type OperatorInfo struct {
	Name    string
	Version string
	Status  string
}

// GetCluster returns the singleton ChoCluster resource.
func (q *Querier) GetCluster(ctx context.Context) (*choristerv1alpha1.ChoCluster, error) {
	var list choristerv1alpha1.ChoClusterList
	if err := q.list(ctx, &list); err != nil {
		return nil, wrapError("ChoCluster", "", "", err)
	}
	if len(list.Items) == 0 {
		return nil, fmt.Errorf("ChoCluster: no cluster resource found")
	}
	return &list.Items[0], nil
}

// GetOperatorDetails returns detailed info about each managed operator from the ChoCluster spec and status.
func (q *Querier) GetOperatorDetails(ctx context.Context) ([]OperatorInfo, error) {
	cluster, err := q.GetCluster(ctx)
	if err != nil {
		return nil, err
	}

	// Build operator list from spec versions and status
	type opDef struct {
		Name    string
		Version string
	}

	var defs []opDef
	if cluster.Spec.Operators != nil {
		ops := cluster.Spec.Operators
		if ops.Kro != "" {
			defs = append(defs, opDef{"kro", ops.Kro})
		}
		if ops.StackGres != "" {
			defs = append(defs, opDef{"stackgres", ops.StackGres})
		}
		if ops.NATS != "" {
			defs = append(defs, opDef{"nats", ops.NATS})
		}
		if ops.Dragonfly != "" {
			defs = append(defs, opDef{"dragonfly", ops.Dragonfly})
		}
		if ops.CertManager != "" {
			defs = append(defs, opDef{"cert-manager", ops.CertManager})
		}
		if ops.Gatekeeper != "" {
			defs = append(defs, opDef{"gatekeeper", ops.Gatekeeper})
		}
		if ops.Tetragon != "" {
			defs = append(defs, opDef{"tetragon", ops.Tetragon})
		}
	}

	// If no spec versions, fall back to status keys only
	if len(defs) == 0 && cluster.Status.OperatorStatus != nil {
		for name := range cluster.Status.OperatorStatus {
			defs = append(defs, opDef{name, ""})
		}
	}

	infos := make([]OperatorInfo, 0, len(defs))
	for _, d := range defs {
		status := "Unknown"
		if cluster.Status.OperatorStatus != nil {
			if s, ok := cluster.Status.OperatorStatus[d.Name]; ok {
				status = s
			}
		}
		infos = append(infos, OperatorInfo{
			Name:    d.Name,
			Version: d.Version,
			Status:  status,
		})
	}

	return infos, nil
}
