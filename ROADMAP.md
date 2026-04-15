# Roadmap

Implementation checklist for chorister. Each step is designed to be:

1. **AI-implementable** — scoped enough for a Claude session to produce working code
2. **Integration-testable** — verifiable against a Kind cluster with Cilium CNI
3. **Incremental** — each step builds on the previous and produces a working artifact

This document is the implementation sequence, not the product contract. README and architecture describe the target end-state; this roadmap calls out MVP slices explicitly.

---

## Testing setup

Fast controller integration tests should run first with envtest. Full end-to-end integration tests run against a local Kind cluster with Cilium. The test harness is itself an early deliverable.

### Prerequisites

- Go 1.25+
- Kubebuilder
- setup-envtest
- Kind
- kubectl
- Helm (for operator installs during testing)
- cilium-cli

---

## Phase 0: Project scaffold & test harness

- [x] **0.1 — Initialize project with Kubebuilder**
  - Run `kubebuilder init` with the project domain and repository path
  - Use Kubebuilder's standard layout as the base for the controller codebase
  - Keep the CLI as a separate binary under `cmd/chorister/`
  - Ensure the repository contains `api/`, `internal/controller/`, `config/`, `hack/`, `test/`, `cmd/controller/`, and `cmd/chorister/`
  - Makefile includes targets for `build`, `test`, `lint`, `generate`, `manifests`, and `e2e`
  - **Test:** `go build ./...` succeeds and Kubebuilder-generated manifests render without error

- [x] **0.2 — Set up envtest for controller integration tests**
  - Add envtest asset installation via `setup-envtest`
  - Add a shared Go test harness for API server and etcd lifecycle
  - Create helpers to install CRDs, create test namespaces, and wait on conditions
  - Add a trivial reconciliation test that creates a custom resource and asserts the API server accepts it
  - **Test:** `make test` runs envtest-backed tests locally without requiring Kind

- [x] **0.3 — Kind cluster provisioning script with Cilium**
  - Script: `hack/setup-test-cluster.sh`
  - Creates Kind cluster (multi-node: 1 control-plane, 2 workers)
  - Disables default CNI, installs Cilium via Helm with `hubble.enabled=true`
  - Waits for Cilium to be ready (`cilium status --wait`)
  - Installs Gateway API CRDs
  - **Test:** `cilium status` shows all agents ready, `kubectl get nodes` shows Ready

- [x] **0.4 — E2E test framework**
  - Use `sigs.k8s.io/e2e-framework` rather than raw client-go as the default harness
  - Helper functions: create namespace, apply manifest, wait-for-condition, cleanup
  - CI-friendly: `make e2e` creates cluster → runs tests → destroys cluster
  - **Test:** a trivial test that creates a namespace and asserts it exists

---

## Phase 1: CRD definitions & controller skeleton

- [x] **1.1 — Define chorister CRD types in Go**
  - `api/v1alpha1/types.go`: `ChoApplication`, `ChoDomain`, `ChoCompute`, `ChoDatabase`, `ChoQueue`, `ChoCache`, `ChoStorage`, `ChoNetwork`
  - `api/v1alpha1/types.go`: `ChoDomainMembership`, `ChoPromotionRequest`, `ChoCluster`
  - Use kubebuilder markers for validation, defaulting, printer columns
  - **Test:** `make generate` produces DeepCopy and CRD YAML; CRDs apply to Kind cluster without error

- [x] **1.2 — Controller scaffold with controller-runtime**
  - `cmd/controller/main.go`: manager setup, register all reconcilers (empty stubs)
  - Health/readiness probes at `/healthz`, `/readyz`
  - Leader election enabled
  - Dockerfile for controller image
  - **Test:** deploy controller to Kind, `kubectl get pods -n cho-system` shows Running, health endpoints return 200

- [x] **1.3 — CLI skeleton**
  - `cmd/chorister/main.go` using cobra
  - Subcommands (stubs): `setup`, `login`, `apply`, `sandbox`, `diff`, `status`, `promote`, `admin`
  - `sandbox` manages sandbox lifecycle (`create`, `destroy`, `list`); `apply` always targets an existing sandbox
  - Kubeconfig loading, context selection
  - **Test:** `chorister --help` prints usage; `chorister version` prints build info

---

## Phase 1A: Comprehensive test suite skeleton

Write the full test suite **before** implementing reconciliation logic. Every important UX scenario gets a test case now. Tests that depend on unimplemented components use `t.Skip("awaiting Phase N")`. This phase is not about making tests pass — it is about locking down what "correct" looks like so that every subsequent phase has a clear finish line.

**Principles:**
- **Unit tests** — pure logic functions (compilation, validation, diffing, cycle detection). No cluster needed. Cover edge cases.
- **envtest integration tests** — controller reconciliation flows against a real API server + etcd (no kubelet). Cover the full CRD lifecycle.
- **E2E tests** — full Kind cluster scenarios for NetworkPolicy enforcement, pod scheduling, CLI workflows. Already scaffolded in Phase 0.4.
- **Architecture-first skeletons** — if the end-state architecture commits to a safety invariant or UX flow that lands after MVP, add the test now and mark it `t.Skip("awaiting Phase N")` rather than leaving the behavior implicit.

### Unit tests — `internal/compiler/`, `internal/validation/`

