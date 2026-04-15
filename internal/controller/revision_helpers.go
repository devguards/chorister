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

package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choristerv1alpha1 "github.com/chorister-dev/chorister/api/v1alpha1"
)

const (
	// LabelRevision is the namespace label used for blue-green controller revision matching.
	LabelRevision = "chorister.dev/rev"
)

// ShouldSkipForRevision checks if a controller with the given revision should skip
// reconciliation for a resource in the given namespace. Returns true if the namespace
// revision label doesn't match or if the controller is not the stable revision for
// unlabeled namespaces.
func ShouldSkipForRevision(ctx context.Context, c client.Client, controllerRevision, namespace string) (bool, error) {
	ns := &corev1.Namespace{}
	if err := c.Get(ctx, types.NamespacedName{Name: namespace}, ns); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	nsRev := ns.Labels[LabelRevision]
	if nsRev != "" {
		// Namespace is labeled — must match controller revision
		return nsRev != controllerRevision, nil
	}

	// Namespace not labeled — check ChoCluster for stable revision
	clusterList := &choristerv1alpha1.ChoClusterList{}
	if err := c.List(ctx, clusterList); err != nil {
		return false, nil
	}
	if len(clusterList.Items) > 0 {
		stableRev := clusterList.Items[0].Spec.ControllerRevision
		if stableRev != "" && stableRev != controllerRevision {
			return true, nil
		}
	}

	return false, nil
}
