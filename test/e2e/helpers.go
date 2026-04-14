//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

func createNamespace(ctx context.Context, t *testing.T, cfg *envconf.Config, name string) context.Context {
	t.Helper()

	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := cfg.Client().Resources().Create(ctx, namespace); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace %q: %v", name, err)
	}

	if err := waitForCondition(ctx, 30*time.Second, 500*time.Millisecond, func() (bool, error) {
		return namespaceExists(ctx, cfg, name)
	}); err != nil {
		t.Fatalf("wait for namespace %q: %v", name, err)
	}

	return ctx
}

func cleanupNamespace(ctx context.Context, t *testing.T, cfg *envconf.Config, name string) context.Context {
	t.Helper()

	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := cfg.Client().Resources().Delete(ctx, namespace); err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("delete namespace %q: %v", name, err)
	}

	if err := waitForCondition(ctx, 30*time.Second, 500*time.Millisecond, func() (bool, error) {
		exists, err := namespaceExists(ctx, cfg, name)
		return !exists, err
	}); err != nil {
		t.Fatalf("wait for namespace cleanup %q: %v", name, err)
	}

	return ctx
}

func namespaceExists(ctx context.Context, cfg *envconf.Config, name string) (bool, error) {
	var namespaces corev1.NamespaceList
	if err := cfg.Client().Resources().List(ctx, &namespaces); err != nil {
		return false, err
	}

	for _, namespace := range namespaces.Items {
		if namespace.Name == name {
			return true, nil
		}
	}

	return false, nil
}

func applyManifest(ctx context.Context, t *testing.T, _ *envconf.Config, manifestPath string) context.Context {
	t.Helper()

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", manifestPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("apply manifest %q: %v: %s", manifestPath, err, string(output))
	}

	return ctx
}

func waitForCondition(ctx context.Context, timeout, interval time.Duration, check func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	var lastErr error

	for {
		ok, err := check()
		if err != nil {
			lastErr = err
		} else if ok {
			return nil
		}

		if time.Now().After(deadline) {
			if lastErr != nil {
				return fmt.Errorf("condition timed out after %s: %w", timeout, lastErr)
			}
			return fmt.Errorf("condition timed out after %s", timeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