- [x] **1A.1 — Compilation unit tests (t.Skip where compiler not yet built)**
  - `TestCompileCompute_DeploymentShape` — ChoCompute → Deployment+Service fields, labels, resource requests
  - `TestCompileCompute_JobVariant` — variant=job → Job manifest
  - `TestCompileCompute_CronJobVariant` — variant=cronjob → CronJob with schedule
  - `TestCompileCompute_GPUVariant` — gpu workload → Deployment/Job with `nvidia.com/gpu` limits and expected labels
  - `TestCompileCompute_ScaleToZeroVariant` — scale-to-zero variant compiles to the selected engine contract (skip until scale-to-zero engine is chosen)
  - `TestCompileCompute_HPA` — autoscaling spec → HPA manifest
  - `TestCompileCompute_PDB` — replicas>1 → PDB with correct minAvailable
  - `TestCompileDatabase_SGCluster` — ChoDatabase → SGCluster fields, instance count for ha=true/false
  - `TestCompileDatabase_Credentials` — credential Secret with expected keys (host, port, username, password, uri)
  - `TestCompileQueue_NATSResources` — ChoQueue → NATS JetStream manifests
  - `TestCompileCache_Dragonfly` — ChoCache → Dragonfly Deployment+Service, size→resource mapping
  - `TestCompileStorage_ObjectBackend` — ChoStorage object variant → provider binding/manifests for S3/GCS/Azure backend
  - `TestCompileStorage_BlockPVC` — ChoStorage block variant → PVC with expected class, size, and access mode
  - `TestCompileStorage_FilePVC` — ChoStorage file variant → RWX-capable PVC or storage-class specific manifest
  - `TestCompileNetwork_IngressHTTPRoute` — ChoNetwork ingress → Gateway API HTTPRoute
  - `TestCompileNetwork_EgressCiliumPolicy` — egress allowlist → CiliumNetworkPolicy with FQDN rules
  - `TestCompileNetwork_CrossApplicationLink` — `link` resource → HTTPRoute + ReferenceGrant + CiliumEnvoyConfig + blocking NetworkPolicy (skip until Phase 13.3)
  - Table-driven tests with edge cases: zero replicas, empty image, missing required fields
  - **Test:** `go test ./internal/compiler/...` — skipped tests report which phase unblocks them

- [x] **1A.2 — Validation unit tests**
  - `TestValidateConsumesSupplies_Mismatch` — A consumes B but B does not supply → error
  - `TestValidateConsumesSupplies_OK` — matched consumes/supplies → no error
  - `TestValidateCycleDetection` — A→B→C→A → error with cycle path
  - `TestValidateCycleDetection_DAG` — acyclic graph → no error
  - `TestValidateIngressRequiresAuth` — internet ingress without auth block → compile error
  - `TestValidateIngressAllowedIdP` — ingress auth references unapproved IdP → compile error with allowed IdPs in message
  - `TestValidateEgressWildcard` — wildcard egress → compile error
  - `TestValidateEgressUnapprovedDestination` — egress destination missing from application allowlist → compile error
  - `TestValidateComplianceEscalation` — domain sensitivity cannot weaken app compliance → error
  - `TestValidateSizingTemplate_Undefined` — size references unknown template → compile error
  - `TestValidateSizingTemplate_ErrorMessage` — undefined size error includes template name and available options
  - `TestValidateQuotaExceeded` — explicit resources exceed namespace quota → error
  - `TestValidateExplicitResourcesVsQuota` — explicit override bypasses template but still fails quota validation with quota details
  - `TestValidateArchivedResourceDependencies` — compute/queue/cache referencing archived database or queue → compile error (skip until Phase 18)
  - `TestValidateArchiveRetentionMinimum` — application archive retention below 30 days → validation error (skip until Phase 18)
  - `TestValidateRestrictedMembershipExpiryRequired` — restricted domain or regulated app membership without `expiresAt` → validation error
  - **Test:** `go test ./internal/validation/...`

- [x] **1A.3 — Diff engine unit tests**
  - `TestDiff_Added` — resource in sandbox but not prod → shows "added"
  - `TestDiff_Removed` — resource in prod but not sandbox → shows "removed"
  - `TestDiff_Changed` — field differs → shows field-level diff
  - `TestDiff_NoDifferences` — identical → empty result
  - `TestDiff_RenameShowsRemoveAndAdd` — resource rename is surfaced as remove+add rather than hidden mutation
  - `TestDiff_CompilationRevisionChange` — same DSL, different controller revision → surfaces compilation diff
  - **Test:** `go test ./internal/diff/...`

### envtest integration tests — `internal/controller/`

- [x] **1A.4 — ChoApplication lifecycle (envtest)**
  - `TestChoApplication_NamespaceCreation` — create app with 2 domains → 2 namespaces with correct labels
  - `TestChoApplication_NamespaceDeletion` — delete app → namespaces cascade-deleted via owner refs
  - `TestChoApplication_DomainAddRemove` — add domain → new namespace. Remove domain → namespace deleted
  - `TestChoApplication_DefaultDenyNetworkPolicy` — each namespace gets deny-all + DNS-allow NetworkPolicy
  - `TestChoApplication_ResourceQuota` — namespace gets ResourceQuota matching app policy
  - `TestChoApplication_LimitRange` — namespace gets LimitRange matching app policy
  - **Test:** `make test` (envtest)

- [x] **1A.5 — ChoCompute lifecycle (envtest)**
  - `TestChoCompute_CreatesDeploymentAndService` — ChoCompute → Deployment + ClusterIP Service
  - `TestChoCompute_StatusReflectsReadyReplicas` — status.ready tracks Deployment readiness
  - `TestChoCompute_JobVariant` — variant=job → Job, not Deployment
  - `TestChoCompute_CronJobVariant` — variant=cronjob → CronJob
  - `TestChoCompute_HPACreation` — autoscaling spec → HPA
  - `TestChoCompute_PDBCreation` — replicas>1 → PDB
  - `TestChoCompute_UpdateImage` — change image → Deployment updated
  - `TestChoCompute_Deletion` — delete CRD → Deployment+Service cleaned up
  - **Test:** `make test` (envtest)

