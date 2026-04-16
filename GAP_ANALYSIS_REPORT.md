# Gap Analysis Report

Date: 2026-04-15 (revised)

Scope: This report compares the current repository implementation to the target architecture defined in `ARCHITECTURE_DECISIONS.md`. It is based on source inspection of CRDs, controllers, CLI code, internal packages, tests, scenario scripts, and verification of every claim against the actual code.

## Executive Summary

The codebase is substantially implemented. All 12 CRDs exist, all major reconcilers are registered and functional, the CLI covers the full command surface (setup, login, apply, diff, sandbox, promote, approve, reject, admin CRUD, status, logs, events, wait, export), and resource backends integrate with their target operators (StackGres, NATS JetStream, Dragonfly, kro ObjectStorageClaim).

The remaining gaps are **not** in basic controller or CLI structure. They fall into three categories: (1) a critical namespace contract bug that breaks the CLI→controller chain, (2) simulated/placeholder behavior in specific subsystems (scanning, setup `--wait`, database snapshots), and (3) stale test suites that describe an earlier state of the codebase and skip all end-to-end verification.

## What Matches The Architecture

- **Operator framework**: controller-runtime manager with 12 registered reconcilers in `cmd/main.go`.
- **CRD set**: All 12 types under `api/v1alpha1/` match the architecture.
- **Application/domain hierarchy**: Namespace creation, default-deny policies, ResourceQuota, LimitRange, cross-application links, consumes/supplies validation, cycle detection.
- **Sandbox lifecycle**: Namespace creation, default-deny, cost estimation, budget enforcement, idle detection/auto-destroy.
- **Promotion pipeline**: Full state machine (Pending→Approved→Executing→Completed/Failed/Rejected), approval counting, resource copy for all 6 types, archive orphaned stateful resources, security scan gate.
- **Production safety**: CLI refuses `apply` to production, production namespaces get view-only RBAC.
- **Resource backends**: ChoCompute→Deployment/Job/CronJob/HPA/PDB. ChoDatabase→StackGres SGCluster+SGInstanceProfile with Patroni HA. ChoQueue→NATS StatefulSet with PVCs and JetStream. ChoCache→Dragonfly. ChoStorage→PVC (block/file) + kro ObjectStorageClaim (object).
- **Network guardrails**: Ingress auth validation, egress allowlist, compliance escalation, archived-resource dependency checks, retention minimums in `internal/validation/`.
- **Compiler**: HTTPRoute, CiliumNetworkPolicy, CiliumEnvoyConfig, Tetragon TracingPolicy, cert-manager Certificate, Cilium encryption policy, kro RGD for object storage. 10 real tests with full assertions.
- **CLI**: Full command surface — setup, login (OIDC device flow), apply (via `internal/loader`), sandbox CRUD, diff (via `internal/diff`), status, promote, approve, reject, admin (app/domain/member/scan/isolate/unisolate), export, get, wait, logs, events, docs.
- **Audit**: LokiLogger and NoopLogger with `--audit-sink` flag (noop/loki/auto).
- **Domain membership**: RBAC RoleBindings, expiry enforcement, role mapping, restricted-domain requires expiresAt.

## Critical Gaps

### 1. ChoApplication namespace not set — controllers cannot find it

**Bug.** The CLI `admin app create` command creates `ChoApplication` without setting `Namespace` in ObjectMeta (`cmd/chorister/main.go` line 1524). This means the resource is created in the kubeconfig's current namespace (typically `default`).

However, `ChoSandbox` and `ChoPromotionRequest` are created in `cho-system` (the `controlPlaneNamespace`). The controllers look up the parent `ChoApplication` using `pr.Namespace` / `sandbox.Namespace` — which is `cho-system`. They will never find an app created in `default`.

**Affected controllers:**
- `ChoPromotionRequestReconciler` at `chopromotionrequest_controller.go` line 98: `r.Get(ctx, types.NamespacedName{Name: pr.Spec.Application, Namespace: pr.Namespace}, app)`
- `ChoSandboxReconciler.lookupApplication()` at `chosandbox_controller.go` line 290: uses `sandbox.Namespace`
- `ChoDomainMembershipReconciler` at `chodomainmembership_controller.go` line 73: uses `membership.Namespace`

