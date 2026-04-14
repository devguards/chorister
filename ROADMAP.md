# Roadmap

Implementation checklist for chorister. Each step is designed to be:

1. **AI-implementable** — scoped enough for a Claude session to produce working code
2. **Integration-testable** — verifiable against a Kind cluster with Cilium CNI
3. **Incremental** — each step builds on the previous and produces a working artifact

This document is the implementation sequence, not the product contract. README and architecture describe the target end-state; this roadmap calls out MVP slices explicitly.

---

## Test cluster setup

All integration tests run against a local Kind cluster with Cilium. The test harness is itself an early deliverable.

### Prerequisites

- Go 1.22+
- Kind
- kubectl
- Helm (for operator installs during testing)
- cilium-cli

---

## Phase 0: Project scaffold & test harness

- [ ] **0.1 — Initialize Go module and project structure**
  - `go mod init github.com/chorister-dev/chorister`
  - Directory layout: `cmd/chorister/`, `cmd/controller/`, `internal/`, `api/v1alpha1/`, `test/e2e/`
  - Makefile with targets: `build`, `test`, `lint`, `generate`, `e2e`
  - **Test:** `go build ./...` succeeds

- [ ] **0.2 — Kind cluster provisioning script with Cilium**
  - Script: `hack/setup-test-cluster.sh`
  - Creates Kind cluster (multi-node: 1 control-plane, 2 workers)
  - Disables default CNI, installs Cilium via Helm with `hubble.enabled=true`
  - Waits for Cilium to be ready (`cilium status --wait`)
  - Installs Gateway API CRDs
  - **Test:** `cilium status` shows all agents ready, `kubectl get nodes` shows Ready

- [ ] **0.3 — E2E test framework**
  - Use `sigs.k8s.io/e2e-framework` or raw client-go
  - Helper functions: create namespace, apply manifest, wait-for-condition, cleanup
  - CI-friendly: `make e2e` creates cluster → runs tests → destroys cluster
  - **Test:** a trivial test that creates a namespace and asserts it exists

---

## Phase 1: CRD definitions & controller skeleton

- [ ] **1.1 — Define chorister CRD types in Go**
  - `api/v1alpha1/types.go`: `ChoApplication`, `ChoDomain`, `ChoCompute`, `ChoDatabase`, `ChoQueue`, `ChoCache`, `ChoStorage`, `ChoNetwork`
  - `api/v1alpha1/types.go`: `ChoDomainMembership`, `ChoPromotionRequest`, `ChoCluster`
  - Use kubebuilder markers for validation, defaulting, printer columns
  - **Test:** `make generate` produces DeepCopy and CRD YAML; CRDs apply to Kind cluster without error

- [ ] **1.2 — Controller scaffold with controller-runtime**
  - `cmd/controller/main.go`: manager setup, register all reconcilers (empty stubs)
  - Health/readiness probes at `/healthz`, `/readyz`
  - Leader election enabled
  - Dockerfile for controller image
  - **Test:** deploy controller to Kind, `kubectl get pods -n cho-system` shows Running, health endpoints return 200

- [ ] **1.3 — CLI skeleton**
  - `cmd/chorister/main.go` using cobra
  - Subcommands (stubs): `setup`, `login`, `apply`, `sandbox`, `diff`, `status`, `promote`, `admin`
  - `sandbox` manages sandbox lifecycle (`create`, `destroy`, `list`); `apply` always targets an existing sandbox
  - Kubeconfig loading, context selection
  - **Test:** `chorister --help` prints usage; `chorister version` prints build info

---

## Phase 2: Core reconciliation — ChoApplication & namespace management

- [ ] **2.1 — ChoApplication reconciler → namespace creation**
  - Reconciler watches `ChoApplication`
  - For each domain in `.spec.domains`, ensure namespace `{app}-{domain}` exists
  - Apply standard labels: `chorister.dev/application`, `chorister.dev/domain`
  - Set owner references for cleanup
  - **Test:** create `ChoApplication` with 2 domains → assert 2 namespaces exist with correct labels. Delete application → namespaces deleted.