- [x] **1A.6 — ChoDatabase lifecycle (envtest, skip SGCluster assertions)**
  - `TestChoDatabase_CredentialSecretCreated` — Secret with host/port/username/password/uri keys
  - `TestChoDatabase_HA_InstanceCount` — ha=true → 2+ instances in compiled output
  - `TestChoDatabase_SingleInstance` — ha=false → 1 instance
  - `TestChoDatabase_Deletion_ArchiveLifecycle` — delete → status=Archived, not removed (skip until Phase 18)
  - **Test:** `make test` (envtest)

- [x] **1A.7 — ChoQueue and ChoCache lifecycle (envtest)**
  - `TestChoQueue_CredentialSecretCreated` — NATS connection secret
  - `TestChoCache_DeploymentAndService` — Dragonfly resources created
  - `TestChoCache_SizeMapping` — small/medium/large → correct resource requests
  - **Test:** `make test` (envtest)

- [x] **1A.8 — Network policy reconciliation (envtest)**
  - `TestNetworkPolicy_ConsumesGeneratesAllowRule` — A consumes B:8080 → A→B:8080 allowed
  - `TestNetworkPolicy_NoConsumeNoAccess` — no declaration → no NetworkPolicy allow-rule
  - `TestNetworkPolicy_SupplyMismatch_StatusError` — A consumes B but B doesn't supply → error in status
  - `TestNetworkPolicy_WrongPortBlocked` — A consumes B:8080 but B exposes 9090 only → no allow rule for the undeclared port
  - `TestNetworkPolicy_DNSAlwaysAllowed` — generated deny-all policy still preserves kube-dns egress on port 53
  - `TestNetworkPolicy_CiliumL7_RestrictedDomain` — sensitivity=restricted → CiliumNetworkPolicy with L7 rules (skip until Phase 6.3)
  - `TestNetworkPolicy_EgressAllowlist` — app egress policy → CiliumNetworkPolicy FQDN rules (skip until Phase 13.1)
  - `TestNetworkPolicy_CrossApplicationLinkResources` — `link` produces HTTPRoute + ReferenceGrant + CiliumEnvoyConfig + direct-traffic deny policy (skip until Phase 13.3)
  - **Test:** `make test` (envtest)

- [x] **1A.9 — Sandbox lifecycle (envtest)**
  - `TestSandbox_CreatesIsolatedNamespace` — sandbox namespace `{app}-{domain}-sandbox-{name}`
  - `TestSandbox_CopiesDomainConfig` — sandbox gets domain's compute/db/queue/cache specs
  - `TestSandbox_OwnNetworkPolicy` — sandbox has independent deny-all policy
  - `TestSandbox_Destruction` — delete sandbox → namespace and all resources removed
  - `TestSandbox_StatefulResourceNoArchive` — DB in sandbox deleted immediately, no archive (skip until Phase 18.4)
  - `TestSandbox_IdleAutoDestroy` — idle past threshold → warning condition → destroyed (skip until Phase 20.1)
  - **Test:** `make test` (envtest)

- [x] **1A.10 — Promotion lifecycle (envtest)**
  - `TestPromotion_StatusLifecycle` — Pending → Approved → Executing → Completed
  - `TestPromotion_InsufficientApprovals` — stays Pending until required approvals met
  - `TestPromotion_ApprovalRoleValidation` — approval from disallowed role does not satisfy policy
  - `TestPromotion_CopiesCompiledManifests` — production namespace updated on approval
  - `TestPromotion_StoresDiffAndCompiledRevision` — request/status captures resource diff and `compiledWithRevision`
  - `TestPromotion_BlockedByDegradedDomain` — degraded domain → promotion rejected (skip until Phase 15.3)
  - `TestPromotion_ImageScanBlock` — critical CVE → promotion blocked (skip until Phase 14.1)
  - **Test:** `make test` (envtest)

- [x] **1A.11 — RBAC & membership (envtest)**
  - `TestMembership_DeveloperRoleBinding` — developer → edit RoleBinding in sandbox
  - `TestMembership_ViewerRoleBinding` — viewer → view RoleBinding
  - `TestMembership_ProductionViewOnly` — all human roles get view-only in production namespace
  - `TestMembership_Expiry` — expired membership → RoleBinding deleted, status=expired
  - `TestMembership_RestrictedDomainRequiresExpiry` — restricted domain membership without `expiresAt` is rejected
  - `TestMembership_OIDCGroupRemoval_DeprovisionsBindings` — subject removed from synced OIDC group → membership/RoleBinding removed (skip until OIDC sync lands)
  - `TestMembership_OrgAdmin` — org-admin → admin RoleBinding
  - **Test:** `make test` (envtest)

- [x] **1A.12 — ChoCluster bootstrap (envtest)**
  - `TestChoCluster_OperatorInstallation` — ChoCluster triggers operator installations (skip operator CRD checks until Phase 12)
  - `TestChoCluster_OperatorReinstallation` — deleted operator → controller reinstalls
  - `TestChoCluster_SizingTemplates` — ChoCluster sizing templates available for resource compilation
  - `TestChoCluster_FinOpsRates` — cost rates readable from ChoCluster spec (skip until Phase 20.2)
  - `TestChoCluster_DefaultSizingTemplatesInstalled` — `chorister setup`/ChoCluster bootstrap creates baseline templates for compute, database, cache, and queue
  - `TestChoCluster_AuditWriteFailureBlocksReconciliation` — synchronous audit sink failure marks reconcile as failed and avoids partial apply (skip until Phase 11.2)
  - **Test:** `make test` (envtest)

### E2E scenario tests — `test/e2e/`

