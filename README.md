# chorister

An opinionated infrastructure platform that runs as a Kubernetes operator. Define your entire stack in a DSL, get production-grade infrastructure with security, compliance, and networking built in — not bolted on.

This README describes the intended end-state product shape and UX. See `ROADMAP.md` for what is planned first versus what is already implemented.

---

## This is an illustrative full-stack app

```hcl
application "saas-product" {

  policy {
    compliance = "standard"   // essential | standard | regulated
    promotion {
      requiredApprovers = 1
      requireSecurityScan = true
    }
    network {
      egress {
        allow "stripe"  { host = "api.stripe.com";  port = 443 }
        allow "slack"   { host = "hooks.slack.com";  port = 443 }
      }
      ingress {
        allowedIdPs = ["https://login.company.com"]
      }
    }
  }

  domain "auth" {
    owners      = ["team-identity@company.com"]
    sensitivity = "confidential"
    supplies    { services = ["api"]; port = 8080 }

    compute "api"    { image = "auth-api:v2.1.0"; replicas = 3 }
    database "users" { engine = "postgres"; size = "medium"; ha = true }
    cache "sessions" { size = "small" }
  }

  domain "payments" {
    owners      = ["team-payments@company.com"]
    sensitivity = "confidential"
    consumes "auth" { services = ["api"]; port = 8080 }
    supplies        { services = ["api"]; port = 8080 }

    compute "api"     { image = "payments-api:v3.0.1"; replicas = 3 }
    compute "worker"  { image = "payments-worker:v3.0.1"; replicas = 2 }
    database "ledger" { engine = "postgres"; size = "large"; ha = true }
    queue "events"    { type = "nats" }
  }

  domain "dashboard" {
    owners = ["team-product@company.com"]
    consumes "auth"     { services = ["api"]; port = 8080 }
    consumes "payments" { services = ["api"]; port = 8080 }

    compute "web" { image = "dashboard:v1.4.0"; replicas = 2 }
    cache "views" { size = "small" }

    network "public" {
      ingress {
        from = "internet"; port = 443
        auth { jwt { issuer = "https://login.company.com" } }
        routes {
          "/app/*"    {}
          "/healthz"  { auth = "none" }
        }
      }
    }
  }
}
```

The example above uses HCL-like pseudo-syntax to show the model clearly. The first implementation will finalize the input format as YAML or CUE; the resource model and workflow are the committed parts.

That's it. `chorister apply` and you get:

- **3 isolated namespaces** with deny-all NetworkPolicy
- **HA Postgres** (Patroni failover, PgBouncer pooling, automated backups) — no Cloud SQL bill
- **NATS JetStream** queue, **Dragonfly** caches — all on-cluster
- **Zero-trust networking** — payments can reach auth:8080, dashboard can reach both, nothing else is allowed
- **JWT-authenticated ingress** via Gateway API + Cilium L7 filtering
- **Egress locked** to Stripe and Slack only — no surprise outbound traffic
- **`standard` compliance** enforced: image scanning before promotion, audit logs to Loki, OPA admission policies, RBAC lifecycle, HA databases, continuous vulnerability scanning
- **Grafana dashboards** with metrics, logs, and traces wired automatically

All running on any K8s cluster. Same manifests on GKE, EKS, bare metal, or your laptop with Kind.

---

## Why chorister?

**Infrastructure platforms today are either too flexible or too fragile.**

Terraform gives you infinite knobs and a state file that drifts. Helm charts are copy-paste YAML with string templating. Platform teams spend months wiring together 15 tools that each solve 10% of the problem. Developers wait weeks for "just a Postgres database."

chorister takes a different stance: **one opinionated path, enforced by a K8s operator.**

You declare what you need — compute, database, queue, cache, network boundaries — in a high-level DSL. The chorister controller compiles it into production-ready K8s manifests, wires up networking, enforces security policies, and reconciles continuously. No Terraform. No cloud-specific APIs. No glue scripts.

### The bet

- **K8s is the universal control plane.** If it runs on K8s, it runs anywhere — GKE, EKS, AKS, bare metal, your laptop.
- **Opinions beat optionality.** One database engine (StackGres), one queue (NATS), one cache (Dragonfly), one CNI (Cilium). Fewer choices = fewer failure modes.
- **Compliance is a compiler feature, not an audit scramble.** CIS Controls, SOC 2, ISO 27001, CIS K8s Benchmark — enforced at compile time and reconciliation, not checked after the fact.

---

## Killer Features

### 1. Sandbox-first development, deterministic promotion

Developers work in isolated sandboxes. In normal chorister workflows, humans do not write production namespaces directly. Production changes flow through `ChoPromotionRequest` approvals and are applied by the controller.

```
chorister apply        → always targets a sandbox
chorister diff         → compare sandbox vs production
chorister promote      → create a promotion request (requires approval)
```

There is no `--env prod` flag. chorister-managed RBAC keeps human roles read-only in production namespaces; any break-glass cluster access outside chorister is outside this product contract.