- [ ] **2.2 — Default deny NetworkPolicy per namespace**
  - When namespace is created, controller creates a deny-all ingress+egress NetworkPolicy
  - Allow DNS egress (kube-dns) so pods can resolve
  - **Test:** create application → assert NetworkPolicy exists in each domain namespace. Deploy a pod → confirm it cannot reach pods in other namespaces.

- [ ] **2.3 — Resource quota and LimitRange from application policy**
  - Read `.spec.policy.quotas.defaultPerDomain`
  - Create ResourceQuota and LimitRange in each domain namespace
  - **Test:** create application with quota config → assert ResourceQuota exists → attempt to create pod exceeding quota → expect rejection

---

## Phase 3: Compute resource compilation

- [ ] **3.1 — ChoCompute reconciler → Deployment + Service**
  - Watch `ChoCompute` CRD
  - Compile to: kro RGD + instance that render Deployment (with resource requests/limits, liveness/readiness probes placeholder) and Service (ClusterIP)
  - Apply in target namespace
  - Update `.status` with ready replica count
  - **Test:** create `ChoCompute` → assert Deployment and Service exist → wait for pods Ready → check `.status.ready == true`

- [ ] **3.2 — HPA and PDB for compute**
  - If `replicas > 1`, create PodDisruptionBudget (minAvailable = replicas-1)
  - If `autoscaling` spec present, create HorizontalPodAutoscaler
  - **Test:** create ChoCompute with replicas=3 → assert PDB exists with minAvailable=2. Create with autoscaling → assert HPA exists.

- [ ] **3.3 — Compute variants: Job and CronJob**
  - If `variant = "job"`, compile to K8s Job
  - If `variant = "cronjob"`, compile to CronJob with schedule
  - **Test:** create ChoCompute variant=job → assert Job runs to completion. Create variant=cronjob → assert CronJob is created with correct schedule.

---

## Phase 4: Database resource compilation (StackGres)

- [ ] **4.1 — Install StackGres operator in test cluster**
  - Add StackGres install to `hack/setup-test-cluster.sh`
  - Verify SGCluster CRD is available
  - **Test:** `kubectl get crd sgclusters.stackgres.io` succeeds

- [ ] **4.2 — ChoDatabase reconciler → SGCluster**
  - Watch `ChoDatabase` CRD
  - Compile to: kro RGD + instance that render SGCluster + SGPoolingConfig (PgBouncer) + SGBackupConfig
  - `ha: false` → 1 instance. `ha: true` → 2+ instances with Patroni
  - **Test:** create `ChoDatabase` with ha=false → assert SGCluster with 1 instance. Create with ha=true → assert 2+ instances. Wait for cluster ready.

- [ ] **4.3 — Database secret wiring**
  - Controller creates a Secret with connection string, username, password
  - Secret name follows convention: `{domain}--database--{name}-credentials`
  - **Test:** create ChoDatabase → assert Secret exists with expected keys (host, port, username, password, uri)

---

## Phase 5: Queue and cache compilation

- [ ] **5.1 — Install NATS operator in test cluster**
  - Add NATS operator install to test cluster script
  - **Test:** NATS CRDs available in cluster

- [ ] **5.2 — ChoQueue reconciler → NATS JetStream**
  - Watch `ChoQueue` CRD
  - Compile to kro RGD + instance that render NATS JetStream resources (StatefulSet or operator CR)
  - Expose connection credentials as Secret
  - **Test:** create ChoQueue → assert NATS resources exist → verify connectivity from a test pod

- [ ] **5.3 — ChoCache reconciler → Dragonfly**
  - Watch `ChoCache` CRD
  - Compile to kro RGD + instance that render Dragonfly Deployment + Service
  - Size mapping: small/medium/large → resource requests
  - **Test:** create ChoCache → assert Deployment + Service exist → verify Redis-compatible connectivity from test pod

---

## Phase 6: Network resource — consumes/supplies enforcement

- [ ] **6.1 — Compile consumes/supplies → NetworkPolicy**
  - When ChoApplication has `consumes`/`supplies` declarations, generate allow-rules in NetworkPolicy
  - Only the declared port + namespace selector. Everything else stays denied.
  - **Test:** domain A consumes domain B on port 8080 → deploy pods in both → pod in A can reach B:8080 → pod in A cannot reach B:9090 → pod in C cannot reach B:8080