- [x] **1A.13 — Developer daily workflow (e2e, Kind+Cilium)**
  - `TestE2E_DeveloperWorkflow` — full scenario:
    1. Create ChoApplication with 2 domains (payments, auth)
    2. `chorister sandbox create --domain payments --name alice`
    3. `chorister apply --domain payments --sandbox alice` with compute + database
    4. Assert resources running in sandbox namespace
    5. `chorister diff --domain payments --sandbox alice` shows differences from prod (empty prod)
    6. `chorister promote --domain payments --sandbox alice` → ChoPromotionRequest created
    7. Approve promotion → production namespace updated
    8. `chorister diff` → no differences
    9. `chorister sandbox destroy --domain payments --name alice` → namespace cleaned up
    10. Re-run `chorister diff` after a controller revision change → compilation drift is surfaced even when DSL is unchanged (skip until Phase 21)
  - Skip sub-steps that depend on unimplemented phases; run the rest
  - **Test:** `make e2e`

- [x] **1A.14 — Network isolation (e2e, Kind+Cilium)**
  - `TestE2E_NetworkIsolation` — full scenario:
    1. Create app with payments (consumes auth:8080) and auth (supplies :8080)
    2. Deploy test pods in both namespaces
    3. Assert payments→auth:8080 succeeds
    4. Assert payments→auth:9090 blocked
    5. Assert unrelated-namespace→auth:8080 blocked
    6. Assert all outbound traffic except declared egress blocked (skip FQDN until Phase 13)
  - **Test:** `make e2e`

- [x] **1A.15 — Cross-application link flow (e2e, Kind+Cilium)**
  - `TestE2E_CrossApplicationLink` — app A links to app B through the internal gateway:
    1. Create two applications with an approved bilateral `link`
    2. Assert direct pod-to-pod cross-application traffic is blocked
    3. Assert HTTPRoute + ReferenceGrant are present
    4. Assert traffic succeeds only through the gateway path
    5. Assert rate limiting / auth policy manifests are attached (skip live rate-limit verification until Phase 13.3)
  - **Test:** `make e2e`

- [x] **1A.16 — Production safety (e2e)**
  - `TestE2E_CannotApplyToProd` — `chorister apply` targeting production namespace → rejected
  - `TestE2E_PromotionRequiresApproval` — promotion with 0 approvals does not modify prod
  - `TestE2E_ProductionRBACViewOnly` — developer ServiceAccount cannot create/update resources in production namespace
  - **Test:** `make e2e`

- [x] **1A.17 — Compliance and policy enforcement (e2e, skip per profile)**
  - `TestE2E_EssentialCompliance` — no privileged pods, non-root enforced (skip until Phase 10)
  - `TestE2E_StandardCompliance` — adds image scanning gate on promotion (skip until Phase 14)
  - `TestE2E_RegulatedCompliance` — seccomp, AppArmor, Tetragon TracingPolicy (skip until Phase 15.2)
  - `TestE2E_IngressRequiresAuth` — internet ingress without auth → rejected (skip until Phase 10.3)
  - **Test:** `make e2e`

- [x] **1A.18 — Incident response and archive safety (e2e, skip where deferred)**
  - `TestE2E_AdminIsolateDomain` — `chorister admin isolate` tightens NetworkPolicy and freezes promotions (skip until incident workflow lands)
  - `TestE2E_ArchivedResourceBlocksPromotion` — removing a production database archives it and any dependent compute promotion is rejected until refs are removed (skip until Phase 18)
  - `TestE2E_AdminDeleteArchivedResource` — archived stateful resource requires explicit admin delete after retention window (skip until Phase 18)
  - **Test:** `make e2e`

### CLI unit tests — `cmd/chorister/`

- [x] **1A.19 — CLI argument parsing and safety rails**
  - `TestCLI_ApplyRefusesProductionNamespace` — hardcoded rejection for prod targets
  - `TestCLI_ApplyRequiresSandboxFlag` — apply without `--sandbox` → error
  - `TestCLI_SandboxCreateRequiresDomain` — `sandbox create` without `--domain` → error
  - `TestCLI_SandboxCreateBudgetExceeded` — sandbox create rejected when estimated monthly cost would exceed domain budget (skip until Phase 20)
  - `TestCLI_DiffOutputFormat` — diff output is human-readable (added/changed/removed)
  - `TestCLI_DiffOutputIncludesCompilationRevision` — diff surfaces controller revision drift when manifests change without DSL edits
  - `TestCLI_PromoteCreatesCRD` — promote command creates ChoPromotionRequest CRD
  - `TestCLI_ExportOutputsValidYAML` — export produces valid K8s manifests (skip until Phase 15.4)
  - `TestCLI_SetupIdempotent` — running setup twice is safe (skip until Phase 12.2)
  - `TestCLI_AdminMemberAudit_FlagsStale` — `admin member audit` reports stale memberships / expired access (skip until membership audit lands)
  - `TestCLI_AdminResourceDeleteArchived` — `admin resource delete --archived` requires explicit archived target and emits audit-friendly confirmation output (skip until Phase 18)
  - `TestCLI_AdminUpgradeBlueGreen` — `admin upgrade` manages revision install / promote / rollback flags safely (skip until Phase 21)
  - `TestCLI_ErrorMessages_Actionable` — user-facing errors include blocked action, violated invariant, and next remediation step
  - **Test:** `go test ./cmd/chorister/...`

---

## Phase 2: Core reconciliation — ChoApplication & namespace management

- [x] **2.1 — ChoApplication reconciler → namespace creation**
  - Reconciler watches `ChoApplication`
  - For each domain in `.spec.domains`, ensure namespace `{app}-{domain}` exists
  - Apply standard labels: `chorister.dev/application`, `chorister.dev/domain`
  - Set owner references for cleanup
  - **Test:** create `ChoApplication` with 2 domains → assert 2 namespaces exist with correct labels. Delete application → namespaces deleted.