**Fix:** Add `Namespace: controlPlaneNamespace` to the ChoApplication ObjectMeta in `admin app create`, or make ChoApplication cluster-scoped.

### 2. Webhook API group mismatch — webhooks will never intercept requests

**Bug.** Both validating webhooks use the wrong API group in their kubebuilder markers:

```
groups=chorister.chorister.dev
```

The actual CRD group is `chorister.dev` (per `groupversion_info.go` line 30 and all generated CRDs in `config/crd/bases/`). The PROJECT file has `group: chorister` + `domain: chorister.dev`, which kubebuilder combines as `chorister.chorister.dev` — but the `groupversion_info.go` was manually corrected to `chorister.dev`.

The generated webhook manifests will target `chorister.chorister.dev` and never match actual admission requests for `chorister.dev` resources.

**Affected files:**
- `internal/webhook/v1alpha1/choapplication_webhook.go` line 48
- `internal/webhook/v1alpha1/chonetwork_webhook.go` line 48

**Fix:** Change `groups=chorister.chorister.dev` to `groups=chorister.dev` in both webhook markers, then run `make manifests`.

### 3. Simulated vulnerability scanner — scan gate provides no real security

The scanner in `internal/scanning/trivy.go` is a string-matching placeholder. It detects "vulnerabilities" by checking if the image name contains `"critical"`, `"cve"`, `"vuln"`, or `"high"`. It returns `SIMULATED-CRITICAL-CVE` findings.

The promotion security scan gate (in `chopromotionrequest_controller.go` line 496) calls this scanner. For compliance profiles `standard`/`regulated` that require `requireSecurityScan: true`, the gate runs but provides zero real security value.

**Impact:** The architecture promises image scanning before promotion. The code path exists and is wired, but the underlying scanner is a placeholder.

## High-Severity Gaps

### 4. Degraded/isolated domain does not block promotion

`IsDomainIsolated()` exists as a helper in `choapplication_controller.go` (line 1010), and the CLI `admin isolate`/`admin unisolate` commands set/remove the annotation. However, the promotion controller **never checks** isolation status before executing a promotion.

The corresponding unit test at `chopromotionrequest_controller_test.go` line 487 is `Skip("awaiting Phase 15.3")`.

**Impact:** An admin can isolate a domain during an incident, but promotions to that domain still proceed.

### 5. All 14 E2E tests are skipped

Every E2E test beyond the basic suite setup is skipped with `Skip("awaiting Phase N")` reasons:

| File | Skipped tests | Skip reasons |
|---|---|---|
| `test/e2e/developer_workflow_test.go` | 2 | Phase 2-8, Phase 21 |
| `test/e2e/production_safety_test.go` | 2 | Phase 8, Phase 9 |
| `test/e2e/compliance_test.go` | 3 | Phase 10, Phase 14, requires Tetragon |
| `test/e2e/network_test.go` | 4 | Phase 6-7, Phase 13, Phase 13.3 |
| `test/e2e/incident_archive_test.go` | 3 | requires Kind cluster |

The skip reasons reference ROADMAP phases that are marked `[x]` complete. The tests have never been un-skipped despite the features being implemented.

**Impact:** Source code implements several guarantees with no end-to-end verification.

### 6. Scenario test scripts are stale — use kubectl fallbacks for implemented commands

19+ operations across 8 scenario scripts use `# STUB: chorister <command> not implemented — use kubectl` comments and fall back to raw `kubectl apply`. These commands (`admin app create`, `apply`, `diff`, `admin member remove`) are now fully implemented in the CLI.

**Affected scenarios:** 01-platform-bootstrap, 02-developer-sandbox, 03-sandbox-to-production, 05-ingress-jwt, 06-archive-safety, 07-full-stack, 10-domain-membership, 11-finops-budget.