- [ ] **6.2 — Supply/consume validation**
  - If domain A consumes domain B, but B does not supply → reconciliation error on ChoApplication status
  - Cycle detection: A→B→C→A → error
  - **Test:** create application with mismatched consumes/supplies → assert error in `.status.conditions`. Fix the mismatch → assert error clears.

- [ ] **6.3 — CiliumNetworkPolicy for L7 filtering**
  - For domains with `sensitivity = "restricted"`, generate CiliumNetworkPolicy with L7 HTTP path rules
  - **Test:** create restricted domain with L7 rules → assert CiliumNetworkPolicy exists → verify path-level filtering (allowed path works, disallowed path is blocked)

---

## Phase 7: Sandbox lifecycle

- [ ] **7.1 — Sandbox creation and isolation**
  - `ChoSandbox` CRD or annotation-based
  - Controller creates namespace `{app}-{domain}-sandbox-{name}`
  - Copies domain config into sandbox namespace
  - Each sandbox is fully isolated (own NetworkPolicy, own resources)
  - **Test:** create sandbox → assert namespace exists with all resources from domain spec → assert sandbox cannot reach production namespace

- [ ] **7.2 — Sandbox destruction and cleanup**
  - Delete sandbox CRD → controller deletes namespace and all resources
  - Owner references ensure cascade
  - **Test:** create sandbox → verify resources exist → delete sandbox → verify namespace gone

- [ ] **7.3 — CLI: `chorister apply` targets sandbox only**
  - CLI `apply` command reads the DSL file and creates/updates CRDs in an existing sandbox namespace
  - `chorister sandbox` remains the lifecycle command group (`create`, `destroy`, `list`), not a second apply surface
  - Refuses to target production namespace (hardcoded check + server-side rejection)
  - **Test:** `chorister apply --domain payments --sandbox alice` succeeds. Any attempt to apply to prod namespace is rejected.

---

## Phase 8: Diff and promotion

- [ ] **8.1 — Diff engine: sandbox vs production**
  - Compare compiled manifests between sandbox and production namespaces
  - Output human-readable diff (resource-level: added, changed, removed)
  - **Test:** apply different configs to sandbox and prod → `chorister diff` shows differences → apply same config → diff shows no changes

- [ ] **8.2 — ChoPromotionRequest reconciler**
  - Create `ChoPromotionRequest` CRD
  - Status lifecycle: Pending → Approved → Executing → Completed/Failed
  - Controller copies compiled Blueprint from sandbox namespace to production namespace on approval
  - **Test:** create ChoPromotionRequest → assert status=Pending → simulate approval (patch status) → assert production namespace updated → status=Completed

- [ ] **8.3 — Approval gate enforcement**
  - Read promotion policy from ChoApplication (requiredApprovers, allowedRoles)
  - Controller validates approvals before proceeding
  - Block if insufficient approvals
  - **Test:** create promotion with policy requiring 2 approvers → add 1 approval → assert still Pending → add 2nd → assert Executing then Completed

---

## Phase 9: Identity & access control

- [ ] **9.1 — ChoDomainMembership reconciler → RoleBinding**
  - Watch `ChoDomainMembership` CRD
  - Map role to namespace-scoped access in sandboxes: org-admin→admin, domain-admin→admin, developer→edit, viewer→view
  - Create RoleBinding in domain namespace
  - **Test:** create membership for alice as developer in payments → assert RoleBinding exists → verify alice can create pods in payments namespace → verify alice cannot create pods in other namespaces

- [ ] **9.2 — Membership expiry enforcement**
  - Controller checks `expiresAt` on reconciliation
  - Expired memberships: delete RoleBinding, update membership status
  - **Test:** create membership with expiresAt in the past → assert RoleBinding is removed → status shows expired

- [ ] **9.3 — Production RBAC lockdown**
  - Production namespaces: all human roles get view-only (no edit)
  - Only controller ServiceAccount can modify production resources
  - **Test:** create developer and org-admin memberships → assert both can administer sandboxes as expected → assert both are view-only in production

---

## Phase 10: OPA/Gatekeeper policy enforcement