- [x] **2.2 — Default deny NetworkPolicy per namespace**
  - When namespace is created, controller creates a deny-all ingress+egress NetworkPolicy
  - Allow DNS egress (kube-dns) so pods can resolve
  - **Test:** create application → assert NetworkPolicy exists in each domain namespace. Deploy a pod → confirm it cannot reach pods in other namespaces.

- [x] **2.3 — Resource quota and LimitRange from application policy**
  - Read `.spec.policy.quotas.defaultPerDomain`
  - Create ResourceQuota and LimitRange in each domain namespace
  - **Test:** create application with quota config → assert ResourceQuota exists → attempt to create pod exceeding quota → expect rejection

---

## Phase 3: Compute resource compilation

- [x] **3.1 — ChoCompute reconciler → Deployment + Service**
  - Watch `ChoCompute` CRD
  - Compile to: kro RGD + instance that render Deployment (with resource requests/limits, liveness/readiness probes placeholder) and Service (ClusterIP)
  - Apply in target namespace
  - Update `.status` with ready replica count
  - **Test:** create `ChoCompute` → assert Deployment and Service exist → wait for pods Ready → check `.status.ready == true`

- [x] **3.2 — HPA and PDB for compute**
  - If `replicas > 1`, create PodDisruptionBudget (minAvailable = replicas-1)
  - If `autoscaling` spec present, create HorizontalPodAutoscaler
  - **Test:** create ChoCompute with replicas=3 → assert PDB exists with minAvailable=2. Create with autoscaling → assert HPA exists.

- [x] **3.3 — Compute variants: Job and CronJob**
  - If `variant = "job"`, compile to K8s Job
  - If `variant = "cronjob"`, compile to CronJob with schedule
  - **Test:** create ChoCompute variant=job → assert Job runs to completion. Create variant=cronjob → assert CronJob is created with correct schedule.

---

## Phase 4: Database resource compilation (StackGres)

- [x] **4.1 — Install StackGres operator in test cluster**
  - Add StackGres install to `hack/setup-test-cluster.sh`
  - Verify SGCluster CRD is available
  - **Test:** `kubectl get crd sgclusters.stackgres.io` succeeds

- [x] **4.2 — ChoDatabase reconciler → SGCluster**
  - Watch `ChoDatabase` CRD
  - Compile to: kro RGD + instance that render SGCluster + SGPoolingConfig (PgBouncer) + SGBackupConfig
  - `ha: false` → 1 instance. `ha: true` → 2+ instances with Patroni
  - **Test:** create `ChoDatabase` with ha=false → assert SGCluster with 1 instance. Create with ha=true → assert 2+ instances. Wait for cluster ready.

- [x] **4.3 — Database secret wiring**
  - Controller creates a Secret with connection string, username, password
  - Secret name follows convention: `{domain}--database--{name}-credentials`
  - **Test:** create ChoDatabase → assert Secret exists with expected keys (host, port, username, password, uri)

---

## Phase 5: Queue and cache compilation

- [x] **5.1 — Install NATS operator in test cluster**
  - Add NATS operator install to test cluster script
  - **Test:** NATS CRDs available in cluster

- [x] **5.2 — ChoQueue reconciler → NATS JetStream**
  - Watch `ChoQueue` CRD
  - Compile to kro RGD + instance that render NATS JetStream resources (StatefulSet or operator CR)
  - Expose connection credentials as Secret
  - **Test:** create ChoQueue → assert NATS resources exist → verify connectivity from a test pod

- [x] **5.3 — ChoCache reconciler → Dragonfly**
  - Watch `ChoCache` CRD
  - Compile to kro RGD + instance that render Dragonfly Deployment + Service
  - Size mapping: small/medium/large → resource requests
  - **Test:** create ChoCache → assert Deployment + Service exist → verify Redis-compatible connectivity from test pod

---

## Phase 6: Network resource — consumes/supplies enforcement

- [x] **6.1 — Compile consumes/supplies → NetworkPolicy**
  - When ChoApplication has `consumes`/`supplies` declarations, generate allow-rules in NetworkPolicy
  - Only the declared port + namespace selector. Everything else stays denied.
  - **Test:** domain A consumes domain B on port 8080 → deploy pods in both → pod in A can reach B:8080 → pod in A cannot reach B:9090 → pod in C cannot reach B:8080

- [x] **6.2 — Supply/consume validation**
  - If domain A consumes domain B, but B does not supply → reconciliation error on ChoApplication status
  - Cycle detection: A→B→C→A → error
  - **Test:** create application with mismatched consumes/supplies → assert error in `.status.conditions`. Fix the mismatch → assert error clears.

- [x] **6.3 — CiliumNetworkPolicy for L7 filtering**
  - For domains with `sensitivity = "restricted"`, generate CiliumNetworkPolicy with L7 HTTP path rules
  - **Test:** create restricted domain with L7 rules → assert CiliumNetworkPolicy exists → verify path-level filtering (allowed path works, disallowed path is blocked)

---

## Phase 7: Sandbox lifecycle

- [x] **7.1 — Sandbox creation and isolation**
  - `ChoSandbox` CRD or annotation-based
  - Controller creates namespace `{app}-{domain}-sandbox-{name}`
  - Copies domain config into sandbox namespace
  - Each sandbox is fully isolated (own NetworkPolicy, own resources)
  - **Test:** create sandbox → assert namespace exists with all resources from domain spec → assert sandbox cannot reach production namespace

- [x] **7.2 — Sandbox destruction and cleanup**
  - Delete sandbox CRD → controller deletes namespace and all resources
  - Owner references ensure cascade
  - **Test:** create sandbox → verify resources exist → delete sandbox → verify namespace gone

