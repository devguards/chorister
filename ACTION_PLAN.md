# Action Plan

## Report Accuracy Assessment

The original gap report was **stale**: 10 of 13 claimed gaps described an earlier state of the codebase where CLI commands and resource backends were not yet implemented. Re-verification confirmed that `apply`, `setup`, `login`, `diff`, `admin app create`, `admin domain create`, `admin member remove` are all fully implemented. ChoDatabase creates StackGres resources, ChoQueue uses PVCs+JetStream, ChoStorage handles object variant via kro, and the compiler has 10 real tests.

The revised report identifies **3 critical**, **3 high-severity**, **5 medium-severity**, and **3 low-severity** gaps. Two of the critical gaps are bugs that must be fixed before the CLI→controller chain can work.

---

## Priority 1 — Critical Bugs (fix immediately)

### P1.1 — ChoApplication namespace mismatch (Gap 1)

`admin app create` in `cmd/chorister/main.go` line 1524 creates ChoApplication without `Namespace`. All controllers look it up in the CR's own namespace (`cho-system`). The app goes to `default`.

**Fix:** Add `Namespace: controlPlaneNamespace` to the ChoApplication ObjectMeta in `newAdminAppCreateCmd()`.

- [ ] **P1.1.1** Fix `admin app create` to set `Namespace: controlPlaneNamespace`
- [ ] **P1.1.2** Verify `admin app list/get/delete` also target `cho-system`
- [ ] **P1.1.3** Update scenario scripts that create ChoApplication via kubectl to use `-n cho-system`
- [ ] **P1.1.4** Add unit test for namespace consistency

### P1.2 — Webhook API group mismatch (Gap 2)

Webhook markers use `groups=chorister.chorister.dev` but the actual CRD group is `chorister.dev`.

**Fix:** Change the `groups=` value in both webhook markers.

- [ ] **P1.2.1** Fix marker in `internal/webhook/v1alpha1/choapplication_webhook.go` line 48
- [ ] **P1.2.2** Fix marker in `internal/webhook/v1alpha1/chonetwork_webhook.go` line 48
- [ ] **P1.2.3** Run `make manifests` to regenerate webhook configuration
- [ ] **P1.2.4** Verify generated `config/webhook/manifests.yaml` targets `chorister.dev`

---

## Priority 2 — High-Severity Gaps

### P2.1 — Degraded domain blocks promotion (Gap 4)

`IsDomainIsolated()` exists but promotion controller never calls it.

- [ ] **P2.1.1** Add isolation check in `ChoPromotionRequestReconciler.Reconcile()` before `Executing` phase
- [ ] **P2.1.2** Return `Failed` with reason `DomainIsolated` when target domain is isolated
- [ ] **P2.1.3** Un-skip test at `chopromotionrequest_controller_test.go` line 487

### P2.2 — Un-skip E2E tests (Gap 5)

14 E2E tests reference completed ROADMAP phases but remain skipped.

- [ ] **P2.2.1** Triage each skipped test: categorize as "feature exists, un-skip" vs "requires cluster infra"
- [ ] **P2.2.2** Un-skip tests where the underlying feature is implemented
- [ ] **P2.2.3** For cluster-dependent tests (Cilium, Tetragon), add CI job with `hack/setup-test-cluster.sh`

### P2.3 — Update scenario scripts to use CLI (Gap 6)

19+ STUB comments fall back to kubectl for commands that are implemented.

- [ ] **P2.3.1** Replace `kubectl apply` stubs with `chorister admin app create` in scenarios 01, 03, 05, 06, 07, 10, 11, 12
- [ ] **P2.3.2** Replace `kubectl apply` stubs with `chorister apply` in scenarios 02, 03, 06, 07
- [ ] **P2.3.3** Replace diff stub in scenario 03 with actual `chorister diff`
- [ ] **P2.3.4** Replace `admin member remove` stub in scenario 10 with actual CLI command
- [ ] **P2.3.5** Fix namespace references: scenarios using `-n default` should use `-n cho-system`

---

## Priority 3 — Medium-Severity Gaps

### P3.1 — Real vulnerability scanner (Gap 3)

- [ ] **P3.1.1** Implement `TrivyScanner` that shells out to `trivy image` or calls Trivy server API
- [ ] **P3.1.2** Wire scanner selection via `ChoCluster.Spec.Scanning.Backend` (trivy/grype/signature)
- [ ] **P3.1.3** Keep `SignatureScanner` as test/dev fallback

### P3.2 — Setup `--wait` implementation (Gap 7)

- [ ] **P3.2.1** Poll Deployment `.status.readyReplicas` until it matches `.spec.replicas`
- [ ] **P3.2.2** Add timeout (default 120s) with progress output

### P3.3 — Database final snapshot (Gap 8)

- [ ] **P3.3.1** Trigger StackGres SGBackup before marking database as Deletable
- [ ] **P3.3.2** Store actual backup reference in `status.finalSnapshotRef`

### P3.4 — Cloud provider plugin (Gap 9)

- [ ] **P3.4.1** Add kro provider installation to `ChoClusterReconciler` when object storage is used
- [ ] **P3.4.2** Document provider configuration in README

### P3.5 — Additional webhooks (Gap 11)

- [ ] **P3.5.1** Add validating webhooks for ChoPromotionRequest (require non-empty application, domain, sandbox)
- [ ] **P3.5.2** Add validating webhooks for ChoDomainMembership (require expiresAt for restricted domains)
- [ ] **P3.5.3** Consider defaulting webhooks for ChoCompute (default variant, replicas)

---

## Priority 4 — Low-Severity Gaps

### P4.1 — OIDC sync (Gap 12)

- [ ] Watch OIDC group claims and reconcile ChoDomainMembership automatically

### P4.2 — Compilation stability tracking (Gap 13)

- [ ] Track compiled output hash in ChoPromotionRequest; diff can surface revision drift

### P4.3 — ROADMAP cleanup (Gap 14)

- [ ] Relabel incomplete features: change `[x]` to `[~]` for Phase 14 (scanning), Phase 15.3 (isolation blocks promotion), Phase 19.3 (compilation stability), Phase 20.3 (budget enforcement)

---

## Sequencing

```
Immediate:   P1.1 (namespace fix) + P1.2 (webhook group fix)
             These are bugs, not features. Fix and run make manifests.

Week 1:      P2.1 (isolation blocks promotion)
             P2.3 (update scenario scripts to use CLI)
             P3.2 (setup --wait)

Week 2:      P2.2 (un-skip E2E tests)
             P3.1 (real scanner)
             P3.5 (additional webhooks)

Later:       P3.3 (database snapshots)
             P3.4 (cloud provider plugin)
             P4.* (low severity)
```

---

## Document Hygiene

Per `copilot-instructions.md`, canonical docs must be updated first:

- [ ] **Doc.1** Update `ROADMAP.md` to relabel incomplete phases with `[~]` instead of `[x]`
- [ ] **Doc.2** Update `SECURITY_COMPLIANCE.md` if webhook group fix changes compliance posture
- [ ] **Doc.3** Sync `README.md` after fixes are applied
