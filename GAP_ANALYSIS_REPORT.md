# Gap Analysis Report

Date: 2026-04-16

Scope: This report compares the current repository implementation to the target architecture in ARCHITECTURE_DECISIONS.md. It is based on direct source inspection of the CRDs, controllers, CLI, compiler, validation, webhooks, and tests.

## Executive Summary

The implementation is materially closer to the planned architecture than the previous report suggested. The operator shape is in place: the CRD surface exists, reconcilers are implemented, the CLI is no longer a skeleton, promotion and sandbox workflows are real, and several items previously reported as critical gaps have already been fixed.

The remaining differences are mostly in hardening and end-to-end completeness rather than missing scaffolding. The most important gaps are:

1. Promotion security scanning still defaults to a simulated scanner.
2. Audit logging is implemented but disabled by default via the manager flag default.
3. The observability stack is deployed, but not with the object-storage-backed configuration described in the architecture.
4. Admission validation only exists for a subset of CRD types.
5. Some advanced architecture promises are modeled in types but not fully enforced or verified end to end.

## What Matches The Planned Architecture

- The core operator model is implemented: the controller manager registers the full reconciler set and acts as the control-plane entrypoint.
- The API surface is broadly present: the CRD set under api/v1alpha1 matches the architecture's major resource model.
- The CLI is a real thin client rather than a stub. It includes setup, login, apply, sandbox lifecycle, promote, approve, reject, diff, status, export, and admin flows.
- Sandbox-first workflows are implemented. The sandbox reconciler creates isolated namespaces, default-deny policy, and cleanup logic.
- Promotion is implemented as a real state machine. Approval counting, execution, and failure handling exist in the promotion controller.
- Domain isolation is enforced in promotion. An isolated domain blocks execution.
- The earlier ChoApplication namespace mismatch has been fixed. The CLI now creates the application in the control-plane namespace.
- Webhook markers use the correct API group, chorister.dev.
- OIDC-backed membership deprovisioning logic exists in the domain membership reconciler.
- setup --wait performs actual deployment readiness polling.
- The network/compiler path exists for Gateway API, Cilium policy generation, certificates, and Tetragon policy compilation.
- End-to-end tests are no longer broadly skipped; the older report's blanket E2E-skip claim is stale.

## Confirmed Current Gaps

### 1. Promotion security scanning is still simulated by default

Severity: High

The scanning package contains a real Trivy-backed scanner implementation, but the active default path still returns a signature-based simulated scanner. The promotion controller uses scanning.NewDefaultScanner() unless a scanner is injected.

What this means:
- Security gating exists structurally.
- The default runtime behavior does not provide real vulnerability detection.
- Applications relying on requireSecurityScan or higher compliance profiles do not yet get the architecture's intended protection unless the scanner wiring is changed.

Evidence:
- internal/scanning/trivy.go: NewDefaultScanner returns SignatureScanner.
- internal/controller/chopromotionrequest_controller.go: getScanner falls back to scanning.NewDefaultScanner().

### 2. Audit logging is opt-in, not the default runtime behavior

Severity: High

The architecture treats the intent log as a synchronous, fail-fast part of reconciliation. The manager supports this behavior, but the default flag value is still audit-sink=noop, which disables audit delivery unless explicitly configured.

What this means:
- The fail-fast audit path exists in the controllers.
- A default deployment will not emit the audit trail promised by the architecture unless operators set the audit sink.

Evidence:
- cmd/main.go: auditSink defaults to noop.
- internal/controller/chocluster_controller.go: reconciliation blocks when the configured audit logger fails.

### 3. Observability deployment does not yet match the object-storage-backed architecture

Severity: High

The ChoCluster API models observability versions and retention. The cluster reconciler deploys Loki, Mimir, Tempo, Alloy, and Grafana as plain Deployments and Services. The architecture, however, specifies LGTM backed by object storage with shared bucket-backed persistence and defined retention defaults.

What this means:
- The stack is bootstrapped.
- The storage backend, shared-bucket wiring, and retention behavior described in the architecture are not implemented in the reconciler.

Evidence:
- api/v1alpha1/chocluster_types.go: ObservabilitySpec and RetentionSpec exist.
- internal/controller/chocluster_controller.go: reconcileObservability only creates Deployments and Services and does not wire storage backends or retention configuration.

### 4. Admission validation only covers part of the API surface

Severity: Medium

Webhook validation exists for ChoApplication, ChoNetwork, ChoDomainMembership, and ChoPromotionRequest. The rest of the CRDs rely on reconciliation-time validation and status failures rather than admission rejection.

What this means:
- The most policy-sensitive resources have webhook validation.
- Many invalid specs still make it into the cluster and fail later than they should.

Evidence:
- internal/webhook/v1alpha1 contains four webhook implementations, not a full CRD-wide validation layer.

### 5. Object storage support is present but still partial at the platform-integration layer

Severity: Medium

ChoStorage can reconcile object variants into kro ObjectStorageClaim resources, and ChoCluster has cloud-provider bootstrap logic. But the architecture describes an end-to-end cloud-resource path with provider-backed fulfillment. The current implementation deploys a generic provider controller image and object claim resources, but there is not yet enough source evidence of a full, verified provider bootstrap path including provider-specific configuration, credentials, and end-to-end fulfillment guarantees.

What this means:
- The architecture path is partially implemented.
- The codebase shows intent and partial wiring, but not a clearly complete platform contract for object storage provisioning.

Evidence:
- internal/controller/chostorage_controller.go: object storage reconciles to ObjectStorageClaim.
- internal/controller/chocluster_controller.go: reconcileCloudProvider deploys a provider controller image.

### 6. Compilation-stability tracking across upgrades is still incomplete

Severity: Low

The architecture calls for visibility into compiled output changes across controller revisions. The diff package exists, but compilation-stability tracking is still flagged by a skipped test.

Evidence:
- internal/diff/diff_test.go: awaiting Phase 19.3 compilation stability tracking.

## Findings That Were Stale In The Previous Report

The prior version of this report overstated several gaps that are no longer present in the current codebase.

- The ChoApplication namespace bug is fixed.
- Webhook API group mismatch is fixed.
- Domain isolation blocking is implemented.
- setup --wait is implemented as real deployment polling.
- OIDC membership deprovisioning logic exists.
- Broad claims that the CLI is mostly stubbed or that E2E coverage is entirely skipped are no longer accurate.

## Bottom Line

The current implementation is broadly aligned with the planned architecture at the structural level. The main remaining gaps are about production-grade depth, especially around real security scanning, default-on auditing, and the richer platform integrations promised for observability and cloud-backed storage.

If the question is whether the codebase still differs from the planned architecture, the answer is yes, but the differences are narrower and more operational than foundational. The platform shape is there. The remaining work is mostly about making the existing paths trustworthy enough to match the architecture's stronger guarantees.