- [x] **7.3 — CLI: `chorister apply` targets sandbox only**
  - CLI `apply` command reads the DSL file and creates/updates CRDs in an existing sandbox namespace
  - `chorister sandbox` remains the lifecycle command group (`create`, `destroy`, `list`), not a second apply surface
  - Refuses to target production namespace (hardcoded check + server-side rejection)
  - **Test:** `chorister apply --domain payments --sandbox alice` succeeds. Any attempt to apply to prod namespace is rejected.

---

## Phase 8: Diff and promotion

- [x] **8.1 — Diff engine: sandbox vs production**
  - Compare compiled manifests between sandbox and production namespaces
  - Output human-readable diff (resource-level: added, changed, removed)
  - **Test:** apply different configs to sandbox and prod → `chorister diff` shows differences → apply same config → diff shows no changes

- [x] **8.2 — ChoPromotionRequest reconciler**
  - Create `ChoPromotionRequest` CRD
  - Status lifecycle: Pending → Approved → Executing → Completed/Failed
  - Controller copies compiled Blueprint from sandbox namespace to production namespace on approval
  - **Test:** create ChoPromotionRequest → assert status=Pending → simulate approval (patch status) → assert production namespace updated → status=Completed

- [x] **8.3 — Approval gate enforcement**
  - Read promotion policy from ChoApplication (requiredApprovers, allowedRoles)
  - Controller validates approvals before proceeding
  - Block if insufficient approvals
  - **Test:** create promotion with policy requiring 2 approvers → add 1 approval → assert still Pending → add 2nd → assert Executing then Completed

---

## Phase 9: Identity & access control

- [x] **9.1 — ChoDomainMembership reconciler → RoleBinding**
  - Watch `ChoDomainMembership` CRD
  - Map role to namespace-scoped access in sandboxes: org-admin→admin, domain-admin→admin, developer→edit, viewer→view
  - Create RoleBinding in domain namespace
  - **Test:** create membership for alice as developer in payments → assert RoleBinding exists → verify alice can create pods in payments namespace → verify alice cannot create pods in other namespaces

- [x] **9.2 — Membership expiry enforcement**
  - Controller checks `expiresAt` on reconciliation
  - Expired memberships: delete RoleBinding, update membership status
  - **Test:** create membership with expiresAt in the past → assert RoleBinding is removed → status shows expired

- [x] **9.3 — Production RBAC lockdown**
  - Production namespaces: all human roles get view-only (no edit)
  - Only controller ServiceAccount can modify production resources
  - **Test:** create developer and org-admin memberships → assert both can administer sandboxes as expected → assert both are view-only in production

---

## Phase 10: OPA/Gatekeeper policy enforcement

- [x] **10.1 — Install Gatekeeper and constraint templates**
  - Controller installs Gatekeeper during ChoCluster reconciliation
  - Constraint templates: no-privileged, no-hostPID, no-hostNetwork, drop-all-caps, non-root, image-allowlist
  - **Test:** Gatekeeper pods running. Create privileged pod → rejected. Create non-root pod → accepted.

- [x] **10.2 — Compliance-profile-driven constraints**
  - `essential`: basic pod security (no privilege escalation, non-root)
  - `regulated`: add seccomp RuntimeDefault, AppArmor
  - Controller installs the right set of constraints based on ChoApplication compliance profile
  - **Test:** create application with `compliance: essential` → assert Level 1 constraints exist. Update to `compliance: regulated` → assert seccomp constraint added.

- [x] **10.3 — Compile-time guardrails**
  - Controller rejects manifests at compile time for: internet ingress without auth, wildcard egress, egress to unapproved destinations
  - **Test:** submit ChoNetwork with ingress from internet and no auth block → assert compile error in status. Add auth block → assert success.

---

## Phase 11: Observability stack

- [x] **11.1 — Grafana LGTM installation via ChoCluster**
  - Controller reconciles ChoCluster to install: Grafana Alloy, Mimir, Loki, Tempo
  - All configured to use local PVCs (for Kind; object storage in real clusters)
  - **Test:** create ChoCluster → assert Alloy, Mimir, Loki, Tempo pods running → Grafana accessible

- [x] **11.2 — Audit event logging to Loki**
  - Controller writes structured JSON audit events to Loki on every reconciliation
  - Events: who, what, when, domain, application, action, result
  - Synchronous: if Loki write fails, reconciliation fails
  - **Test:** create/update a ChoCompute → query Loki for audit event → assert event contains expected fields

- [x] **11.3 — Controller-generated Grafana dashboards**
  - Per-domain dashboard ConfigMap: pod status, resource usage, network flows
  - Grafana sidecar auto-loads dashboards
  - **Test:** create application with domain → assert Grafana dashboard ConfigMap exists in monitoring namespace

---

## Phase 12: ChoCluster — full stack bootstrap

- [x] **12.1 — ChoCluster reconciler: operator lifecycle**
  - ChoCluster CRD defines which operators to install and their versions
  - Controller installs/upgrades: kro, StackGres, NATS operator, Dragonfly operator, cert-manager, Gatekeeper
  - If operator is deleted, controller reinstalls on next reconciliation
  - **Test:** create ChoCluster → assert all operators running. Delete StackGres operator → wait for reconciliation → assert reinstalled.

- [x] **12.2 — `chorister setup` CLI command**
  - Installs controller Deployment + CRDs into `cho-system` namespace
  - Creates default ChoCluster CRD to trigger stack bootstrap
  - Idempotent: running twice is safe
  - **Test:** run `chorister setup` on clean cluster → assert controller running + CRDs registered. Run again → no errors, same state.