### 2. Zero-trust networking from a dependency graph

Declare which domains consume and supply services. The controller compiles this into NetworkPolicy, CiliumNetworkPolicy (L7), and Gateway API routes. Everything else is **deny by default**.

```
domain "payments" {
  consumes "auth"   { services = ["api"]; port = 8080 }
  consumes "orders" { services = ["api"]; port = 8080 }
  supplies          { services = ["api"]; port = 8080 }
}
```

No undeclared traffic. No forgotten firewall rules. The dependency graph IS the network policy.

### 3. Compliance as code — one word, four frameworks

Set your compliance posture at the application level. Pick a profile. The controller maps it to the right combination of CIS Controls, CIS K8s Benchmark, SOC 2, and ISO 27001 — and enforces it everywhere, automatically.

```hcl
compliance = "standard"   // essential | standard | regulated
```

| Profile | When to use | What it maps to internally |
|---|---|---|
| `essential` | Internal tools, dev environments | CIS Controls IG1, CIS K8s Level 1 |
| `standard` | Production SaaS, customer-facing | CIS Controls IG2, CIS K8s Level 1, SOC 2 (Security + Availability) |
| `regulated` | Banking, healthcare, government | CIS Controls IG3, CIS K8s Level 2, SOC 2 (Security + Availability + Confidentiality), ISO 27001 |

This single word activates: OPA constraints, image scanning gates, Tetragon runtime detection, audit log retention, access review automation, encrypted storage, and dozens of other controls. Each mapped to specific framework safeguards. You don't need to know what "IG2" means.

### 4. The controller bootstraps everything

`chorister setup` installs the controller and CRDs. The controller reads `ChoCluster` and installs the rest: kro, StackGres, NATS, Dragonfly, Cilium, Grafana LGTM, OPA, cert-manager. If an operator gets deleted, the controller reinstalls it. The stack IS desired state.

### 5. Server-side compilation, thin CLI

The CLI is a thin client — it creates CRDs and watches status via the K8s Watch API. All compilation, validation, and enforcement happens in the controller on-cluster. One compiler version. Full cluster context for validation. No client-side drift.

### 6. DDD-native hierarchy

```
Application (policy boundary)
  └── Domain (bounded context = namespace = team)
       └── Components (compute, database, queue, cache, storage, network)
```

Domains are bounded contexts (payments, orders, auth), not infrastructure layers (api, worker, frontend). Data sensitivity, access control, and network policy all operate at the domain boundary.

### 7. Portable by construction

Same manifests run on GKE, EKS, AKS, bare metal, and local Kind clusters. No Cloud SQL, no RDS, no Pub/Sub. StackGres Postgres is 3-5x cheaper than managed equivalents and runs identically everywhere.

Cloud-native exceptions are limited to object storage (S3/GCS/Azure Blob) and container registries — services with zero ops burden and no portability cost.

---

## Architecture at a glance

```
┌──────────────────────────────────────────────────────────┐
│  chorister CLI  (thin client — CRD CRUD + watch)         │
└───────────────────────┬──────────────────────────────────┘
                        │ kubectl apply (CRDs)
                        ▼
┌──────────────────────────────────────────────────────────┐
│  K8s Cluster                                             │
│                                                          │
│  chorister controller                                    │
│    • Compiles DSL → kro RGDs + K8s manifests             │
│    • Validates & enforces compliance guardrails           │
│    • Reconciles memberships → RoleBindings                │
│    • Reconciles promotions → approval gates               │
│                                                          │
│  kro         → composition (dependency ordering)         │
│  StackGres   → PostgreSQL (Patroni HA, PgBouncer)        │
│  NATS        → queues (JetStream)                        │
│  Dragonfly   → cache (Redis-compatible)                  │
│  Cilium      → networking (CNI + Gateway API + Hubble)   │
│  Grafana LGTM → observability (Mimir + Loki + Tempo)    │
│  OPA         → policy enforcement                        │
│  cert-manager → TLS                                      │
└──────────────────────────────────────────────────────────┘
```

---

## Quick start

```bash
# Bootstrap the controller (installs CRDs, controller bootstraps the rest)
chorister setup

# Authenticate
chorister login

# Create a sandbox
chorister sandbox create --domain payments --name alice

# Apply your DSL to a sandbox
chorister apply --domain payments --sandbox alice

# Check status (real-time, streaming)
chorister status payments

# Compare sandbox to production
chorister diff payments --sandbox alice

# Promote to production (requires approval)
chorister promote payments --sandbox alice
```

---

## Documentation

- [Architecture Decisions](ARCHITECTURE_DECISIONS.md) — canonical command surface, safety model, target architecture, CRD schemas, component stack
- [Security & Compliance Framework Mapping](SECURITY_COMPLIANCE.md) — canonical compliance profile mappings across CIS Controls, SOC 2, ISO 27001, and CIS K8s Benchmark

---

## Status

chorister is in early development. See [ROADMAP.md](ROADMAP.md) for the implementation plan.
