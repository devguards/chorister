# Architecture Decisions

Foundational design choices for chorister — an opinionated infrastructure platform that runs as a K8s operator, with sandbox-first workflow, deterministic promotion, and compliance built in.

Unless marked as deferred or MVP-only, this document describes the target end-state architecture. It is the canonical source for command semantics and production safety guarantees.

**Core invariant: chorister is a K8s operator.** Users submit DSL as CRDs (or via CLI). The chorister controller reconciles CRDs directly into K8s-native resources, operator CRDs (StackGres, Cilium, cert-manager, etc.), and — only for cloud resources like object storage — kro ResourceGraphDefinitions. K8s is the control plane. etcd is the state store. The CLI is a thin client — it creates/updates CRDs and watches status.

---

## Table of Contents

1. [Overview & Core Philosophy](#1-overview--core-philosophy)
2. [Hierarchy: Application, Domain, Components](#2-hierarchy-application-domain-components)
3. [Platform Target: K8s-Only](#3-platform-target-k8s-only)
4. [Component Stack](#4-component-stack)
5. [State & Reconciliation Model](#5-state--reconciliation-model)
6. [Blueprint Artifact Format](#6-blueprint-artifact-format)
7. [DSL / Input Format](#7-dsl--input-format)
8. [Provider / Plugin Architecture](#8-provider--plugin-architecture)
9. [Resource Types & Compilation Targets](#9-resource-types--compilation-targets)
10. [Network & Security Model](#10-network--security-model)
11. [Identity & Access Control](#11-identity--access-control)
12. [Audit, Compliance & Approval Gates](#12-audit-compliance--approval-gates)
13. [Observability](#13-observability)
14. [Real-time Feedback](#14-real-time-feedback)
15. [Naming & Resource Identity](#15-naming--resource-identity)
16. [Secret Management](#16-secret-management)
17. [Platform Gaps & Deferred Decisions](#17-platform-gaps--deferred-decisions)
18. [Multi-cluster & Scaling Path](#18-multi-cluster--scaling-path)
19. [Implementation Timeline](#19-implementation-timeline)
20. [Stateful Resource Deletion Safety](#20-stateful-resource-deletion-safety)
21. [Controller Upgrade & CRD Versioning](#21-controller-upgrade--crd-versioning)
22. [Sandbox Lifecycle & FinOps Quotas](#22-sandbox-lifecycle--finops-quotas)
23. [Resource Sizing Model](#23-resource-sizing-model)

---

## 1. Overview & Core Philosophy

### Architecture

```
┌────────────────────────────────────────────────────────────┐
│  chorister CLI  (thin client)                              │
│  • Creates/updates chorister CRDs on cluster               │
│  • Watches status (K8s Watch API → TUI/UI)                 │
│  • No compilation logic — just CRD CRUD + watch            │
└───────────────────────┬────────────────────────────────────┘
                        │ kubectl apply (CRDs only)
                        ▼
┌────────────────────────────────────────────────────────────┐
│  K8s Cluster                                               │
│                                                            │
│  chorister controller (runs on cluster)                    │
│       │  • Reconciles CRDs → K8s resources directly         │
│       │  • Validates & enforces compliance guardrails        │
│       │  • Reconciles ChoDomainMembership → RoleBindings     │
│       │  • Reconciles ChoPromotionRequest → approval gates   │
│       │                                                     │
│       ├──► K8s native ── Deployment, Service, NetworkPolicy │
│       ├──► Operators ── StackGres, Cilium, cert-manager     │
│       └──► kro ── cloud resources only (object storage)     │
└────────────────────────────────────────────────────────────┘
```

### What chorister does NOT build

- ❌ Cloud resource provisioning APIs (delegated to kro/Crossplane for object storage, etc.)
- ❌ Auth server (OIDC IdPs), security scanners (OPA/Trivy), CI/CD, observability UI (Grafana)

### Deployment model

Two components:

1. **chorister controller** — Deployment in `cho-system`. Watches CRDs, reconciles them directly into K8s resources and operator CRDs. For cloud resources (object storage), delegates to kro/Crossplane. Bootstraps the rest of the stack (operators, observability) via `ChoCluster` CRD reconciliation.
2. **chorister CLI** — thin client. Creates CRDs, watches status. No compilation logic.

```bash
$ chorister setup   # installs ONLY: controller + CRDs into cho-system
                    # controller reads ChoCluster CRD → installs operators, LGTM stack
                    # kro/Crossplane installed only if object storage is configured
```

Why the controller bootstraps the stack (not the CLI):
- Stack IS desired state — if an operator gets deleted, the controller reinstalls it
- `chorister setup` is idempotent (just a CRD). Upgrades = update ChoCluster CRD
- CLI stays truly thin — no Helm, no templating

### Two environments: sandboxes + production

| Environment | Count | Who modifies | How |
|---|---|---|---|
| **Sandbox** | Many per domain | Developer (owner) | `chorister apply` directly |
| **Production** | One per domain | Controller only | **Only via approved ChoPromotionRequest** |

**Core safety invariant:** production can only be modified by the controller after an approved `ChoPromotionRequest`. No `chorister apply` to prod. chorister-managed RBAC does not grant human write access to production namespaces. Any break-glass cluster-admin access outside chorister is outside this contract.

### CLI design: persona-based

```bash
# ─── PLATFORM ADMIN (org-admin) ── rare ──────────────────────────
$ chorister setup                    # bootstrap controller
$ chorister login                    # OIDC authentication
$ chorister admin app create         # create ChoApplication
$ chorister admin app set-policy     # update compliance/HA/promotion policy
$ chorister admin domain create      # create domain
$ chorister admin member add/remove/list
$ chorister admin compliance report

# ─── DOMAIN OWNER (domain-admin) ── weekly ───────────────────────
$ chorister approve/reject <id>      # approve/reject ChoPromotionRequest
$ chorister promote <domain>         # create ChoPromotionRequest
$ chorister requests                 # list pending promotions
$ chorister status <domain>          # domain health across envs

# ─── DEVELOPER ── daily, sandbox-scoped ──────────────────────────
$ chorister apply                    # apply DSL to sandbox (always sandbox)
$ chorister sandbox create/destroy/list
$ chorister diff                     # compare sandbox vs prod
$ chorister status / logs / export
```

| Safety rail | Enforcement |
|---|---|
| `chorister apply` always targets sandbox | CLI default. No `--env prod` flag exists. |
| Production only via `chorister promote` | Only the controller ServiceAccount writes production resources. |
| Promotion requires approval | Controller enforces ChoPromotionRequest policy. |
| Sandbox ownership | Developer can only modify their own sandboxes. |

**Server-side compilation** — single source of truth for compiler version, continuous reconciliation, validation with full cluster context, CLI stays simple.

---

## 2. Hierarchy: Application, Domain, Components

```
Organization (the cluster, implicit)
  └── Application (product + policy boundary)
       └── Domain (bounded context, one team, one K8s namespace)
            └── Components (compute, database, queue, cache, storage, network)
```

| Concept | Scope | Owner | Example |
|---|---|---|---|
| **Application** | Product + policy boundary | Engineering leadership | "mycompany" (startup) or "retail-banking" (enterprise) |
| **Domain** | Bounded context (DDD) | One team | "payments", "orders", "auth" |
| **Components** | Technical resources in a domain | Same team | `compute "api"`, `database "ledger"`, `queue "events"` |

### Application = policy boundary

Startup: one Application. Enterprise: 3-5 (different products with different compliance/HA needs).

Domains inherit all policy from their Application:

| Policy | Set at Application level |
|---|---|
| Compliance profile | `essential` / `standard` / `regulated` (maps internally to CIS Controls, CIS K8s Benchmark, SOC 2, ISO 27001) |
| HA strategy | single-cluster, hot-cold, active-active |
| Promotion policy | required approvers, security scan gates, ticket refs |
| Resource quotas | default CPU/memory/storage per domain |
| Internet egress ceiling | approved external APIs |
| Internet ingress ceiling | approved IdPs, anonymous route policy |

### Domain = bounded context, NOT component

| ✅ Domain | ❌ Not a domain |
|---|---|
| payments, orders, auth, billing, inventory | api, worker, frontend, database |

### DSL example

```
application "myproduct" {
  domain "payments" {
    owners      = ["team-payments@company.com"]
    sensitivity = "confidential"

    consumes "auth"   { services = ["api"]; port = 8080 }
    consumes "orders" { services = ["api"]; port = 8080 }
    supplies          { services = ["api"]; port = 8080 }

    compute "api"     { image = "payments-api:latest"; replicas = 3 }
    compute "worker"  { image = "payments-worker:latest"; replicas = 2 }
    database "ledger" { engine = "postgres"; size = "medium"; ha = true }
    queue "events"    { type = "nats" }
    cache "sessions"  { size = "small" }
  }

  domain "auth" {
    owners      = ["team-identity@company.com"]
    sensitivity = "restricted"

    supplies { services = ["api"]; port = 8080 }

    compute "api"    { image = "auth-api:latest"; replicas = 3 }
    database "users" { engine = "postgres"; size = "medium"; ha = true }
    cache "tokens"   { size = "medium" }
  }
}
```

### `consumes` + `supplies` contract

Controller enforces during reconciliation:
1. **Supply/consume match** — mismatch = reconciliation error
2. **No undeclared access** — NetworkPolicy blocks undeclared traffic
3. **Cycle detection** — cycles = reconciliation error
4. **Port enforcement** — only declared port + namespace

### Access policy levels

| Level | Enforced by | When |
|---|---|---|
| **Level 1** (service) | K8s NetworkPolicy | Always (minimum). |
| **Level 2** (API path) | Cilium L7 CiliumNetworkPolicy | Optional. Required for `restricted` domains. |

### Data sensitivity classification

Domain-level, not per-resource. The domain IS the sensitivity boundary.

| Level | Controller enforces |
|---|---|
| `public` / `internal` (default) | Standard protections: NetworkPolicy, RBAC, audit, encryption at rest. |
| `confidential` | + TLS for all cross-domain traffic (Cilium WireGuard), enhanced audit. |
| `restricted` | + Level 2 access policy required, ChoDomainMembership expiry enforced, access review reminders. |

> **Encryption at rest is always on.** Every database gets encrypted volumes and encrypted backups regardless of sensitivity. Sensitivity levels control *additional* protections.

Application compliance profile sets the org baseline. Domain `sensitivity` can only escalate above it, never weaken it.

### Cross-application links via Gateway API

Cross-app traffic always routes through the internal gateway, never direct pod-to-pod.

| | `consumes` (intra-app) | `link` (cross-app) |
|---|---|---|
| Traffic path | Direct pod-to-pod (NetworkPolicy) | Through internal gateway |
| Auth | None enforced by infra | Gateway-enforced (JWT/API key) |
| Rate limiting | Not needed | Required (CiliumEnvoyConfig) |
| Access control | NetworkPolicy namespace selectors | ReferenceGrant (supplier explicitly allows consumer) |

Compiles to: HTTPRoute (consumer) + ReferenceGrant (supplier) + CiliumNetworkPolicy (L7 auth) + CiliumEnvoyConfig (rate limit + circuit breaker) + NetworkPolicy (blocks direct cross-app).

### K8s mapping

```
Application "myproduct"       → label: chorister.dev/application=myproduct
Domain "payments"              → Namespace: myproduct-payments
                                 Labels: chorister.dev/application=myproduct, chorister.dev/domain=payments
                                 NetworkPolicy: only declared consumes/supplies traffic
```

### ChoApplication CRD

```yaml
apiVersion: chorister.dev/v1alpha1
kind: ChoApplication
metadata:
  name: myproduct
  namespace: cho-system
spec:
  owners: ["cto@company.com"]
  policy:
    compliance: regulated      # essential | standard | regulated
    auditRetention: 2y
    ha:
      strategy: hot-cold
      clusters: { primary: gke-us-east1, failover: eks-us-east-1 }
    promotion:
      requiredApprovers: 1
      allowedRoles: [domain-admin, org-admin]
      requireSecurityScan: true
      requireTicketRef: false
    quotas:
      defaultPerDomain: { cpu: "16 cores", memory: "32Gi", storage: "100Gi" }
    network:
      egress:
        allowlist:
          - host: api.stripe.com
            port: 443
            criticality: high
            expectedLatency: 200ms
            alertOnErrorRate: 5%
          - host: hooks.slack.com
            port: 443
            criticality: low
      ingress:
        allowedIdPs:
          - issuer: "https://login.company.com"
            jwksUri: "https://login.company.com/.well-known/jwks.json"
        allowAnonymousRoutes: true
  domains:
    - name: payments
      owners: ["team-payments@company.com"]
      consumes: [{ domain: auth, services: ["api"], port: 8080 }]
      supplies: { services: ["api"], port: 8080 }
    - name: auth
      owners: ["team-identity@company.com"]
      supplies: { services: ["api"], port: 8080 }
  links:
    - name: capital-markets-data
      target: application/capital-markets
      target_domain: pricing
      port: 8080
      consumers: ["payments"]
      auth: { type: jwt }
      rateLimit: { requestsPerMinute: 1000 }
      circuitBreaker: { consecutiveErrors: 5 }
```

---

## 3. Platform Target: K8s-Only

chorister deploys everything on K8s. No Cloud SQL. No RDS. No Pub/Sub. Same manifests run on GKE, EKS, AKS, bare metal, your laptop.

| Reason | Detail |
|---|---|
| True portability | Same Blueprint runs anywhere K8s runs |
| Cost savings | StackGres is 3-5x cheaper than Cloud SQL |
| Solo dev scope | One compilation target, not N cloud APIs |
| No Crossplane trap | Cloud service abstraction needs hundreds of contributors |

**Rejected:** cloud-service compiler (N providers × M services), serverless containers (removes whole stack), wrap Terraform (BSL license, non-deterministic apply).

**Cloud-native exceptions** (cluster-level, trivially thin API):

| Exception | Why |
|---|---|
| Object storage (S3/GCS/Azure Blob) | 11 9's durability, zero ops |
| Container registry (GCR/ECR/ACR) | Zero ops, built-in scanning |

"I don't want to run Postgres on K8s" — chorister sets it up properly (Patroni auto-failover, PgBouncer, automated backups). If a team insists on Cloud SQL, they provision it outside chorister.

---

## 4. Component Stack

Opinionated choices — one per category, no menu:

| Category | Tool | Role |
|---|---|---|
| Cloud resource composition | kro (or Crossplane) | Cloud-only: object storage provisioning via cloud APIs. Not used for K8s-native or operator-managed resources. |
| PostgreSQL | StackGres | HA via Patroni, PgBouncer pooling, automated backups |
| Queues (standard) | NATS JetStream | Pub/sub, task queues, at-least-once delivery |
| Queues (streaming) | AutoMQ (Strimzi fallback) | Kafka-compatible, S3/GCS-backed |
| Cache | Dragonfly | Redis-compatible, multi-threaded |
| Networking | Cilium | CNI + kube-proxy replacement + Gateway API + NetworkPolicy + Hubble observability |
| TLS | cert-manager | Let's Encrypt wildcard certs |
| Policy | OPA/Gatekeeper | K8s admission control |
| Runtime detection | Tetragon | eBPF syscall monitoring, file integrity (opt-in per compliance profile) |
| Observability | Grafana LGTM | Alloy + Mimir + Loki + Tempo → all on object storage |

`chorister setup` installs the controller. The controller reconciles `ChoCluster` to install operators and the observability stack. If someone deletes an operator, the controller reinstalls it.

### Direct reconcilers vs kro delegation

The controller uses **two reconciliation strategies** based on whether the target is reachable via the K8s API:

| Resource | Strategy | Why |
|---|---|---|
| Deployment, Service, Job, CronJob, HPA, PDB | Direct reconciler | Native K8s resources — no indirection needed |
| StackGres SGCluster, SGPoolingConfig, SGBackupConfig | Direct reconciler | Operator CRDs — K8s API reachable |
| NATS StatefulSet/operator CRDs | Direct reconciler | K8s API reachable |
| Dragonfly Deployment | Direct reconciler | K8s API reachable |
| NetworkPolicy, CiliumNetworkPolicy, CiliumEnvoyConfig | Direct reconciler | K8s API reachable |
| Gateway API HTTPRoute, ReferenceGrant | Direct reconciler | K8s API reachable |
| cert-manager Certificate | Direct reconciler | K8s API reachable |
| Tetragon TracingPolicy | Direct reconciler | K8s API reachable |
| RBAC RoleBindings | Direct reconciler | K8s API reachable |
| **Object storage (S3/GCS/Azure Blob)** | **kro / Crossplane** | **Requires cloud provider APIs — controller should not embed cloud SDKs** |

**Decision boundary:** "Can the controller create this resource with a K8s API client?" Yes → direct reconciler. No (requires cloud provider API) → delegate to kro/Crossplane.

This keeps the controller free of cloud SDK dependencies while still managing the full stack. If a team needs a cloud-managed database (Cloud SQL, RDS), they provision it outside chorister.

---

## 5. State & Reconciliation Model

```
CLI creates/updates CRDs (desired state in etcd)
  → controller watches, validates, reconciles
    → creates K8s resources directly (Deployments, Services, operator CRDs, etc.)
    → for cloud resources only: creates kro RGDs (object storage provisioning)
  → controller writes audit event to Loki (synchronous)
  → controller updates CRD .status (CLI watches via K8s Watch API)
```

| Aspect | Handled by |
|---|---|
| Desired state | etcd (K8s CRDs) |
| K8s-native resources | chorister controller (direct reconciliation) |
| Operator-managed resources | chorister controller → operator CRDs (StackGres, Cilium, etc.) |
| Cloud resources | kro/Crossplane (object storage, container registry) |
| Watch/notify | K8s informers (HTTP/2 streaming, millisecond propagation) |

**Why direct reconcilers for most resources:** The controller already has a K8s client. Creating a Deployment or an SGCluster via the K8s API is a single API call. Routing through kro RGDs adds indirection (RGD → kro reconciler → K8s API) with no functional benefit for resources the controller can manage directly. Direct reconcilers are simpler to debug, test, and reason about.

**Why kro/Crossplane for cloud resources:** Object storage (S3/GCS/Azure Blob) requires cloud provider APIs. Embedding AWS/GCP/Azure SDKs in the controller would bloat the binary, add authentication complexity, and create a maintenance burden across provider versions. kro or Crossplane already solve this with provider plugins.

**Rejected:** kro for all resources (unnecessary indirection for K8s-native resources), own state file (Terraform's worst parts), cloud APIs as source of truth (slow, no diffs).

---

## 6. Blueprint Artifact Format

The controller reconciles each domain’s CRDs into K8s resources applied directly to the cluster. For cloud resources, kro RGDs are used:

```
k8s/                                  # direct reconciler output
  namespace.yaml
  networkpolicy.yaml
  deployment-api.yaml
  service-api.yaml
  sgcluster-main.yaml                 # StackGres operator CRD
  ciliumnetworkpolicy-egress.yaml
kro/                                  # cloud resources only
  rgd-storage-uploads.yaml            # kro RGD for object storage
```

All standard K8s manifests: `kubectl get` works, compiled state visible in CRD `.status`, exportable for GitOps via `chorister export`.

---

## 7. DSL / Input Format

| Format | Pros | Cons |
|---|---|---|
| YAML | No parser, universal | Verbose, no types |
| CUE | Typed, Go-native, constraints built-in | Learning curve |
| Custom DSL (HCL-like) | Clean syntax, full control | Build parser, no editor support |

Examples elsewhere in this document use HCL-like pseudo-syntax to illustrate the resource model. The committed decision is server-side compilation; the concrete authoring format for the first implementation remains open.

**Recommendation: CUE or YAML — decide Week 1.** Compiler sits between DSL and Blueprint; swapping DSL doesn't change anything downstream.

CUE guardrails example:

```cue
database: [Name=_]: {
    backups: "daily" | "hourly" | "continuous"  // cannot be empty
    _network: "private"                          // hidden, always private
}
```

---

## 8. Provider / Plugin Architecture

```go
type Provider interface {
    Name() string
    Compile(resource Resource, env Environment) ([]Manifest, error)
    Validate(resource Resource, env Environment) []ValidationError
    PreFlight(manifests []Manifest, env Environment) []PreFlightError
}
```

Day 1 provider: `k8s`. Runs inside the controller. Adding a resource type = implementing `Compile()` for that resource. User-facing CRD is always `ChoDatabase`, `ChoCompute`, etc.

---

## 9. Resource Types & Compilation Targets

6 resource types, each compiles to K8s manifests only.

| Resource type | Variant | K8s target |
|---|---|---|
| `compute` | long-running | Deployment + Service + HPA + PDB |
| `compute` | scale-to-zero | TBD (Knative / KEDA / GKE Autopilot) |
| `compute` | job | Job / CronJob |
| `compute` | GPU | Job or Deployment + `nvidia.com/gpu` limit |
| `database` | postgresql | StackGres SGCluster + SGPoolingConfig + SGBackupConfig |
| `queue` | standard | NATS JetStream |
| `queue` | streaming | AutoMQ (Strimzi fallback) |
| `storage` | object | S3 / GCS / Azure Blob |
| `storage` | block/file | PVC |
| `cache` | — | Dragonfly Deployment + Service |
| `network` | — | NetworkPolicy + HTTPRoute + CiliumNetworkPolicy |

**Database HA:** single `ha = true` flag. `false` → 1 instance. `true` → 2+ with Patroni auto-failover. Never expose replication topology.

**Encryption at rest:** always on. Controller selects encrypted StorageClass. `chorister setup` validates one exists. No option to disable.

---

## 10. Network & Security Model

### 3-zone trust model

| Zone | Default | Auth |
|---|---|---|
| Intra-domain (same namespace) | ✅ Allow | None |
| Cross-domain (intra-app) | ❌ Deny | `consumes`/`supplies` → NetworkPolicy |
| Internet ingress | ❌ Deny | JWT required (infra-enforced) |
| Internet egress | ❌ Deny | Application-level allowlist |
| Cross-application | ❌ Deny | Bilateral `link` → Gateway API |

### Egress: application ceiling, domain selection

Application sets the approved external APIs. Domains select a subset. If a domain references an unapproved API → compile error (even in sandbox).

Adding a new external API requires **two independent flows**: (1) platform admin adds to application allowlist, (2) domain promotes with the new egress declaration. Separate CRDs, separate approvals.

### Ingress: auth-required by default

If `from = "internet"`, JWT auth is mandatory unless a route is explicitly marked `auth = "none"`. The IdP must be in the Application's `allowedIdPs`.

```
network "api-boundary" {
  ingress {
    from = "internet"; port = 443
    auth { jwt { jwks_uri = "..."; issuer = "..."; audience = ["..."] } }
    routes {
      "/api/*"           {}                             # auth required (default)
      "/api/admin/*"     { claims { role = "admin" } }  # auth + claim
      "/healthz"         { auth = "none" }              # explicit anonymous
      "/webhooks/stripe" { auth = "none", hmac = true } # webhook
    }
  }
}
```

Compiles to: Gateway API HTTPRoute + CiliumNetworkPolicy L7 (JWT verification).

### Guardrails (compile errors)

- Internet ingress without auth block
- `auth = "none"` on all routes
- Wildcard egress (`allow = ["*"]`)
- Egress to unapproved destinations

### Cilium as unified networking

Cilium replaces CNI + kube-proxy + gateway controller + NetworkPolicy enforcement + network observability in one DaemonSet. Native support on GKE ("Dataplane V2"), AKS ("Azure CNI Powered by Cilium"), and EKS (Helm install).

Cilium-specific CRDs used: CiliumNetworkPolicy (L7/JWT, FQDN egress), CiliumEnvoyConfig (rate limiting, circuit breaker).

### Egress health monitoring

Hubble captures per-FQDN egress metrics → Mimir stores → controller generates Grafana alerting rules from allowlist metadata (`criticality`, `expectedLatency`, `alertOnErrorRate`). Results in `ChoApplication.status.egressHealth`.

### Runtime threat detection (Tetragon)

| Compliance profile | Detection level | Overhead |
|---|---|---|
| `essential` + public/internal | Off (Hubble network monitoring only) | ~0% |
| `standard` | Network anomaly detection (Hubble alerting) | ~0% |
| `regulated` | Full Tetragon (syscall, file integrity, process exec) | ~1-2% CPU |
| Any profile + `restricted` | Full Tetragon | ~1-2% CPU |

Domains can opt-in explicitly: `runtimeDetection = "full"`.

Why Tetragon over Falco: same eBPF stack as Cilium, TracingPolicy CRDs (policy-as-code), events flow to same Loki pipeline.

---

## 11. Identity & Access Control

### 3-layer model

```
Layer 1: Authentication — OIDC IdP (external)
Layer 2: Authorization — ChoDomainMembership CRDs
Layer 3: Enforcement — K8s RBAC + NetworkPolicy
```

No user database. OIDC IdP = identity. etcd (CRDs) = authorization. K8s RBAC = enforcement.

### RBAC roles

| Role | Sandbox | Production |
|---|---|---|
| org-admin | admin (all sandboxes) | view |
| domain-admin | admin (all sandboxes) | view |
| developer | edit (own sandboxes only) | view |
| viewer | view | view |

Human roles are read-only in production namespaces. Platform-level administration happens through `cho-system` and other control-plane objects, not by writing workload resources in production namespaces.

### ChoDomainMembership CRD

```yaml
apiVersion: chorister.dev/v1alpha1
kind: ChoDomainMembership
metadata:
  name: alice-payments
  namespace: cho-system
spec:
  application: myproduct
  domain: payments
  identity: alice@company.com
  role: developer
  source: oidc-group          # "manual" | "oidc-group"
  oidcGroup: "team-payments"
  expiresAt: "2026-07-14T00:00:00Z"
```

Controller watches ChoDomainMembership → creates K8s RoleBindings.

### Automated access deprovisioning

| Mechanism | What it solves |
|---|---|
| **OIDC group sync** | Employee leaves IdP group → controller deletes membership + RoleBindings |
| **TTL/expiry** | Safety net if group sync fails |
| **Manual audit** | `chorister admin member audit` flags stale accounts |

For `restricted` domains or `regulated` applications, `expiresAt` is required on all memberships.

---

## 12. Audit, Compliance & Approval Gates

### Two audit logs

| Log | Records | Storage |
|---|---|---|
| chorister intent log | Who promoted, who approved, what DSL was compiled | Loki (object storage, default 2y retention) |
| K8s audit log | Actual API calls, resource mutations | K8s-native |

Intent log is **controller-driven and synchronous** — if Loki rejects the write, reconciliation fails. No client can bypass the audit trail.

### Security scanning: aggregate, don't build

| Layer | Tool | Integration |
|---|---|---|
| Policy-as-code | OPA/Gatekeeper | K8s admission — chorister installs constraint templates |
| Image scanning | Trivy / Grype | chorister reads scan results before promote |
| Continuous scanning | CronJob per domain | `standard`+: daily re-scan of deployed images → ChoVulnerabilityReport CRDs |
| Cluster hardening | kube-bench | Periodic CronJob, results in ChoCluster.status.cisBenchmark |

### ChoPromotionRequest

```yaml
apiVersion: chorister.dev/v1alpha1
kind: ChoPromotionRequest
metadata:
  name: payments-prod-042
  namespace: cho-system
spec:
  domain: payments
  sandbox: alice
  requestedBy: alice@company.com
  diff: |
    compute/api-server: replicas 2 → 3
    database/orders-db: tier small → medium
  externalRef: "JIRA-4521"
  policy:
    requiredApprovers: 1
    allowedRoles: [domain-admin, org-admin]
status:
  phase: Pending  # → Approved → Executing → Completed / Rejected / Failed
  approvals:
    - approver: bob@company.com
      role: domain-admin
      approvedAt: "2026-04-13T14:30:00Z"
```

Sandboxes are free. Production always requires approval. Not configurable — platform invariant.

### Incident response: service health baseline

chorister detects and responds to service degradation — not full incident management (use PagerDuty/Opsgenie for that).

| Signal | Controller action |
|---|---|
| Pod crash loops | Flag domain as Degraded, emit Grafana alert |
| Deployment stalled | Degraded + block further promotions |
| Database unhealthy | Flag, alert, trigger backup verification |
| Egress provider degraded | Flag in status.egressHealth, alert |
| New CVE in running image | Flag in status.vulnerabilities, alert |

Lifecycle: `chorister admin isolate` (tighten NetworkPolicy, freeze promotions) → investigate → `chorister promote --rollback` or fix forward → `chorister admin unisolate`.

---

## 13. Observability

Grafana LGTM on object storage. Not a chorister resource type — users interact with Grafana directly.

| Component | Role | Storage |
|---|---|---|
| Grafana Alloy | Collection agent | Stateless |
| Mimir | Metrics (Prometheus-compatible) | Object storage |
| Loki | Logs + audit events | Object storage |
| Tempo | Distributed tracing | Object storage |
| Grafana | Dashboards + alerting | ConfigMap |

All backends share one object storage bucket. Default retention: 30d metrics, 14d logs, 7d traces (configurable).

---

## 14. Real-time Feedback

kro uses real K8s informers (HTTP/2 streaming, not polling) for cloud resources it manages. For directly-reconciled resources, the controller uses its own informers. Status changes propagate in milliseconds:

```
Operator updates resource.status = "Ready"
  → K8s Watch event → kro updates parent .status
  → CLI watching → TUI renders
```

```
$ chorister apply --domain payments --sandbox alice

  Namespace payments--alice          ✓ created (0.1s)
  Deployment api                     ✓ 1/1 ready (4.2s)
  SGCluster main-db                  ⏳ creating... (1m23s)
  NatsCluster events                 ✓ ready (2.1s)
  SGCluster main-db                  ✓ ready (5m12s)
  ✅ Sandbox alice ready!
```

---

## 15. Naming & Resource Identity

```
Resource:  {domain}--{resource-type}--{name}     # payments--compute--api
Namespace: {application}-{domain}                 # myproduct-payments
Sandbox:   {application}-{domain}-sandbox-{name}  # myproduct-payments-sandbox-alice
```

Production has no suffix. Sandboxes always have `-sandbox-{name}`. Impossible to confuse the two.

---

## 16. Secret Management

Blueprints declare typed secret slots. Each environment binds slots to a backend:

```yaml
secrets:
  - name: DATABASE_PASSWORD
    type: string
environments:
  production:
    secrets:
      DATABASE_PASSWORD:
        source: gcp-secret-manager
        ref: projects/myproject/secrets/payments-db-password/versions/latest
  sandbox:
    secrets:
      DATABASE_PASSWORD:
        source: k8s-secret
        ref: auto-generated
```

Backends: K8s Secrets (start here), GCP Secret Manager, AWS Secrets Manager, HashiCorp Vault.

---

## 17. Platform Gaps & Deferred Decisions

| Gap | Decision | Status |
|---|---|---|
| **DNS** | Wildcard DNS + Gateway API routing. Manual `*.company.com → <LB IP>` during setup. cert-manager wildcard cert. | MVP |
| **CI/CD** | Out of scope. `compute.source` is an image reference. chorister provides example CI configs. | Not MVP |
| **Backup/DR** | StackGres automated backups + Loki on object storage now. etcd backup + PVC snapshots post-MVP. | Partial |
| **GitOps** | Push-based for MVP (`chorister promote`). `chorister export` dumps compiled Blueprint for ArgoCD/Flux. | MVP |

---

## 18. Multi-cluster & Scaling Path

HA strategy is Application-level policy. `ha = true` on a component = HA within a cluster. `policy.ha.strategy` = HA across clusters.

| Strategy | Traffic | Data | Use case |
|---|---|---|---|
| single-cluster | All to one cluster | Local only | MVP, internal tools |
| hot-cold | Primary active, failover idle | Async replication | Production, RTO < 1h |
| active-active | Load balanced | Sync/conflict-resolution | Mission-critical |

**Growth path:** Phase 1 (MVP): single-cluster → Phase 2: hot-cold (same region, cross-cloud) → Phase 3: active-active → Phase 4: geographic distribution.

---

## 19. Implementation Timeline

```
WEEK 1:
├── Decide: YAML vs CUE, Scale-to-zero engine
├── Define: chorister CRDs
├── Scaffold: Go project, controller, CLI skeleton
└── Setup: dev K8s cluster with StackGres + NATS + OIDC

WEEK 2:
├── Build: Controller core (watch CRDs, reconcile directly)
├── Build: K8s provider (compute → Deployment + Service + HPA)
├── Build: Controller audit (Loki writes on every reconciliation)
├── Build: CLI thin client (CRD CRUD + Watch API → TUI)
├── Build: ChoDomainMembership reconciler
└── Test: end-to-end compute flow

WEEK 3-4:
├── Build: database provider (→ StackGres)
├── Build: queue provider (→ NATS)
├── Build: network resource (→ NetworkPolicy + HTTPRoute + CiliumNetworkPolicy)
├── Build: Object storage → kro/Crossplane RGDs
└── Test: compute + database + network → working app

WEEK 5-6:
├── Build: Sandbox create/destroy
├── Build: Diff engine, Promote command
├── Build: Object storage provisioning
└── Test: full loop — sandbox → verify → diff → promote

WEEK 7-8:
├── Build: Guardrails (compile-time rejection)
├── Build: `chorister setup` (controller bootstraps stack)
├── Build: ChoPromotionRequest approval flow
├── Polish: error messages, TUI, docs
└── Milestone: complete flow demo
```

---

## 20. Stateful Resource Deletion Safety

**Core safety invariant:** removing a stateful resource (database, queue, storage) from the DSL and promoting must never cause immediate data loss. Stateful resources in production use an **archive lifecycle**, not soft-delete.

### Archive lifecycle

When a stateful resource is removed from the DSL, the controller does not delete it. It transitions the resource to `Archived` state.

```
Active → Archived (resource removed from DSL + promoted)
  → Archived for 30 days (data intact, connections refused)
    → Deletable (after retention period, explicit delete required)
```

| State | Data | Connections | Visible in status/UI | Billed/counted |
|---|---|---|---|---|
| `Active` | Read/write | Open | Yes, as active | Yes |
| `Archived` | Read-only (backups continue) | Refused — dependents get compile error | Yes, marked `archived` | Yes |
| `Deletable` | Read-only | Refused | Yes, marked `deletable` | Yes |
| `Deleted` | Gone (final backup snapshot retained) | N/A | No | No |

### Rules

1. **Promotion creates the archive.** Removing `database "ledger"` from the DSL and promoting transitions the production resource to `Archived`. The sandbox resource is deleted immediately (sandboxes are ephemeral).
2. **Archived resources block dependents.** Any `compute` that references an archived database gets a compile error. This forces teams to explicitly clean up references, not silently lose a dependency.
3. **30-day retention is mandatory.** The controller enforces a minimum 30-day archive period before the resource becomes `Deletable`. Configurable upward at the Application level (`policy.archiveRetention`), never downward.
4. **Deletion is explicit.** After the retention period, the resource moves to `Deletable` but is NOT automatically deleted. A platform admin must run `chorister admin resource delete --archived <resource>` to finalize. This is an audited action.
5. **Final snapshot.** On transition to `Deleted`, the controller takes a final backup snapshot (StackGres backup, NATS stream snapshot) to object storage. Snapshot retention follows the Application's `auditRetention` policy.
6. **Sandboxes are exempt.** Sandbox stateful resources are deleted immediately on sandbox destruction. No archive lifecycle — sandboxes are disposable by design.

### CRD status

```yaml
status:
  lifecycle: Archived          # Active | Archived | Deletable
  archivedAt: "2026-04-14T00:00:00Z"
  deletableAfter: "2026-05-14T00:00:00Z"
  finalSnapshotRef: "s3://backups/myproduct-payments/ledger/final-20260514.sql.gz"
```

### What this prevents

- Accidental `database "ledger"` removal deleting production data
- Renaming a resource (remove + add) silently dropping the old one
- Cascading failures from removing a dependency that other components still reference

**Rejected:** soft-delete (ambiguous — is it running or not?), immediate delete with confirmation prompt (CLI prompts are not safe for automation), garbage-collection timers (invisible countdown).

---

## 21. Controller Upgrade & CRD Versioning

### Controller upgrade: blue-green with version tags

Follows the Istio revision model. Two controller versions can coexist on the same cluster. Namespaces declare which controller version manages them.

```yaml
apiVersion: chorister.dev/v1alpha1
kind: ChoCluster
metadata:
  name: cluster-config
spec:
  controller:
    revisions:
      - name: "1-4"          # currently active
        tag: stable
      - name: "1-5"          # canary
        tag: canary
```

Namespaces are tagged with `chorister.dev/rev`:

```yaml
metadata:
  labels:
    chorister.dev/rev: "1-4"   # this namespace is managed by controller 1-4
```

### Upgrade procedure

1. **Install new revision.** `chorister admin upgrade --revision 1-5` deploys the new controller alongside the old one. Both run simultaneously.
2. **Canary.** Retag one low-risk domain's namespace to `chorister.dev/rev: "1-5"`. The new controller picks it up; the old controller ignores it. Verify.
3. **Roll forward.** Retag remaining namespaces. `chorister admin upgrade --promote 1-5` retags all namespaces and sets the new revision as `stable`.
4. **Rollback.** If canary fails, retag namespace back. Old controller resumes. `chorister admin upgrade --rollback 1-5` removes the canary revision.
5. **Cleanup.** After all namespaces are on the new revision, remove the old controller deployment.

Each controller revision only reconciles namespaces matching its `chorister.dev/rev` label. Untagged namespaces default to the `stable` revision.

### CRD versioning

| Principle | Rule |
|---|---|
| **Backward compatible within a major** | New fields are additive with defaults. Existing fields are never removed or renamed within `v1alpha1` → `v1` lineage. |
| **Version bump = storage migration** | `v1alpha1` → `v1beta1` → `v1`. Each bump includes a conversion webhook. Old CRDs continue to work via conversion. |
| **No silent recompilation** | A controller upgrade that changes compiled output (e.g., different Deployment spec from the same DSL) must record the change in the audit log and show it in `chorister diff`. |
| **CRD schema validation** | Controller validates that its CRD schemas are compatible with existing resources on startup. Refuses to start if it would break existing CRDs, logs the incompatibility. |

### Compilation stability

The controller records a `compiledWithRevision` field in each resource's status. `chorister diff` shows when compiled output differs between the current controller and the running state, even if the DSL hasn't changed. This makes controller upgrades visible in the normal promotion flow.

---

## 22. Sandbox Lifecycle & FinOps Quotas

### Sandbox limits

Sandbox creation is governed by a **domain budget**, not a hard sandbox count. The budget is configurable at both Application and Domain level (domain overrides application default).

```yaml
apiVersion: chorister.dev/v1alpha1
kind: ChoApplication
metadata:
  name: myproduct
spec:
  policy:
    sandbox:
      defaultBudgetPerDomain: "$500/month"    # estimated cost ceiling for all sandboxes in a domain
      maxIdleDays: 7                            # auto-destroy after N days of no applies
      budgetAlertThreshold: 80                  # percent — warn domain owner
```

Domain-level override:

```yaml
domains:
  - name: payments
    sandbox:
      budget: "$1000/month"    # payments team gets a larger sandbox budget
      maxIdleDays: 14
```

### FinOps-based quotas

Rather than counting sandboxes or CPU cores, quotas are expressed as **estimated monthly cost**. The controller calculates sandbox cost from resource declarations:

| Resource | Cost model |
|---|---|
| `compute` | vCPU × hours + memory × hours (configurable rates in ChoCluster) |
| `database` | Instance size × hours + storage GB |
| `queue` | Instance count × hours |
| `cache` | Memory size × hours |

Platform admins set the per-unit rates in `ChoCluster.spec.finops.rates` to match their actual infrastructure costs (cloud bill, bare-metal amortization, etc.).

```yaml
apiVersion: chorister.dev/v1alpha1
kind: ChoCluster
metadata:
  name: cluster-config
spec:
  finops:
    rates:
      cpuPerHour: "$0.03"          # per vCPU-hour
      memoryGBPerHour: "$0.004"    # per GB-hour
      storageGBPerMonth: "$0.10"   # per GB-month
      postgresSmall: "$50/month"   # flat rate per size tier
      postgresMedium: "$150/month"
      postgresLarge: "$400/month"
```

### Behavior

| Event | Action |
|---|---|
| `chorister sandbox create` | Controller estimates cost of the sandbox spec. If domain total would exceed budget → reject with cost breakdown. |
| Budget at 80% (configurable) | Alert domain owner via status condition + Grafana alert. |
| Budget exceeded | Block new sandbox creation. Existing sandboxes continue running. |
| Sandbox idle > `maxIdleDays` | Controller destroys sandbox. Warns 24h before via status. |
| Domain owner requests increase | Update domain sandbox budget (requires org-admin if above application default). |

### Status visibility

```yaml
status:
  sandbox:
    activeSandboxes: 3
    estimatedMonthlyCost: "$340"
    budget: "$500/month"
    budgetUsagePercent: 68
```

This model lets a team with a $500 budget run one big sandbox or five small ones — their choice. It also sets the foundation for production FinOps reporting in later phases (same cost model, applied to production namespaces).

### What this prevents

- Runaway sandbox sprawl (forgotten sandboxes consuming cluster resources)
- Unfair resource distribution between teams
- Cost surprises from idle infrastructure

**Deferred:** production FinOps (cost attribution per domain, chargeback reports, showback dashboards). Same cost model, applied to production namespaces post-MVP.

---

## 23. Resource Sizing Model

Resource sizing uses **named templates** rather than fixed small/medium/large tiers. Platform admins define sizing templates in `ChoCluster`; domain teams reference them by name.

### Why templates, not fixed tiers

- Fixed tiers assume one mapping fits all clusters. A "medium" Postgres on a 64-core bare-metal node is different from "medium" on a 4-vCPU Kind cluster.
- Templates let platform admins tune sizing to their actual hardware and cost model.
- Teams can request new templates without changing the DSL schema.

### Template definition (platform admin)

```yaml
apiVersion: chorister.dev/v1alpha1
kind: ChoCluster
metadata:
  name: cluster-config
spec:
  sizingTemplates:
    database:
      small:  { cpu: "500m",  memory: "1Gi",  storage: "10Gi",  instances: 1 }
      medium: { cpu: "2",     memory: "4Gi",  storage: "50Gi",  instances: 2 }
      large:  { cpu: "4",     memory: "16Gi", storage: "200Gi", instances: 3 }
      xlarge: { cpu: "8",     memory: "32Gi", storage: "500Gi", instances: 3 }
    cache:
      small:  { cpu: "250m",  memory: "512Mi" }
      medium: { cpu: "1",     memory: "2Gi"   }
      large:  { cpu: "2",     memory: "8Gi"   }
    queue:
      small:  { cpu: "250m",  memory: "512Mi", replicas: 1 }
      medium: { cpu: "1",     memory: "2Gi",   replicas: 3 }
      large:  { cpu: "2",     memory: "4Gi",   replicas: 3 }
```

### Usage in DSL

```
database "ledger" { engine = "postgres"; size = "medium"; ha = true }
cache "sessions"  { size = "small" }
```

`size` references a template name. If the name doesn't exist in `ChoCluster.spec.sizingTemplates` → compile error.

### Explicit override

Teams can bypass templates with explicit resource specs when a template doesn't fit:

```
database "analytics" {
  engine   = "postgres"
  ha       = true
  cpu      = "6"
  memory   = "24Gi"
  storage  = "1Ti"
}
```

Explicit values override the template entirely. The controller still validates against namespace ResourceQuota.

### Defaults

chorister ships sensible defaults in the CRD (matching the `small` tier above). `chorister setup` creates default templates. Platform admins customize post-setup.

---

## Decision Summary

| # | Decision | Choice |
|---|---|---|
| 1 | Platform target | K8s-only. Exceptions: object storage, container registry. |
| 2 | Architecture | K8s operator. Controller reconciles CRDs directly into K8s resources. CLI is thin (CRD CRUD + watch). |
| 3 | State | K8s as control plane. etcd for state, direct reconcilers for K8s-reachable resources, kro/Crossplane for cloud resources only. |
| 4 | Hierarchy | Organization → Application → Domain → Components. Domain = DDD bounded context = namespace. |
| 5 | Resource types | 6: compute, database, queue, storage, cache, network. K8s manifests only. |
| 6 | DSL format | Not finalized yet. CUE or YAML for initial implementation; examples are illustrative only. |
| 7 | Blueprint | K8s manifests (direct reconciler output) + kro RGDs for cloud resources only. Exportable for GitOps. |
| 8 | Network | 3-zone trust. Intra-domain=free. Cross-domain=consumes/supplies. Internet=JWT. Cross-app=Gateway API. |
| 9 | Identity | OIDC + ChoDomainMembership → K8s RoleBindings. No user database. |
| 10 | Audit | Controller writes to Loki (synchronous) + K8s audit log. Tamper-proof. |
| 11 | Approval | ChoPromotionRequest CRD. Production always requires approval. |
| 12 | Observability | Grafana LGTM (Alloy + Mimir + Loki + Tempo) on object storage. |
| 13 | Networking | Cilium (CNI + Gateway API + NetworkPolicy + Hubble). |
| 14 | Database | StackGres. Patroni HA, PgBouncer, automated backups. |
| 15 | Queue | NATS (standard) + AutoMQ/Strimzi (streaming). |
| 16 | Cache | Dragonfly (Redis-compatible). |
| 17 | Multi-cluster | Single cluster for MVP. Hot-cold then active-active post-MVP. |
| 18 | CI/CD | Out of scope. chorister promotes images, not code. |
| 19 | GitOps | Push-based MVP. Exportable Blueprints for ArgoCD/Flux. |
| 20 | Stateful deletion safety | Archive lifecycle: Archived → 30-day retention → explicit delete. No accidental data loss. |
| 21 | Controller upgrade | Blue-green with namespace version tags (Istio revision model). CRDs backward-compatible within major. |
| 22 | Sandbox lifecycle | FinOps-based quotas (estimated cost budget per domain). Auto-destroy idle sandboxes. |
| 23 | Resource sizing | Named templates in ChoCluster, customizable by platform admin. Explicit overrides allowed. |