- [ ] **10.1 — Install Gatekeeper and constraint templates**
  - Controller installs Gatekeeper during ChoCluster reconciliation
  - Constraint templates: no-privileged, no-hostPID, no-hostNetwork, drop-all-caps, non-root, image-allowlist
  - **Test:** Gatekeeper pods running. Create privileged pod → rejected. Create non-root pod → accepted.

- [ ] **10.2 — Compliance-profile-driven constraints**
  - `essential`: basic pod security (no privilege escalation, non-root)
  - `regulated`: add seccomp RuntimeDefault, AppArmor
  - Controller installs the right set of constraints based on ChoApplication compliance profile
  - **Test:** create application with `compliance: essential` → assert Level 1 constraints exist. Update to `compliance: regulated` → assert seccomp constraint added.

- [ ] **10.3 — Compile-time guardrails**
  - Controller rejects manifests at compile time for: internet ingress without auth, wildcard egress, egress to unapproved destinations
  - **Test:** submit ChoNetwork with ingress from internet and no auth block → assert compile error in status. Add auth block → assert success.

---

## Phase 11: Observability stack

- [ ] **11.1 — Grafana LGTM installation via ChoCluster**
  - Controller reconciles ChoCluster to install: Grafana Alloy, Mimir, Loki, Tempo
  - All configured to use local PVCs (for Kind; object storage in real clusters)
  - **Test:** create ChoCluster → assert Alloy, Mimir, Loki, Tempo pods running → Grafana accessible

- [ ] **11.2 — Audit event logging to Loki**
  - Controller writes structured JSON audit events to Loki on every reconciliation
  - Events: who, what, when, domain, application, action, result
  - Synchronous: if Loki write fails, reconciliation fails
  - **Test:** create/update a ChoCompute → query Loki for audit event → assert event contains expected fields

- [ ] **11.3 — Controller-generated Grafana dashboards**
  - Per-domain dashboard ConfigMap: pod status, resource usage, network flows
  - Grafana sidecar auto-loads dashboards
  - **Test:** create application with domain → assert Grafana dashboard ConfigMap exists in monitoring namespace

---

## Phase 12: ChoCluster — full stack bootstrap

- [ ] **12.1 — ChoCluster reconciler: operator lifecycle**
  - ChoCluster CRD defines which operators to install and their versions
  - Controller installs/upgrades: kro, StackGres, NATS operator, Dragonfly operator, cert-manager, Gatekeeper
  - If operator is deleted, controller reinstalls on next reconciliation
  - **Test:** create ChoCluster → assert all operators running. Delete StackGres operator → wait for reconciliation → assert reinstalled.

- [ ] **12.2 — `chorister setup` CLI command**
  - Installs controller Deployment + CRDs into `cho-system` namespace
  - Creates default ChoCluster CRD to trigger stack bootstrap
  - Idempotent: running twice is safe
  - **Test:** run `chorister setup` on clean cluster → assert controller running + CRDs registered. Run again → no errors, same state.

