# chorister Real-World Scenario Test Suite

## What This Is

This is the **scenario test suite** — bash-script-driven, end-to-end workflows that emulate how real personas use the platform.
It is distinct from the existing Go-based tests:

| Test layer | Location | What it tests | Runs via |
|---|---|---|---|
| **Unit / envtest** | `internal/.../*_test.go` | Individual controllers against a fake K8s API | `make test` |
| **API integration** | `test/e2e/` | K8s API object lifecycle on a real cluster | `make test-e2e` |
| **Scenario (this suite)** | `test/scenarios/` | Full CLI workflows with real stub apps | `make test-scenarios` |

**Why bash?** Each scenario should be readable as a story. It emulates exactly what a human or CI job would type.
The CLI binary (`bin/chorister`) is the subject under test. If a command is wrong, the scenario fails.

---

## Naming Proposals (Before Implementation)

The existing make targets need new names to distinguish the two Go-based test layers:

| Old name | New name | Why |
|---|---|---|
| `make test-e2e` / `make e2e` | `make test-api` / `make api-test` | Tests K8s API objects on a live cluster, not user-facing workflows |
| `make test-e2e-lite` | `make test-api-lite` | Same, but plain Kind |

> **AI agent note:** Renaming the Makefile targets and the `test/e2e/` directory is tracked as task **0-rename** below.
> Do NOT rename until confirmed with the user. Plan document describes the new structure; existing tests stay put for now.

---

## Cluster Requirements Per Scenario

| Requirement | Scenarios that need it |
|---|---|
| Kind cluster with Cilium | All |
| Gateway API CRDs | 04, 05, 09 |
| NATS operator | 02, 03, 07, 08 |
| cert-manager | 05, 09 |
| Dragonfly operator (or plain Deployment) | 02, 03, 07 |
| StackGres (or stub DB) | 02, 03, 06, 07, 08 |
| Tetragon | 08 (security events) |

Scenarios can share a single cluster if run sequentially, or use **separate named Kind clusters** for full parallelism.
The setup scripts in each scenario folder accept a `--cluster-name` flag for this purpose.

---

## Stub Applications

All scenarios share stub applications from `test/scenarios/apps/`.

### `echo-api` (Go HTTP server)
A minimal HTTP server that:
- Reads `DATABASE_URL`, `REDIS_URL`, `NATS_URL` from environment
- On startup: tries to connect to each, logs success/failure
- `GET /healthz` → 200 if all configured backends are reachable
- `GET /status` → JSON with connection state per backend
- `GET /env` → JSON dump of non-secret env vars (for debugging)
- `POST /write-db` → inserts a row; proves DB write works
- `POST /publish` → publishes a NATS message; proves queue works
- `POST /cache-set` → sets a Redis key; proves cache works
- `GET /read-db` → reads back the last inserted row
- `GET /subscribe` → pulls one pending NATS message

### `security-trigger` (Go HTTP server)
A minimal server that intentionally performs detectable actions when called:
- `POST /exec-shell` → forks `/bin/sh -c echo hi` (triggers Tetragon process exec)
- `POST /write-sensitive` → writes to `/etc/trigger-test` (triggers Tetragon file write)
- `POST /tcp-scan` → opens connections to 10 random IPs (triggers egress anomaly)
- `GET /healthz` → 200 always

Both apps are multi-arch Docker images (`linux/amd64`, `linux/arm64`) built and loaded into Kind by the setup scripts.
They should be kept < 10 MB and statically linked.

---

## Scenario Index