**Impact:** Scenarios don't exercise the CLI at all; they only test the controller via direct kubectl. CLI regressions would be invisible.

## Medium-Severity Gaps

### 7. `chorister setup --wait` is a no-op

The `--wait` flag is accepted but does not poll the Deployment status (`cmd/chorister/main.go` line 216):

```go
// In a real implementation this would poll the Deployment status.
// For now, we report success — the --wait flag is wired for future use.
fmt.Fprintln(out, "Controller is ready")
```

### 8. Database final snapshot is a placeholder

The production database deletion handler generates a fake snapshot reference (`chodatabase_controller.go` line 182):

```go
// Production: record final snapshot reference (placeholder)
db.Status.FinalSnapshotRef = fmt.Sprintf("snapshot-%s-%s", db.Name, ...)
```

No actual StackGres backup/snapshot is triggered.

### 9. Cloud provider plugin not installed

Object storage `variant: object` compiles to a `kro.run/v1alpha1/ObjectStorageClaim`, but no kro cloud provider is installed to fulfill the claim. The `ChoClusterReconciler` does not deploy or configure a kro provider (AWS/GCP/Azure).

### 10. Audit defaults to no-op logger

The manager defaults to `--audit-sink=noop`. The LokiLogger is fully implemented and the `--audit-sink loki` / `--audit-sink auto` flags are wired. This is by design (safe default), but means audit is not active unless explicitly configured.

### 11. Webhooks only cover 2 of 12 CRD types

Validating webhooks exist only for `ChoApplication` and `ChoNetwork`. The other 10 CRD types have no admission validation. The two existing webhooks also have the API group mismatch (Gap 2) making them non-functional.

## Low-Severity Gaps

### 12. OIDC group sync not implemented

Membership test at `chodomainmembership_controller_test.go` line 309 is `Skip("awaiting OIDC sync implementation")`. Manual membership management works; automatic sync from OIDC group claims does not.

### 13. Compilation stability tracking incomplete

Diff test at `internal/diff/diff_test.go` line 174 is `Skip("awaiting Phase 19.3: Compilation stability tracking")`. The diff package works for resource comparison but doesn't track compilation revision stability.

### 14. ROADMAP checkboxes are misleading

All 21 phases show `[x]`. The ROADMAP header explains this means "scaffolded — types, reconciler stubs, and basic happy-path logic are in place." However, features like degraded-domain blocking (Phase 15.3) and real scanning (Phase 14) are clearly not production-complete despite being checked off.

## Corrections From Previous Report

The following items were listed as gaps in the original report but are **now implemented**:

| Original claim | Current status |
|---|---|
| "`chorister apply` is unimplemented" | **Implemented.** Uses `internal/loader` to parse multi-doc YAML. |
| "No DSL ingestion path" | **Resolved.** Architecture decision: users author chorister CRD YAML; `apply` loads and creates them. |
| "`setup`, `login`, `diff`, `admin app create`, `admin domain create`, `admin member remove` are not implemented" | **All implemented.** Full implementations with flags, validation, and output formatting. |
| "`setup` only supports dry-run" | **Implemented.** Creates namespace, installs CRDs, deploys controller, creates ChoCluster. |
| "`diff` returns not yet implemented" | **Implemented.** Wired to `internal/diff.Compare()` with table/JSON/YAML output. |
| "ChoDatabase does not create StackGres resources" | **Implemented.** Creates SGCluster + SGInstanceProfile via unstructured client. |
| "ChoQueue uses EmptyDir, no JetStream" | **Implemented.** Uses PVC VolumeClaimTemplates + JetStream config with `-js -sd /data`. |
| "ChoStorage no object variant" | **Implemented.** Creates `kro.run/v1alpha1/ObjectStorageClaim` for object variant. |
| "Compiler tests are mostly TODOs" | **Resolved.** 10 real test functions with full assertions; stale placeholder tests removed. |
| "kro integration missing" | **Implemented.** Compiler generates kro RGD for object storage; ChoStorage creates ObjectStorageClaim. |