- [x] **12.3 — Encrypted StorageClass validation**
  - Controller validates that an encrypted StorageClass exists during setup
  - Warn if not found (Kind won't have one, but real clusters must)
  - **Test:** controller starts on Kind → warning in logs about missing encrypted StorageClass (non-blocking for dev)

---

## Phase 13: Ingress & egress networking

- [x] **13.1 — Egress allowlist enforcement**
  - Read `policy.network.egress.allowlist` from ChoApplication
  - Generate CiliumNetworkPolicy with FQDN-based egress rules
  - Block all other egress (except DNS, intra-cluster)
  - **Test:** create application with egress allowlist for `httpbin.org` → pod can reach httpbin.org → pod cannot reach other external hosts

- [x] **13.2 — Ingress with JWT auth requirement**
  - ChoNetwork with `from = "internet"` requires auth block
  - Compile to Gateway API HTTPRoute + CiliumNetworkPolicy with JWT verification
  - **Test:** create ingress with JWT config → assert HTTPRoute + CiliumNetworkPolicy exist. Create ingress without auth → assert compile error.

- [x] **13.3 — Cross-application links via Gateway API**
  - `link` in ChoApplication compiles to: HTTPRoute (consumer) + ReferenceGrant (supplier) + CiliumNetworkPolicy (L7) + CiliumEnvoyConfig (rate limit)
  - **Test:** create two applications with a link between them → assert HTTPRoute, ReferenceGrant, rate limit config exist → traffic flows through gateway

---

## Phase 14: Security scanning & vulnerability management

- [x] **14.1 — Image scanning before promotion**
  - Controller runs Trivy scan on all images in a ChoPromotionRequest
  - Block promotion if critical CVEs found (`standard`+)
  - Store results in ChoVulnerabilityReport CRD
  - **Test:** create promotion request with image containing known CVE → assert promotion blocked. Use clean image → promotion proceeds.

- [x] **14.2 — Continuous vulnerability scanning CronJobs**
  - For `standard`+ applications, controller creates CronJob per domain (daily re-scan)
  - Results written to ChoVulnerabilityReport CRDs
  - **Test:** create `standard` application with deployed images → assert CronJob exists → trigger manual run → assert ChoVulnerabilityReport created

- [x] **14.3 — kube-bench periodic validation**
  - Controller creates kube-bench CronJob for cluster hardening checks
  - Results stored in ChoCluster.status.cisBenchmark
  - **Test:** assert kube-bench CronJob exists → trigger run → assert results in ChoCluster status

---

## Phase 15: Advanced features

- [x] **15.1 — Data sensitivity enforcement**
  - Domain `sensitivity` field: public/internal/confidential/restricted
  - `confidential` → enforce TLS for all cross-domain traffic
  - `restricted` → require L7 policy, membership expiry, full Tetragon
  - **Test:** create domain with sensitivity=restricted → assert CiliumNetworkPolicy has L7 rules, memberships require expiresAt

- [x] **15.2 — Tetragon runtime detection (`regulated`)**
  - Install Tetragon in test cluster
  - Controller generates TracingPolicy CRDs for restricted domains or `regulated` applications
  - Monitor: syscall anomalies, file integrity, unexpected process execution
  - **Test:** install Tetragon → create `regulated` application → assert TracingPolicy exists → exec into pod and trigger a monitored syscall → assert Tetragon event generated

- [x] **15.3 — Service health baseline and incident response**
  - Controller monitors pod health, deployment progress, database status
  - Degraded domain → flag in status, block further promotions
  - `chorister admin isolate` → tighten NetworkPolicy, freeze promotions
  - **Test:** create domain → crash pods intentionally → assert domain status=Degraded → assert promotion is blocked → recover pods → status clears

- [x] **15.4 — `chorister export` for GitOps**
  - Export compiled Blueprint as static YAML files
  - Compatible with ArgoCD/Flux directory structure
  - **Test:** create domain with compute + database → `chorister export` → assert output directory contains valid K8s manifests → `kubectl apply --dry-run=server` succeeds

---

## Phase 16: cert-manager & TLS

- [x] **16.1 — cert-manager installation and wildcard certs**
  - Controller installs cert-manager via ChoCluster reconciliation
  - Creates ClusterIssuer (Let's Encrypt or self-signed for dev)
  - Wildcard Certificate for application domains
  - **Test:** cert-manager pods running → create Certificate → assert TLS secret generated

- [x] **16.2 — Automatic TLS for cross-domain traffic**
  - For `confidential` and `restricted` domains, enforce mTLS via Cilium WireGuard or cert-manager
  - **Test:** create two confidential domains with consumes/supplies → verify traffic is encrypted (inspect Cilium encryption status)

---

## Phase 17: Secret management

- [x] **17.1 — Secret slot declaration and auto-generation**
  - Blueprint declares typed secret slots
  - Sandbox: auto-generate secrets (random password for database, etc.)
  - Store as K8s Secrets with standard naming
  - **Test:** create ChoDatabase in sandbox → assert database credential Secret auto-generated → values are non-empty random strings

- [x] **17.2 — External secret backend integration**
  - Support external backends: GCP Secret Manager, AWS Secrets Manager (via ExternalSecrets operator or direct)
  - Production environments reference external secrets
  - **Test:** (mock) configure external secret reference → assert ExternalSecret CR created → mock backend → assert K8s Secret synced

---

## Phase 18: Stateful resource deletion safety

- [x] **18.1 — Archive lifecycle for stateful resources**
  - When a stateful resource (ChoDatabase, ChoQueue, ChoStorage) is removed from the DSL and promoted, controller transitions it to `Archived` instead of deleting
  - Archived resources: data intact (read-only), connections refused, backups continue
  - Add `status.lifecycle` (Active/Archived/Deletable), `status.archivedAt`, `status.deletableAfter` fields
  - **Test:** create ChoDatabase → promote with database removed from DSL → assert database status=Archived, not deleted → assert data still accessible via backup tools → assert dependent ChoCompute gets compile error

- [x] **18.2 — Archive retention period enforcement**
  - Controller enforces minimum 30-day archive period (configurable upward via `policy.archiveRetention`)
  - After retention period, controller transitions resource to `Deletable` (still not deleted)
  - **Test:** create archived database → assert it cannot be deleted before retention period → advance time (or set short retention for test) → assert status=Deletable

- [x] **18.3 — Explicit deletion of archived resources**
  - `chorister admin resource delete --archived <resource>` finalizes deletion
  - Controller takes final backup snapshot to object storage before deletion
  - Deletion is an audited action (Loki event)
  - **Test:** archive a database → wait for Deletable → run delete command → assert final snapshot exists in object storage → assert resource fully removed → assert audit event logged

- [x] **18.4 — Sandbox exemption from archive lifecycle**
  - Stateful resources in sandbox namespaces are deleted immediately on sandbox destruction
  - No archive lifecycle for sandboxes
  - **Test:** create sandbox with database → destroy sandbox → assert database deleted immediately (no Archived state)

---

## Phase 19: Controller upgrade & CRD versioning

- [ ] **19.1 — Controller revision labeling**
  - Controller reads its revision name from config and only reconciles namespaces with matching `chorister.dev/rev` label
  - Untagged namespaces default to the revision tagged `stable` in ChoCluster
  - **Test:** deploy controller with revision "1-0" → create namespace with `chorister.dev/rev: "1-0"` → assert reconciled. Create namespace with different rev → assert ignored.

- [ ] **19.2 — Blue-green controller upgrade flow**
  - `chorister admin upgrade --revision <new>` deploys a new controller alongside the old one
  - `chorister admin upgrade --promote <rev>` retags all namespaces and marks revision as stable
  - `chorister admin upgrade --rollback <rev>` removes canary revision
  - **Test:** deploy v1 (stable) → deploy v2 (canary) → retag one namespace to v2 → assert v2 reconciles it → promote v2 → assert all namespaces on v2 → old controller idle

- [ ] **19.3 — Compilation stability tracking**
  - Controller records `compiledWithRevision` in each resource's status
  - `chorister diff` shows when compiled output differs between controller revisions even if DSL is unchanged
  - **Test:** compile resource with v1 → upgrade to v2 (different output) → `chorister diff` shows the compilation difference

---

## Phase 20: Sandbox lifecycle & FinOps quotas

- [ ] **20.1 — Sandbox idle detection and auto-destroy**
  - Controller tracks last `chorister apply` timestamp per sandbox
  - Sandboxes idle longer than `policy.sandbox.maxIdleDays` are auto-destroyed
  - 24h warning via status condition before destruction
  - **Test:** create sandbox → wait beyond idle threshold (use short interval for test) → assert warning condition → assert sandbox destroyed

- [ ] **20.2 — FinOps cost estimation engine**
  - Define `ChoCluster.spec.finops.rates` for per-unit cost rates (CPU/hour, memory/GB-hour, storage/GB-month, per-size flat rates)
  - Controller estimates cost of each sandbox based on resource declarations and rates
  - Cost visible in sandbox status: `status.sandbox.estimatedMonthlyCost`
  - **Test:** set rates in ChoCluster → create sandbox with known resources → assert estimated cost matches expected calculation

- [ ] **20.3 — Domain sandbox budget enforcement**
  - `policy.sandbox.defaultBudgetPerDomain` in ChoApplication, overridable per domain
  - `chorister sandbox create` rejected if domain total would exceed budget (with cost breakdown in error)
  - Alert at configurable threshold (default 80%)
  - **Test:** set $100 budget → create sandbox costing $60 → succeeds → create another $60 sandbox → rejected with budget exceeded error → assert alert at 80% threshold

---

## Phase 21: Resource sizing templates

- [ ] **21.1 — Sizing template definitions in ChoCluster**
  - `ChoCluster.spec.sizingTemplates` with per-resource-type named templates (database, cache, queue)
  - `chorister setup` creates sensible defaults
  - `size` field in DSL references template name → compile error if template doesn't exist
  - **Test:** define templates in ChoCluster → create ChoDatabase with `size: "medium"` → assert resource requests match template. Use undefined size → assert compile error.

- [ ] **21.2 — Explicit resource override**
  - DSL allows explicit `cpu`, `memory`, `storage` fields that bypass templates entirely
  - Controller validates against namespace ResourceQuota
  - **Test:** create ChoDatabase with explicit cpu/memory/storage → assert values used instead of template. Exceed quota → assert rejection.

---

## Implementation notes for AI sessions

### Session scope

Each checkbox above is scoped for a single AI coding session. The pattern:

1. Read the relevant architecture section for context
2. Implement the feature (types, reconciler logic, CLI command)
3. Write integration test using the e2e framework
4. Run `make e2e` against Kind+Cilium cluster to verify
5. Commit

### Test cluster requirements

| Component | Required for | Install method |
|---|---|---|
| Kind | All phases | `go install sigs.k8s.io/kind@latest` |
| Cilium | Phase 6+, 10+, 13+ | Helm (in setup script) |
| StackGres | Phase 4 | Helm (in setup script) |
| NATS operator | Phase 5 | Helm (in setup script) |
| Gatekeeper | Phase 10 | Helm (in setup script) |
| Tetragon | Phase 15.2 | Helm (in setup script) |
| cert-manager | Phase 16 | Helm (in setup script) |
| Gateway API CRDs | Phase 13 | kubectl apply (in setup script) |

### What can be deferred

- OIDC login flow (mock with ServiceAccount tokens for testing)
- Real object storage backends (use PVCs in Kind)
- Multi-cluster (Phase 18+ in architecture doc)
- AutoMQ/Strimzi streaming queue (NATS covers standard queue)
- Real external secret backends (mock in tests)