- [ ] **12.3 — Encrypted StorageClass validation**
  - Controller validates that an encrypted StorageClass exists during setup
  - Warn if not found (Kind won't have one, but real clusters must)
  - **Test:** controller starts on Kind → warning in logs about missing encrypted StorageClass (non-blocking for dev)

---

## Phase 13: Ingress & egress networking

- [ ] **13.1 — Egress allowlist enforcement**
  - Read `policy.network.egress.allowlist` from ChoApplication
  - Generate CiliumNetworkPolicy with FQDN-based egress rules
  - Block all other egress (except DNS, intra-cluster)
  - **Test:** create application with egress allowlist for `httpbin.org` → pod can reach httpbin.org → pod cannot reach other external hosts

- [ ] **13.2 — Ingress with JWT auth requirement**
  - ChoNetwork with `from = "internet"` requires auth block
  - Compile to Gateway API HTTPRoute + CiliumNetworkPolicy with JWT verification
  - **Test:** create ingress with JWT config → assert HTTPRoute + CiliumNetworkPolicy exist. Create ingress without auth → assert compile error.

- [ ] **13.3 — Cross-application links via Gateway API**
  - `link` in ChoApplication compiles to: HTTPRoute (consumer) + ReferenceGrant (supplier) + CiliumNetworkPolicy (L7) + CiliumEnvoyConfig (rate limit)
  - **Test:** create two applications with a link between them → assert HTTPRoute, ReferenceGrant, rate limit config exist → traffic flows through gateway

---

## Phase 14: Security scanning & vulnerability management

- [ ] **14.1 — Image scanning before promotion**
  - Controller runs Trivy scan on all images in a ChoPromotionRequest
  - Block promotion if critical CVEs found (`standard`+)
  - Store results in ChoVulnerabilityReport CRD
  - **Test:** create promotion request with image containing known CVE → assert promotion blocked. Use clean image → promotion proceeds.

- [ ] **14.2 — Continuous vulnerability scanning CronJobs**
  - For `standard`+ applications, controller creates CronJob per domain (daily re-scan)
  - Results written to ChoVulnerabilityReport CRDs
  - **Test:** create `standard` application with deployed images → assert CronJob exists → trigger manual run → assert ChoVulnerabilityReport created

- [ ] **14.3 — kube-bench periodic validation**
  - Controller creates kube-bench CronJob for cluster hardening checks
  - Results stored in ChoCluster.status.cisBenchmark
  - **Test:** assert kube-bench CronJob exists → trigger run → assert results in ChoCluster status

---

## Phase 15: Advanced features

- [ ] **15.1 — Data sensitivity enforcement**
  - Domain `sensitivity` field: public/internal/confidential/restricted
  - `confidential` → enforce TLS for all cross-domain traffic
  - `restricted` → require L7 policy, membership expiry, full Tetragon
  - **Test:** create domain with sensitivity=restricted → assert CiliumNetworkPolicy has L7 rules, memberships require expiresAt

- [ ] **15.2 — Tetragon runtime detection (`regulated`)**
  - Install Tetragon in test cluster
  - Controller generates TracingPolicy CRDs for restricted domains or `regulated` applications
  - Monitor: syscall anomalies, file integrity, unexpected process execution
  - **Test:** install Tetragon → create `regulated` application → assert TracingPolicy exists → exec into pod and trigger a monitored syscall → assert Tetragon event generated

- [ ] **15.3 — Service health baseline and incident response**
  - Controller monitors pod health, deployment progress, database status
  - Degraded domain → flag in status, block further promotions
  - `chorister admin isolate` → tighten NetworkPolicy, freeze promotions
  - **Test:** create domain → crash pods intentionally → assert domain status=Degraded → assert promotion is blocked → recover pods → status clears

- [ ] **15.4 — `chorister export` for GitOps**
  - Export compiled Blueprint as static YAML files
  - Compatible with ArgoCD/Flux directory structure
  - **Test:** create domain with compute + database → `chorister export` → assert output directory contains valid K8s manifests → `kubectl apply --dry-run=server` succeeds

---

## Phase 16: cert-manager & TLS

- [ ] **16.1 — cert-manager installation and wildcard certs**
  - Controller installs cert-manager via ChoCluster reconciliation
  - Creates ClusterIssuer (Let's Encrypt or self-signed for dev)
  - Wildcard Certificate for application domains
  - **Test:** cert-manager pods running → create Certificate → assert TLS secret generated

- [ ] **16.2 — Automatic TLS for cross-domain traffic**
  - For `confidential` and `restricted` domains, enforce mTLS via Cilium WireGuard or cert-manager
  - **Test:** create two confidential domains with consumes/supplies → verify traffic is encrypted (inspect Cilium encryption status)

---

## Phase 17: Secret management

- [ ] **17.1 — Secret slot declaration and auto-generation**
  - Blueprint declares typed secret slots
  - Sandbox: auto-generate secrets (random password for database, etc.)
  - Store as K8s Secrets with standard naming
  - **Test:** create ChoDatabase in sandbox → assert database credential Secret auto-generated → values are non-empty random strings

- [ ] **17.2 — External secret backend integration**
  - Support external backends: GCP Secret Manager, AWS Secrets Manager (via ExternalSecrets operator or direct)
  - Production environments reference external secrets
  - **Test:** (mock) configure external secret reference → assert ExternalSecret CR created → mock backend → assert K8s Secret synced

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