| # | Scenario | Parallelizable | Cilium required | Personas exercised |
|---|---|---|---|---|
| [01](#01-platform-bootstrap) | Platform Bootstrap | ✅ | No | Platform admin |
| [02](#02-developer-sandbox-workflow) | Developer Sandbox Workflow | ✅ | No | Developer |
| [03](#03-sandbox-to-production-promotion) | Sandbox → Production Promotion | ✅ | No | Developer + Domain admin |
| [04](#04-network-isolation-and-cross-domain-traffic) | Network Isolation | ✅ | **Yes** | Platform admin + Developer |
| [05](#05-internet-ingress-with-jwt-auth) | Internet Ingress with JWT Auth | ✅ | **Yes** | Developer + Platform admin |
| [06](#06-stateful-resource-archive-safety) | Stateful Resource Archive Safety | ✅ | No | Developer + Platform admin |
| [07](#07-full-stack-stub-app-health-check) | Full Stack Stub App Health Check | ✅ | No | Developer |
| [08](#08-security-events-and-vulnerability-reports) | Security Events & Vuln Reports | ✅ | **Yes** | Platform admin |
| [09](#09-cross-application-link) | Cross-Application Link | ✅ | **Yes** | Developer + Platform admin |
| [10](#10-domain-membership-rbac-and-expiry) | Domain Membership & RBAC | ✅ | No | Platform admin + Developer |
| [11](#11-sandbox-finops-budget-enforcement) | Sandbox FinOps Budget Enforcement | ✅ | No | Platform admin + Developer |
| [12](#12-incident-response-isolate-and-recover) | Incident Response: Isolate & Recover | ✅ | **Yes** | Platform admin |

---

## Task Checklist for AI Agents

Each task is self-contained. Work one task at a time. Mark `[x]` when done.

### Pre-work

- [ ] **0-rename** — (Optional, confirm with user first) Rename `make test-e2e` → `make test-api`, `make e2e` → `make api-test` in `Makefile`. Update `test/e2e/` directory references if renamed. Do NOT rename until user confirms.
- [x] **0-plan** — This document.

### Shared Infrastructure

- [x] **infra-setup-script** — `test/scenarios/setup-scenario-cluster.sh`
  - Wraps `hack/setup-test-cluster.sh`
  - Accepts `--cluster-name`, `--with-stackgres`, `--with-nats`, `--with-tetragon` flags
  - Builds and loads `echo-api` and `security-trigger` images into the cluster
  - Installs the chorister controller + CRDs via `make deploy`
  - Idempotent (safe to re-run)

- [x] **infra-teardown-script** — `test/scenarios/teardown-scenario-cluster.sh`
  - Deletes the named Kind cluster
  - Safe to call even if cluster doesn't exist

- [x] **infra-makefile-targets** — Add to `Makefile`:
  ```makefile
  .PHONY: test-scenarios
  test-scenarios: build ## Run all scenario tests (sequential, single cluster)
      bash test/scenarios/run-all.sh

  .PHONY: test-scenario
  test-scenario: build ## Run a single scenario: make test-scenario SCENARIO=02
      bash test/scenarios/$(SCENARIO)-*/run.sh
  ```

- [x] **stub-app-echo-api** — `test/scenarios/apps/echo-api/`
  - `main.go`: HTTP server as described in Stub Applications section
  - `Dockerfile`: multi-arch, statically linked, < 10 MB
  - `k8s/deployment.yaml`: sample Deployment manifest for testing (not used directly — scenarios use ChoCompute CRDs)

- [x] **stub-app-security-trigger** — `test/scenarios/apps/security-trigger/`
  - `main.go`: security event trigger server
  - `Dockerfile`: same constraints
  - Accepts all triggers via POST endpoints

- [x] **infra-run-all** — `test/scenarios/run-all.sh`
  - Runs each scenario's `run.sh` sequentially
  - Reports pass/fail per scenario
  - Returns non-zero exit if any scenario fails
  - Accepts `--parallel` flag to run scenarios in separate clusters concurrently

### Scenario 01: Platform Bootstrap

**Location:** `test/scenarios/01-platform-bootstrap/`

- [x] **01-run** — `run.sh` orchestrates the scenario:
  1. Spin up cluster (Cilium, no operators yet)
  2. Run through all assertions
  3. Tear down cluster

- [x] **01-assert-setup** — Verify `chorister setup --dry-run` completes without error.
  - **Blocked on:** `chorister setup` actual implementation (currently returns an error unless cluster is running).
  - **Workaround:** Apply CRDs + controller manually via `kubectl apply -k config/default`, then assert controller pod is Running.

- [x] **01-assert-cluster-bootstrap** — Create a `ChoCluster` CR and verify:
  - Controller pod is Running in `cho-system`
  - CRDs are registered (all 12)
  - `chorister admin app list` returns empty list (not an error)

- [x] **01-assert-app-create** — Create a `ChoApplication` via kubectl (CLI `admin app create` is a stub):
  - Assert domain namespaces are created: `myapp-payments`, `myapp-auth`
  - Assert default-deny NetworkPolicy exists in each namespace
  - Assert `chorister status --app myapp` shows both domains

- [x] **01-assert-cli-version** — `chorister version` prints version string, non-empty.

**Assertions file:** `test/scenarios/01-platform-bootstrap/assert.sh`

---

### Scenario 02: Developer Sandbox Workflow

**Location:** `test/scenarios/02-developer-sandbox/`

- [x] **02-run** — `run.sh` setup + all assertions

- [x] **02-setup** — Pre-create a `ChoApplication` with one domain (`payments`).

- [x] **02-assert-sandbox-create** — `chorister sandbox create --domain payments --name alice`
  - Assert sandbox namespace `myapp-payments-sandbox-alice` exists
  - Assert default-deny NetworkPolicy in sandbox namespace
  - `chorister sandbox list --domain payments` shows `alice`

- [x] **02-assert-apply-compute** — Apply a `ChoCompute` CR (echo-api image) to the sandbox.
  - Assert Deployment is created in sandbox namespace
  - Assert Service is created if port is declared
  - `chorister status payments --app myapp` shows compute resource

- [x] **02-assert-apply-database** — Apply a `ChoDatabase` CR (postgres) to the sandbox.
  - Assert credentials Secret is created
  - Assert `lifecycle: Active` in status

- [x] **02-assert-apply-queue** — Apply a `ChoQueue` CR (nats) to the sandbox.
  - Assert credentials Secret is created

- [x] **02-assert-apply-cache** — Apply a `ChoCache` CR (small) to the sandbox.
  - Assert credentials Secret is created

- [x] **02-assert-sandbox-status** — `chorister sandbox list` shows `alice` with resource counts.

- [x] **02-assert-sandbox-destroy** — `chorister sandbox destroy --domain payments --name alice`
  - Assert sandbox namespace is deleted
  - Assert `chorister sandbox list` no longer shows `alice`

---

### Scenario 03: Sandbox → Production Promotion

**Location:** `test/scenarios/03-sandbox-to-production/`

- [x] **03-run** — `run.sh`

- [x] **03-setup** — Pre-create `ChoApplication` with `payments` domain (1 required approver).

- [x] **03-assert-sandbox-and-apply** — Create sandbox, apply `echo-api` compute + database.

- [x] **03-assert-diff-before-promote** — `chorister diff --domain payments --sandbox alice`
  - Assert output contains resources as "Added" (sandbox has them, production doesn't)

- [x] **03-assert-promote-creates-request** — `chorister promote --domain payments --sandbox alice`
  - Assert `ChoPromotionRequest` is created in `cho-system`
  - `chorister requests --domain payments` shows the request in `Pending`

- [x] **03-assert-unapproved-does-not-modify-prod** — Before approval:
  - Assert production namespace does NOT contain the compute Deployment
  - Assert production namespace does NOT contain the database Secret

- [x] **03-assert-approve-promotes** — `chorister approve <request-id>`
  - Assert `ChoPromotionRequest` phase transitions to `Approved` then `Executing` then `Completed`
  - Assert compute Deployment appears in production namespace
  - Assert database credentials Secret appears in production namespace

- [x] **03-assert-diff-after-promote** — `chorister diff --domain payments --sandbox alice`
  - Assert output shows no differences (or "up to date")

- [x] **03-assert-rollback** — `chorister promote --domain payments --rollback`
  - Assert rollback `ChoPromotionRequest` is created

---

### Scenario 04: Network Isolation and Cross-Domain Traffic

**Location:** `test/scenarios/04-network-isolation/`

**Requires:** Cilium in cluster.

- [x] **04-run** — `run.sh`

- [x] **04-setup** — Create `ChoApplication` with:
  - `payments` domain: `consumes auth:8080`, deploys `echo-api` pod
  - `auth` domain: `supplies :8080`, deploys `echo-api` pod
  - `unrelated` domain: no declares

- [x] **04-assert-cross-domain-allowed** — From `payments` pod, curl `auth-echo-api.myapp-auth.svc.cluster.local:8080/healthz`
  - Assert HTTP 200

- [x] **04-assert-wrong-port-blocked** — From `payments` pod, curl `auth` service on port 9090
  - Assert connection refused / timeout (NetworkPolicy blocks it)

- [x] **04-assert-unrelated-blocked** — From `unrelated` domain pod, curl `auth` service on port 8080
  - Assert connection refused / timeout

- [x] **04-assert-reverse-blocked** — From `auth` pod, curl `payments` service (auth does not `consumes payments`)
  - Assert connection refused

- [x] **04-assert-egress-blocked** — From `payments` pod, curl an undeclared external IP
  - Assert connection blocked (requires Cilium FQDN egress enforcement)
  - **Note:** Only runs if cluster has Cilium egress enforcement enabled

---

### Scenario 05: Internet Ingress with JWT Auth

**Location:** `test/scenarios/05-ingress-jwt/`

**Requires:** Cilium, Gateway API, a test OIDC token (can be a self-signed JWT for testing).

- [x] **05-run** — `run.sh`

- [x] **05-setup** — Create `ChoApplication` with an internet-facing `ChoNetwork` resource:
  ```yaml
  ingress:
    from: internet
    port: 443
    auth:
      jwt:
        issuer: https://test.chorister.dev
        jwksUri: http://mock-jwks.cho-system.svc.cluster.local/jwks
    routes:
      /api/*: {}
      /healthz: { auth: none }
  ```
  Also deploy a mock JWKS server in `cho-system`.

- [x] **05-assert-no-auth-rejected** — Applying a `ChoNetwork` with internet ingress but NO auth block
  - Assert controller rejects with validation error (or CRD webhook rejects it)

- [x] **05-assert-healthz-anonymous** — `curl /healthz` without JWT
  - Assert HTTP 200 (anonymous route declared)

- [x] **05-assert-api-requires-jwt** — `curl /api/users` without JWT
  - Assert HTTP 401

- [x] **05-assert-api-with-valid-jwt** — `curl /api/users` with a valid JWT signed by the test issuer
  - Assert HTTP 200

- [x] **05-assert-api-with-invalid-jwt** — `curl /api/users` with a tampered JWT
  - Assert HTTP 401

---

### Scenario 06: Stateful Resource Archive Safety

**Location:** `test/scenarios/06-archive-safety/`

- [x] **06-run** — `run.sh`

- [x] **06-setup** — Promote a `ChoDatabase` (`ledger`) to production via a ChoPromotionRequest.
  - Also promote a `ChoCompute` (`api`) that references the database credentials.

- [x] **06-assert-database-in-production** — Verify database credentials Secret exists in production namespace.

- [x] **06-assert-remove-triggers-archive** — Remove `database "ledger"` from the domain and promote.
  - Assert `ChoDatabase` status transitions to `lifecycle: Archived` (not deleted)
  - Assert actual database resources (StatefulSet / Secret) still exist

- [x] **06-assert-archived-blocks-dependent-promotion** — Try to promote `ChoCompute` that still references the archived database credentials.
  - Assert the promotion is rejected with an error mentioning the archived resource

- [x] **06-assert-sandbox-delete-immediate** — In a sandbox, remove a database and apply.
  - Assert sandbox database is immediately deleted (no archive lifecycle in sandboxes)

- [x] **06-assert-explicit-delete-required** — `chorister admin resource delete --archived ledger --domain payments`
  - Assert only works after the retention period has passed (or with `--force` for testing)
  - Assert final backup snapshot reference is recorded in status

---

### Scenario 07: Full Stack Stub App Health Check

**Location:** `test/scenarios/07-full-stack/`

**This is the integration smoke test: does everything actually work together?**

- [x] **07-run** — `run.sh`

- [x] **07-setup** — Create `ChoApplication` with a `payments` domain. Apply:
  - `ChoCompute`: `echo-api` with env vars wired to secrets
  - `ChoDatabase`: postgres (non-HA for speed)
  - `ChoQueue`: nats
  - `ChoCache`: small

- [x] **07-assert-compute-running** — Deployment is Running (≥1 pod Ready).

- [x] **07-assert-db-connectivity** — `POST /write-db` from inside the pod → HTTP 200
  - Logs show: `Connected to database successfully`

- [x] **07-assert-queue-connectivity** — `POST /publish` + `GET /subscribe` → round-trip message
  - Logs show: `Published to NATS`, `Received from NATS`

- [x] **07-assert-cache-connectivity** — `POST /cache-set` + verify read-back via `GET /status`
  - Logs show: `Connected to cache successfully`

- [x] **07-assert-healthz** — `GET /healthz` returns HTTP 200 with all backends `"ok"`.

- [x] **07-assert-logs-cmd** — `chorister logs payments --sandbox dev` tails pod logs; output contains backend status.

- [x] **07-assert-status-cmd** — `chorister status payments --app myapp` shows `Ready` phase for all resources.

---

### Scenario 08: Security Events and Vulnerability Reports

**Location:** `test/scenarios/08-security/`

**Requires:** Cilium with Tetragon enabled in cluster.

- [x] **08-run** — `run.sh`

- [x] **08-setup** — Deploy `security-trigger` app in a `payments` sandbox.
  Enable `runtimeDetection: full` on the domain (or use `regulated` compliance profile).

- [x] **08-assert-vuln-scan-report** — Apply `ChoCompute` with `security-trigger` image.
  - Wait for periodic vulnerability scan CronJob to run.
  - Assert `ChoVulnerabilityReport` CR is created in domain namespace.
  - `chorister admin vulnerabilities --domain payments` shows the report.

- [x] **08-assert-vuln-blocks-promotion-standard** — Set application compliance to `standard`.
  - Attempt to promote with a known-vulnerable image.
  - Assert `ChoPromotionRequest` is rejected / stays in `Failed` with scan gate message.

- [x] **08-assert-vuln-allows-promotion-clean** — Replace with a clean image.
  - Assert promotion proceeds to `Completed`.

- [x] **08-assert-tetragon-process-exec** — `POST /exec-shell` on `security-trigger` pod.
  - Assert Tetragon event is recorded (check Tetragon logs or Loki query).

- [x] **08-assert-tetragon-file-write** — `POST /write-sensitive` on `security-trigger` pod.
  - Assert Tetragon file integrity event is recorded.

- [x] **08-assert-admin-scan** — `chorister admin scan --domain payments`
  - Assert command triggers a scan and reports findings.

---

### Scenario 09: Cross-Application Link

**Location:** `test/scenarios/09-cross-app-link/`

**Requires:** Cilium, Gateway API.

- [x] **09-run** — `run.sh`

- [x] **09-setup** — Create two `ChoApplication` resources:
  - `retail` app with `payments` domain (consumer)
  - `capital-markets` app with `pricing` domain (supplier)
  - Declare a bilateral `link` in `retail` → `capital-markets/pricing:8080`
  - Deploy `echo-api` in both domains.

- [x] **09-assert-direct-pod-to-pod-blocked** — From `retail-payments` pod, curl `pricing` pod IP directly.
  - Assert blocked (NetworkPolicy / Cilium).

- [x] **09-assert-httproute-and-referencegrant-exist** — `kubectl get httproute,referencegrant -A`
  - Assert HTTPRoute in `retail-payments` exists.
  - Assert ReferenceGrant in `capital-markets-pricing` exists.

- [x] **09-assert-traffic-via-gateway** — From `retail-payments` pod, curl the internal gateway path to `pricing`.
  - Assert HTTP 200.

- [x] **09-assert-undeclared-consumer-blocked** — From a third application's pod, attempt the gateway path to `pricing`.
  - Assert HTTP 403 / blocked (CiliumNetworkPolicy L7).

---

### Scenario 10: Domain Membership, RBAC, and Expiry

**Location:** `test/scenarios/10-domain-membership/`

- [x] **10-run** — `run.sh`

- [x] **10-setup** — Create `ChoApplication` with `payments` domain (sensitivity: `restricted`).

- [x] **10-assert-add-member-requires-expiry** — `chorister admin member add --domain payments --identity alice@co --role developer` (no `--expires-at`)
  - Assert error: "expires-at is required for restricted domain"

- [x] **10-assert-add-member-with-expiry** — `chorister admin member add ... --expires-at 2027-01-01T00:00:00Z`
  - Assert `ChoDomainMembership` CR created.
  - Assert RoleBinding exists in sandbox namespace for alice.

- [x] **10-assert-developer-cannot-write-prod** — Using a ServiceAccount that maps to alice's role:
  - Assert `kubectl auth can-i create deployments --namespace myapp-payments --as alice@co` → `no`

- [x] **10-assert-developer-can-read-prod** — Assert `kubectl auth can-i get pods --namespace myapp-payments --as alice@co` → `yes`

- [x] **10-assert-expired-membership-removed** — Create a membership with expiry in the past.
  - Trigger reconciliation.
  - Assert RoleBinding is deleted.
  - `chorister admin member list --include-expired` shows the expired entry.

- [x] **10-assert-member-audit** — `chorister admin member audit --app myapp`
  - Assert command flags the expired/stale membership.

- [x] **10-assert-member-remove** — `chorister admin member remove <membership-id>`
  - Assert `ChoDomainMembership` deleted.
  - Assert RoleBinding removed.

---

### Scenario 11: Sandbox FinOps Budget Enforcement

**Location:** `test/scenarios/11-finops-budget/`

- [x] **11-run** — `run.sh`

- [x] **11-setup** — Create `ChoApplication` with a very small sandbox budget:
  ```yaml
  policy:
    sandbox:
      defaultBudgetPerDomain: "$10/month"
      maxIdleDays: 1
  ```
  Set `ChoCluster` finops rates so that a single `medium` database exceeds $10/month.

- [x] **11-assert-sandbox-budget-enforced** — Create a sandbox with a `medium` database.
  - Assert sandbox creation is rejected with cost breakdown message.

- [x] **11-assert-small-sandbox-allowed** — Create a sandbox with `small` compute only (under budget).
  - Assert sandbox created successfully.
  - `chorister sandbox list` shows `estimatedMonthlyCost` in status.

- [x] **11-assert-budget-alert-threshold** — Add more resources to approach the budget limit.
  - Assert `ChoApplication.status.sandbox.budgetUsagePercent` > 80.
  - Assert status condition "BudgetAlert" is set.

- [x] **11-assert-idle-auto-destroy** — Create a sandbox, set `maxIdleDays: 0` (or patch lastApplyTime to past).
  - Trigger reconciliation.
  - Assert sandbox namespace is deleted automatically.
  - Assert `chorister sandbox list` no longer shows it.

---

### Scenario 12: Incident Response — Isolate and Recover

**Location:** `test/scenarios/12-incident-response/`

**Requires:** Cilium.

- [x] **12-run** — `run.sh`

- [x] **12-setup** — Create production `payments` domain with `echo-api` compute.
  - Verify it is healthy (GET /healthz returns 200).

- [x] **12-assert-crash-loop-flags-degraded** — Patch the Deployment to use a bad command that crashes.
  - Wait for crash loop.
  - Assert `ChoApplication.status` or domain condition shows `Degraded`.

- [x] **12-assert-isolate-freezes-promotions** — `chorister admin isolate --domain payments`
  - Assert `ChoApplication.status` shows isolation flag.
  - Attempt `chorister promote --domain payments --sandbox dev`.
  - Assert promotion is rejected with isolation message.

- [x] **12-assert-isolate-tightens-network** — From another domain's pod, attempt to reach `payments` service.
  - Assert blocked (isolation tightens NetworkPolicy beyond declared consumes).

- [x] **12-assert-unisolate-restores** — `chorister admin unisolate --domain payments`
  - Restore the Deployment.
  - Assert domain condition returns to `Ready`.
  - Assert cross-domain traffic resumes.
  - Assert new promotions are accepted.

---

## File/Folder Structure

```
test/scenarios/
├── SCENARIOS.md                    ← this file
├── run-all.sh                      ← runs all scenarios (sequential or parallel)
├── setup-scenario-cluster.sh       ← cluster setup with optional operators
├── teardown-scenario-cluster.sh    ← cluster teardown
├── lib/
│   ├── assert.sh                   ← shared assertion helpers (assert_eq, assert_contains, wait_for, etc.)
│   ├── chorister.sh                ← thin wrappers for CLI calls with logging
│   └── kubectl.sh                  ← kubectl helpers (wait_for_deployment, pod_exec, etc.)
├── apps/
│   ├── echo-api/
│   │   ├── main.go
│   │   └── Dockerfile
│   └── security-trigger/
│       ├── main.go
│       └── Dockerfile
├── 01-platform-bootstrap/
│   ├── run.sh
│   └── assert.sh
├── 02-developer-sandbox/
│   ├── run.sh
│   └── assert.sh
├── 03-sandbox-to-production/
│   ├── run.sh
│   └── assert.sh
├── 04-network-isolation/
│   ├── run.sh
│   └── assert.sh
├── 05-ingress-jwt/
│   ├── run.sh
│   ├── mock-jwks/           ← simple Go HTTP server serving a static JWKS
│   └── assert.sh
├── 06-archive-safety/
│   ├── run.sh
│   └── assert.sh
├── 07-full-stack/
│   ├── run.sh
│   └── assert.sh
├── 08-security/
│   ├── run.sh
│   └── assert.sh
├── 09-cross-app-link/
│   ├── run.sh
│   └── assert.sh
├── 10-domain-membership/
│   ├── run.sh
│   └── assert.sh
├── 11-finops-budget/
│   ├── run.sh
│   └── assert.sh
└── 12-incident-response/
    ├── run.sh
    └── assert.sh
```

---

## Agent Implementation Guidelines

When implementing any task above, follow these rules:

### Bash Script Standards
- Every `run.sh` begins with `set -euo pipefail`.
- Every `run.sh` sources `../lib/assert.sh`, `../lib/chorister.sh`, `../lib/kubectl.sh`.
- Every `run.sh` accepts `--cluster-name` (default: `chorister-scenario-NN`) and `--skip-setup` (for fast reruns).
- Every `run.sh` exits 0 on full pass, non-zero on any assertion failure.
- Print `[PASS] <description>` or `[FAIL] <description>` for each assertion.

### CLI Usage — Current Actual Commands
The CLI binary is at `bin/chorister`. Always use the exact flags shown here (verified from `cmd/chorister/main.go`):

```bash
chorister version
chorister setup [--dry-run]
chorister login

chorister apply --domain <d> --sandbox <s> --file <f>

chorister sandbox create --domain <d> --name <n>
chorister sandbox destroy --domain <d> --name <n>
chorister sandbox list [--domain <d>] [--app <a>] [--output json|yaml|table]
chorister sandbox status                             # (subcommand exists, check implementation)

chorister diff --domain <d> --sandbox <s> [--app <a>] [--output ...]
chorister status [<domain>] [--app <a>] [--output ...]
chorister promote --domain <d> --sandbox <s> [--app <a>]
chorister promote --domain <d> --rollback [--app <a>]
chorister approve <promotion-id>
chorister reject <promotion-id>
chorister requests [--domain <d>] [--app <a>] [--status pending|approved|rejected|all] [--output ...]

chorister logs <domain> [--sandbox <s>] [--app <a>]
chorister events [--domain <d>] [--app <a>]
chorister get <resource-type> [name] [--domain <d>] [--app <a>] [--output ...]
chorister wait --domain <d> [--sandbox <s>] [--app <a>] [--timeout <duration>]
chorister export [--domain <d>] [--app <a>] [--output ...]

chorister admin app list [--output ...]
chorister admin app get <name> [--output ...]
chorister admin app create <name>                    # stub — use kubectl apply for now
chorister admin app delete <name> [--dry-run] [--confirm]
chorister admin app set-policy <name>               # stub

chorister admin domain list [--app <a>] [--output ...]
chorister admin domain get <name> --app <a> [--output ...]
chorister admin domain create <name>                # stub
chorister admin domain delete <name> --app <a> [--dry-run] [--confirm]

chorister admin member add --app <a> --domain <d> --identity <e> --role <r> [--expires-at <rfc3339>] [--source manual|oidc-group]
chorister admin member list [--app <a>] [--domain <d>] [--role <r>] [--include-expired] [--output ...]
chorister admin member remove                       # stub
chorister admin member audit --app <a> [--output ...]

chorister admin cluster                             # cluster management subcommands
chorister admin compliance                          # compliance report subcommands
chorister admin audit                               # audit log subcommands
chorister admin finops                              # finops subcommands
chorister admin quotas                              # quota subcommands
chorister admin vulnerabilities --domain <d>
chorister admin scan --domain <d>
chorister admin isolate --domain <d> [--app <a>]
chorister admin unisolate --domain <d> [--app <a>]
chorister admin resource delete --archived <name> [--domain <d>] [--force]
chorister admin upgrade
chorister admin export-config
```

> **Differences from ARCHITECTURE_DECISIONS.md examples:**
> The current CLI has more commands than shown in the architecture doc. Use the list above (derived from actual code) as ground truth for scripts.
> Notable additions: `events`, `get`, `wait`, `docs`, `admin cluster`, `admin audit`, `admin finops`, `admin quotas`, `admin vulnerabilities`, `admin scan`, `admin export-config`, `sandbox status`.

### When a CLI Command Is Still a Stub
If a command prints `(not yet implemented)` and returns nil, the scenario step should:
1. Note this explicitly in a comment.
2. Fall back to `kubectl apply` with the equivalent CRD YAML.
3. Mark the step with `# STUB: replace with CLI call when implemented`.

### CRD YAML for Common Resources
Store reusable CRD YAML fixtures in each scenario folder under `fixtures/`.
Name them `cho-<kind>-<name>.yaml`. Example: `fixtures/cho-compute-echo-api.yaml`.

### Wait Patterns
Use exponential backoff, not `sleep`. The shared `lib/kubectl.sh` should provide:
```bash
wait_for_condition <namespace> <resource> <name> <jsonpath> <expected-value> [timeout=120s]
wait_for_deployment_ready <namespace> <name> [timeout=120s]
wait_for_pod_running <namespace> <label-selector> [timeout=120s]
```

### Parallel Execution
When `run-all.sh --parallel` is used:
- Each scenario gets a unique cluster name: `chorister-scenario-NN`
- Clusters are created/destroyed within each scenario's `run.sh`
- `run-all.sh` waits for all background jobs, collects exit codes

### Test Isolation
- Each scenario creates its own `ChoApplication` with a unique app name (e.g., `scen01-myapp`).
- Do NOT assume shared state between scenarios.
- Do NOT hardcode cluster IPs or node names.

---

## CLI Gap Analysis (Architecture Doc vs Current Implementation)

> Last verified: 2026-04-15 against `cmd/chorister/main.go`

### Stubs (print message, no cluster mutation)

| Command | Stub message | Action needed in scenarios |
|---|---|---|
| `chorister setup` (without `--dry-run`) | Returns error: "setup requires a running Kubernetes cluster" | Bypass: apply CRDs + controller via `kubectl apply -k config/default` |
| `chorister setup --dry-run` | Prints what would happen, returns nil ✅ (dry-run only) | Safe to use in 01-assert-setup |
| `chorister sandbox create` | ✅ Ready | Creates ChoSandbox CRD; controller provisions namespace |
| `chorister sandbox destroy` | ✅ Ready | Deletes ChoSandbox CRD; controller cleans up namespace via finalizer |
| `chorister diff` | Stub — prints "not yet implemented" | Note in scenario 03; skip diff assertions |
| `chorister logs <component>` | ✅ Ready | Streams pod logs via Kubernetes clientset; lists components if no arg |
| `chorister admin app set-policy` | ✅ Ready | Updates compliance, approvers, security scan, archive retention, idle days |
| `chorister export` | ✅ Ready | Exports all Cho CRDs from live domain namespace (ChoCompute, ChoDatabase, ChoQueue, ChoCache, ChoStorage, ChoNetwork) |
| `chorister login` | Prints "login: not yet implemented", returns nil | Not exercised in scenarios |
| `chorister apply --file` | Prints "not yet implemented", returns nil | Bypass with `kubectl apply` + STUB comment in scenarios 02, 03 |
| `chorister reject <id>` | Prints "not yet implemented", returns nil | Note in scenario 03 |
| `chorister admin app create` | Prints "not yet implemented", returns nil | Bypass with `kubectl apply` ChoApplication CRD in scenarios |
| `chorister admin domain create` | Prints "not yet implemented", returns nil | Bypass with `kubectl patch` ChoApplication domains list |
| `chorister admin member remove` | Prints "not yet implemented", returns nil | Note in scenario 10 |

### Implemented (queries or mutates live cluster)

| Command | Status | Notes |
|---|---|---|
| `chorister version` | ✅ Ready | Prints build info |
| `chorister promote` | ✅ Ready | Creates ChoPromotionRequest CRD |
| `chorister promote --rollback` | ✅ Ready | Creates rollback ChoPromotionRequest |
| `chorister requests` | ✅ Ready | Lists ChoPromotionRequests with filters |
| `chorister status [domain]` | ✅ Ready | Domain health summary or detail view |
| `chorister sandbox list` | ✅ Ready | Lists ChoSandbox resources |
| `chorister sandbox status` | ✅ Ready | Detail view for a single sandbox |
| `chorister events` | ✅ Ready | Lists K8s Events for chorister resources |
| `chorister get <type> <name>` | ✅ Ready | Inspects any chorister resource |
| `chorister wait` | ✅ Ready | Polls condition with timeout |
| `chorister export` | ⚠ Partial | Only exports restricted-domain L7 policy; other resource types emit empty file |
| `chorister admin app list` | ✅ Ready | Lists ChoApplication resources |
| `chorister admin app get` | ✅ Ready | Detail view with domain list |
| `chorister admin app delete` | ✅ Ready | Deletes with --dry-run and --confirm gate |
| `chorister admin domain list` | ✅ Ready | Lists domains across apps |
| `chorister admin domain get` | ✅ Ready | Detail view with resource list |
| `chorister admin domain delete` | ✅ Ready | Removes domain from ChoApplication spec |
| `chorister admin domain set-sensitivity` | ✅ Ready | Updates domain sensitivity with compliance check |
| `chorister admin member add` | ✅ Ready | Creates ChoDomainMembership with expiry enforcement |
| `chorister admin member list` | ✅ Ready | Lists with role/domain/expired filters |
| `chorister admin member audit` | ✅ Ready | Flags expired/stale memberships |
| `chorister admin compliance report` | ✅ Ready | Full compliance report per framework |
| `chorister admin compliance status` | ✅ Ready | Summary pass/fail view |
| `chorister admin cluster status` | ✅ Ready | ChoCluster health and operator status |
| `chorister admin cluster operators` | ✅ Ready | Lists managed operators with versions |
| `chorister admin audit` | ✅ Ready | Queries Loki audit log (needs CHORISTER_LOKI_URL) |
| `chorister admin finops report` | ✅ Ready | Cost breakdown by domain/sandbox |
| `chorister admin finops budget` | ✅ Ready | Budget utilization per domain |
| `chorister admin quotas` | ✅ Ready | ResourceQuota utilization per domain |
| `chorister admin vulnerabilities list` | ✅ Ready | Lists ChoVulnerabilityReports |
| `chorister admin vulnerabilities get` | ✅ Ready | Detail view with findings table |
| `chorister admin scan` | ✅ Ready | Triggers scan via CronJob annotation |
| `chorister admin isolate` | ✅ Ready | Sets isolation annotation on ChoApplication |
| `chorister admin unisolate` | ✅ Ready | Removes isolation annotation |
| `chorister admin resource list` | ✅ Ready | Lists resources in a domain namespace |
| `chorister admin resource delete` | ✅ Ready | Deletes an archived resource by name+type+namespace |
| `chorister admin upgrade` | ✅ Ready | Blue-green canary revision management on ChoCluster |
| `chorister admin export-config` | ✅ Ready | Exports ChoApplication + ChoDomainMembership CRDs as YAML |

---

## Priority Order for Implementation

Implement in this order to unlock the most scenarios fastest:

1. **infra-setup-script** + **infra-makefile-targets** — Foundation for all scenarios
2. **stub-app-echo-api** — Required by 02, 03, 04, 07, 09, 12
3. **01-platform-bootstrap** — Validates basic controller + CLI health
4. **02-developer-sandbox** — Core developer workflow
5. **03-sandbox-to-production** — Validates promotion pipeline
6. **07-full-stack** — Proves the stack actually works end-to-end
7. **10-domain-membership** — RBAC validation (many CLI commands are live)
8. **04-network-isolation** — Cilium-dependent
9. **06-archive-safety** — Stateful resource safety
10. **11-finops-budget** — FinOps quotas
11. **stub-app-security-trigger** + **08-security** — Tetragon-dependent
12. **05-ingress-jwt** + **09-cross-app-link** — Gateway API dependent
13. **12-incident-response** — Cilium + full stack required
