# CLI Implementation Checklist

AI agent implementation guide for chorister CLI subcommands.
Each step is scoped for a single AI session, produces testable code, and builds on previous steps.

**Source of truth:** `ARCHITECTURE_DECISIONS.md`

---

## Design Principle: Separation of Concerns

All data retrieval and aggregation logic MUST live in reusable `internal/` packages — **not** in `cmd/chorister/`.
The CLI is a thin presentation layer. A future web UI will import the same packages.

### Package Layout

```
internal/
  query/          # Read-only data retrieval from K8s API (shared by CLI + web UI)
    apps.go       # ChoApplication queries
    domains.go    # Domain + namespace queries
    resources.go  # ChoCompute, ChoDatabase, ChoQueue, ChoCache, ChoStorage queries
    sandboxes.go  # ChoSandbox queries
    memberships.go # ChoDomainMembership queries
    promotions.go # ChoPromotionRequest queries
    cluster.go    # ChoCluster queries
    vulns.go      # ChoVulnerabilityReport queries
    logs.go       # Pod log streaming
    events.go     # K8s Event + chorister audit event queries
  report/         # Aggregation and formatting (shared by CLI + web UI)
    compliance.go # Compliance posture aggregation
    finops.go     # Cost estimation, budget tracking
    health.go     # Domain/app health summary
    capacity.go   # Quota utilization aggregation
```

Each `internal/query/` function takes a `client.Client` (or `kubernetes.Interface` for logs) and returns typed Go structs.
Each `internal/report/` function takes query results and returns structured report objects.
CLI commands call query → report → format-for-terminal. Web UI calls query → report → JSON.

---

## Testing Strategy

- **Unit tests** for `internal/query/` and `internal/report/`: use fake `client.Client` from controller-runtime (`fake.NewClientBuilder()`) — no cluster needed.
- **CLI tests** in `cmd/chorister/main_test.go`: use `executeCmd()` helper for argument parsing, safety rails, output format assertions. Some tests use a fake client injected into the command context.
- **Integration tests** (envtest): end-to-end flow for commands that need a real API server.

---

## Phase CLI-0: Query and Report Foundation

Build the reusable packages first. Every subsequent CLI command depends on these.

- [x] **CLI-0.1 — Create `internal/query/` package with client abstraction**
  - Create `internal/query/query.go` with a `Querier` struct wrapping `client.Client`
  - Constructor: `NewQuerier(client.Client) *Querier`
  - Context-passing pattern: all methods take `context.Context` as first param
  - Error wrapping: all K8s errors wrapped with chorister context (resource kind, name, namespace)
  - **Test:** `internal/query/query_test.go` — construct Querier with fake client, assert no panic
  - **Reuse:** CLI, web UI, and any future API gateway all instantiate Querier with their own client

- [x] **CLI-0.2 — Create `internal/report/` package with output model**
  - Create `internal/report/report.go` with common output types:
    - `TableData` struct: `Headers []string`, `Rows [][]string` (for table rendering)
    - `StatusSummary` struct: `Name string`, `Phase string`, `Conditions []ConditionSummary`, `Details map[string]string`
    - `HealthRollup` struct: `Healthy int`, `Degraded int`, `Unknown int`, `Items []StatusSummary`
  - Helper: `FormatAge(time.Time) string` — "2d", "3h", "5m" human-readable age
  - Helper: `FormatCost(string) string` — "$12.50/mo" formatting
  - **Test:** `internal/report/report_test.go` — unit tests for FormatAge edge cases (zero time, future time), FormatCost formatting
  - **Reuse:** CLI renders TableData as ASCII table; web UI renders as JSON/HTML table

- [x] **CLI-0.3 — CLI output framework: `--output` flag support**
  - Add a shared `addOutputFlag(cmd)` helper in `cmd/chorister/output.go`
  - Supports `--output table` (default), `--output json`, `--output yaml`
  - `renderOutput(cmd, data interface{}, tableData *report.TableData)` dispatches to formatter
  - Table formatter: simple ASCII columns with aligned padding
  - JSON/YAML formatter: marshal the structured data directly
  - **Test:** `cmd/chorister/output_test.go`
    - `TestOutputTable` — renders TableData as aligned ASCII columns
    - `TestOutputJSON` — renders struct as valid JSON
    - `TestOutputYAML` — renders struct as valid YAML
  - **Reuse:** Every list/get/status command uses this. Web UI skips this layer entirely.

---

## Phase CLI-1: Application & Domain CRUD (Platform Admin)

Foundation for all other commands — you can't query domains or resources without apps existing.

- [x] **CLI-1.1 — `admin app list`**
  - `internal/query/apps.go`: `(q *Querier) ListApplications(ctx) ([]choristerv1alpha1.ChoApplication, error)`
  - `internal/report/health.go`: `AppListReport(apps []ChoApplication) TableData` — columns: NAME, DOMAINS, COMPLIANCE, PHASE, AGE
  - CLI: `admin app list [--output table|json|yaml]`
  - **Test (query):** fake client with 3 ChoApplications → ListApplications returns all 3
  - **Test (report):** AppListReport produces correct columns and row count
  - **Test (CLI):** `executeCmd("admin", "app", "list")` with injected fake client → output contains app names

- [x] **CLI-1.2 — `admin app get <name>`**
  - `internal/query/apps.go`: `(q *Querier) GetApplication(ctx, name) (*ChoApplication, error)`
  - `internal/query/domains.go`: `(q *Querier) ListDomainsByApp(ctx, appName) ([]DomainInfo, error)` — returns domain name, namespace, status, resource counts
  - `internal/report/health.go`: `AppDetailReport(app *ChoApplication, domains []DomainInfo) StatusSummary`
  - CLI: `admin app get <name> [--output table|json|yaml]`
  - Shows: policy (compliance, HA, promotion), domain list with status, conditions, quotas
  - **Test (query):** fake client with app + 2 domain namespaces → GetApplication returns app; ListDomainsByApp returns domain info
  - **Test (report):** AppDetailReport includes all policy fields, domain count
  - **Test (CLI):** `executeCmd("admin", "app", "get", "myproduct")` → output contains policy and domain info

- [x] **CLI-1.3 — `admin app delete <name>`**
  - CLI: `admin app delete <name> [--confirm] [--dry-run]`
  - Safety: requires `--confirm` flag or interactive confirmation (print what will be deleted first)
  - Dry-run: list all namespaces and resources that would be deleted
  - Uses `(q *Querier) GetApplication()` + `(q *Querier) ListDomainsByApp()` to enumerate impact
  - Actual deletion: delete the ChoApplication CRD (controller handles cascade via owner refs)
  - **Test (CLI):** `executeCmd("admin", "app", "delete", "myproduct")` without `--confirm` → error asking for confirmation
  - **Test (CLI):** `executeCmd("admin", "app", "delete", "myproduct", "--dry-run")` → prints resources that would be deleted
  - **Test (CLI):** `executeCmd("admin", "app", "delete", "myproduct", "--confirm")` with fake client → deletes the CRD

- [x] **CLI-1.4 — `admin domain list`**
  - `internal/query/domains.go`: `(q *Querier) ListAllDomains(ctx, appFilter string) ([]DomainInfo, error)`
  - `DomainInfo` struct: Name, Application, Namespace, Sensitivity, Phase, ResourceCount, Isolated bool
  - `internal/report/health.go`: `DomainListReport(domains []DomainInfo) TableData` — columns: DOMAIN, APPLICATION, NAMESPACE, SENSITIVITY, PHASE, RESOURCES, ISOLATED
  - CLI: `admin domain list [--app <app>] [--output table|json|yaml]`
  - **Test (query):** fake client with 2 apps, 3 domains each → ListAllDomains("") returns 6; ListAllDomains("app1") returns 3
  - **Test (report):** DomainListReport renders correct columns; isolated domains show ⚠ marker
  - **Test (CLI):** `executeCmd("admin", "domain", "list", "--app", "myproduct")` → lists filtered domains

- [x] **CLI-1.5 — `admin domain get <name>`**
  - `internal/query/resources.go`: `(q *Querier) ListDomainResources(ctx, namespace) (*DomainResources, error)`
  - `DomainResources` struct: Computes, Databases, Queues, Caches, Storages, Networks (each a typed slice)
  - `internal/report/health.go`: `DomainDetailReport(domain DomainInfo, resources *DomainResources) StatusSummary`
  - CLI: `admin domain get <name> --app <app> [--output table|json|yaml]`
  - Shows: domain config, all resources with status, namespace, sensitivity, isolation state
  - **Test (query):** fake client with namespace containing 2 ChoCompute + 1 ChoDatabase → ListDomainResources returns them
  - **Test (report):** DomainDetailReport includes resource breakdown
  - **Test (CLI):** output shows resource table with types, names, status

- [x] **CLI-1.6 — `admin domain delete <name>`**
  - CLI: `admin domain delete <name> --app <app> [--confirm] [--dry-run]`
  - Safety: same confirmation pattern as app delete. Lists all resources that would be archived/deleted.
  - Warns about stateful resources (databases, queues, storage) that will enter archive lifecycle in production.
  - Implementation: removes domain from ChoApplication.spec.domains, controller handles cleanup.
  - **Test (CLI):** without `--confirm` → error. With `--dry-run` → prints impact. With `--confirm` → updates ChoApplication.

---

## Phase CLI-2: Cluster Status & Observability (SRE)

SREs need cluster health visibility before anything else works.

- [x] **CLI-2.1 — `admin cluster status`**
  - `internal/query/cluster.go`: `(q *Querier) GetCluster(ctx) (*ChoCluster, error)` — gets the singleton ChoCluster
  - `internal/report/health.go`: `ClusterStatusReport(cluster *ChoCluster) StatusSummary`
  - CLI: `admin cluster status [--output table|json|yaml]`
  - Shows: controller revision, operator status (each operator name + installed/degraded/missing), CIS benchmark result, observability ready flag, conditions
  - **Test (query):** fake ChoCluster with mixed operator statuses → GetCluster returns it
  - **Test (report):** ClusterStatusReport shows per-operator rows, highlights degraded operators
  - **Test (CLI):** `executeCmd("admin", "cluster", "status")` → table contains operator names and statuses

- [x] **CLI-2.2 — `admin cluster operators`**
  - `internal/query/cluster.go`: `(q *Querier) GetOperatorDetails(ctx) ([]OperatorInfo, error)`
  - `OperatorInfo` struct: Name, Version, Status, Namespace, PodCount, ReadyCount
  - CLI: `admin cluster operators [--output table|json|yaml]`
  - Shows: detailed operator view — version, pod count, health per operator
  - **Test (query):** fake ChoCluster with operator versions + status → returns OperatorInfo list
  - **Test (CLI):** output table has VERSION and PODS columns

---

## Phase CLI-3: Enhanced `status` and Developer Observability

These are the most-used commands for day-to-day development.

- [x] **CLI-3.1 — Enhanced `status` command**
  - `internal/query/apps.go`: `(q *Querier) ListApplications(ctx)` (from CLI-1.1)
  - `internal/query/domains.go`: `(q *Querier) ListDomainsByApp(ctx, appName)` (from CLI-1.2)
  - `internal/query/sandboxes.go`: `(q *Querier) ListSandboxesByDomain(ctx, appName, domainName) ([]SandboxInfo, error)`
  - `SandboxInfo` struct: Name, Owner, Namespace, Phase, Age, LastApplyTime, EstimatedMonthlyCost, IdleWarning bool
  - `internal/report/health.go`: `DomainStatusReport(domain DomainInfo, prodResources *DomainResources, sandboxes []SandboxInfo) StatusSummary`
  - Enhance existing `status [domain]` command:
    - No args: list all apps → all domains → summary health per domain
    - With domain: show production status + sandbox list + degraded/isolated indicators
  - Add `--app` flag (required if multiple apps exist)
  - **Test (query):** fake client with domain + 2 sandboxes → ListSandboxesByDomain returns both
  - **Test (report):** DomainStatusReport includes production health + sandbox table
  - **Test (CLI):** `executeCmd("status", "--app", "myproduct")` → shows all domains with health. `executeCmd("status", "payments", "--app", "myproduct")` → shows domain detail + sandboxes.

- [ ] **CLI-3.2 — `logs` command** *(stub: `logs.go` not created; streaming path prints "not yet implemented")*
  - `internal/query/logs.go`:
    - `(q *Querier) ResolvePodsByComponent(ctx, namespace, component string) ([]corev1.Pod, error)` — finds pods by chorister labels
    - `StreamLogs(ctx, clientset kubernetes.Interface, namespace, podName, container string, follow bool, tail int64, writer io.Writer) error`
  - CLI: `logs [component] --domain <domain> --sandbox <sandbox> [--app <app>] [--follow] [--tail N] [--previous] [--container <name>]`
  - Resolves chorister namespace from `--app` + `--domain` + `--sandbox`
  - Resolves pods from component name (matches `chorister.dev/component` label on Deployment/Job)
  - If multiple pods, selects first ready pod (or shows picker)
  - Without component arg: lists available components in the namespace
  - **Test (query):** `ResolvePodsByComponent` with fake client + labeled pods → returns matching pods
  - **Test (CLI):** `executeCmd("logs", "--domain", "payments", "--sandbox", "alice")` without component → lists available components
  - **Test (CLI):** without `--sandbox` → error (logs always target sandbox; use kubectl for prod)

- [x] **CLI-3.3 — `sandbox status` subcommand**
  - `internal/query/sandboxes.go`: `(q *Querier) GetSandbox(ctx, appName, domainName, sandboxName) (*SandboxDetail, error)`
  - `SandboxDetail` struct: SandboxInfo + Resources *DomainResources + Conditions []metav1.Condition
  - `internal/report/health.go`: `SandboxDetailReport(detail *SandboxDetail) StatusSummary`
  - CLI: `sandbox status --domain <domain> --name <name> [--app <app>] [--output table|json|yaml]`
  - Shows: owner, age, cost, last-apply time, idle warning, resource breakdown, conditions
  - **Test (query):** fake ChoSandbox + resources → GetSandbox returns detail
  - **Test (report):** SandboxDetailReport includes cost, age, idle warning when stale
  - **Test (CLI):** `executeCmd("sandbox", "status", "--domain", "payments", "--name", "alice")` → shows detail

- [x] **CLI-3.4 — Enhanced `sandbox list` with cost/age columns**
  - Uses `(q *Querier) ListSandboxesByDomain()` from CLI-3.1
  - `internal/report/health.go`: `SandboxListReport(sandboxes []SandboxInfo) TableData` — columns: NAME, OWNER, DOMAIN, AGE, LAST-APPLY, COST/MO, IDLE
  - Update existing `sandbox list` command to use query/report pipeline
  - Add `--app` flag
  - **Test (report):** SandboxListReport marks idle sandboxes, formats cost
  - **Test (CLI):** `executeCmd("sandbox", "list", "--domain", "payments")` → table with cost and age columns

- [x] **CLI-3.5 — `events` command**
  - `internal/query/events.go`:
    - `(q *Querier) ListChoristerEvents(ctx, namespace string, since time.Duration, limit int) ([]EventInfo, error)`
    - Queries K8s Events with `fieldSelector` for chorister-related events (reason prefix or involved object)
  - `EventInfo` struct: Time, Type (Normal/Warning), Reason, Message, InvolvedObject (kind+name)
  - `internal/report/health.go`: `EventListReport(events []EventInfo) TableData` — columns: TIME, TYPE, REASON, OBJECT, MESSAGE
  - CLI: `events [--domain <domain>] [--sandbox <sandbox>] [--app <app>] [--since 1h] [--limit 50]`
  - Defaults: last 1h, limit 100
  - **Test (query):** fake client with Events in namespace → ListChoristerEvents filters correctly
  - **Test (CLI):** `executeCmd("events", "--domain", "payments", "--sandbox", "alice")` → event table

---

## Phase CLI-4: Promotion Flow Polish (Domain Owner)

- [x] **CLI-4.1 — Enhanced `requests` with filtering**
  - `internal/query/promotions.go`:
    - `(q *Querier) ListPromotionRequests(ctx, filters PromotionFilter) ([]PromotionInfo, error)`
    - `PromotionFilter` struct: App, Domain, Status string
  - `PromotionInfo` struct: Name, Domain, RequestedBy, Phase, CreatedAt, ApprovalCount, RequiredApprovals
  - `internal/report/health.go`: `PromotionListReport(promotions []PromotionInfo) TableData` — columns: ID, DOMAIN, REQUESTED-BY, STATUS, APPROVALS, AGE
  - Enhance existing `requests` command with: `--domain`, `--status pending|approved|rejected|all`, `--app`
  - **Test (query):** fake client with 5 PromotionRequests → filter by status=Pending returns 2
  - **Test (CLI):** `executeCmd("requests", "--status", "pending")` → shows only pending requests

- [x] **CLI-4.2 — `promote --rollback` flag**
  - Enhance existing `promote` command with `--rollback` flag
  - `--rollback` creates a ChoPromotionRequest with `.spec.rollback = true` — controller rolls production back to previous compiled state
  - Requires `--domain` (and implicitly the current production state to roll back from)
  - Does NOT require `--sandbox` (rollback is from production's own history)
  - **Test (CLI):** `executeCmd("promote", "--domain", "payments", "--rollback")` → creates rollback PromotionRequest
  - **Test (CLI):** `executeCmd("promote", "--domain", "payments", "--rollback", "--sandbox", "alice")` → error (rollback and sandbox are mutually exclusive)

- [ ] **CLI-4.3 — Enhanced `diff` with `--output` format** *(stub: diff command prints "not yet implemented — awaiting compilation integration")*
  - Add `--output table|json|yaml` to existing diff command (uses CLI-0.3 framework)
  - JSON output: structured diff object with added/removed/changed arrays
  - Table output (default): colored human-readable diff
  - **Test (CLI):** `executeCmd("diff", "--domain", "payments", "--sandbox", "alice", "--output", "json")` → valid JSON

---

## Phase CLI-5: Security & Vulnerability Management

- [x] **CLI-5.1 — `admin vulnerabilities list`**
  - `internal/query/vulns.go`:
    - `(q *Querier) ListVulnerabilityReports(ctx, filters VulnFilter) ([]VulnReportInfo, error)`
    - `VulnFilter` struct: App, Domain, MinSeverity string
  - `VulnReportInfo` struct: Domain, Namespace, Scanner, CriticalCount, HighCount, ScannedAt, Phase
  - `internal/report/compliance.go`: `VulnSummaryReport(reports []VulnReportInfo) TableData` — columns: DOMAIN, CRITICAL, HIGH, SCANNER, LAST-SCAN, STATUS
  - CLI: `admin vulnerabilities list [--app <app>] [--domain <domain>] [--severity critical|high|all] [--output table|json|yaml]`
  - Default: shows all domains, critical+high counts only
  - **Test (query):** fake client with 3 VulnerabilityReports, different severities → filter by domain returns 1
  - **Test (report):** VulnSummaryReport highlights critical>0 rows
  - **Test (CLI):** output table shows per-domain vulnerability summary

- [x] **CLI-5.2 — `admin vulnerabilities get <domain>`**
  - `internal/query/vulns.go`: `(q *Querier) GetVulnerabilityReport(ctx, namespace) (*VulnReportDetail, error)`
  - `VulnReportDetail` struct: VulnReportInfo + Findings []VulnerabilityFinding
  - CLI: `admin vulnerabilities get <domain> [--app <app>] [--output table|json|yaml]`
  - Shows: individual findings — image, CVE ID, severity, package, fix version, title
  - **Test (query):** fake ChoVulnerabilityReport with 5 findings → returns all
  - **Test (CLI):** output table shows per-finding detail

- [ ] **CLI-5.3 — `admin scan`** *(stub: prints fake success message, no actual K8s mutations)*
  - CLI: `admin scan [--domain <domain>] [--app <app>]`
  - Triggers on-demand vulnerability scan by creating/updating CronJob with `kubectl.kubernetes.io/trigger-at` annotation
  - Without `--domain`: scans all domains in the app
  - **Test (CLI):** `executeCmd("admin", "scan", "--domain", "payments", "--app", "myproduct")` → prints confirmation
  - **Test (CLI):** without `--app` → error

---

## Phase CLI-6: Audit & Compliance (Compliance Officer / Security)

- [x] **CLI-6.1 — `admin audit`**
  - `internal/query/events.go`:
    - `(q *Querier) QueryAuditLog(ctx, filters AuditFilter) ([]AuditEntry, error)`
    - `AuditFilter` struct: Domain, Action, Actor, Since time.Duration
    - Implementation: queries Loki via HTTP (`/loki/api/v1/query_range` with LogQL)
    - Fallback: if Loki is unreachable, return error with instructions to check Loki pod
  - `AuditEntry` struct: Timestamp, Actor, Action, Resource, Namespace, Application, Domain, Result, Details
  - `internal/report/compliance.go`: `AuditReport(entries []AuditEntry) TableData` — columns: TIME, ACTOR, ACTION, RESOURCE, DOMAIN, RESULT
  - CLI: `admin audit [--domain <domain>] [--action <action>] [--actor <email>] [--since 24h] [--limit 100] [--output table|json|yaml]`
  - **Test (query):** mock Loki HTTP response → QueryAuditLog parses entries correctly
  - **Test (CLI):** `executeCmd("admin", "audit", "--domain", "payments", "--since", "24h")` → shows audit entries
  - **Reuse:** web UI calls QueryAuditLog directly for the audit dashboard

- [x] **CLI-6.2 — `admin compliance report`**
  - `internal/report/compliance.go`:
    - `ComplianceReport(ctx, q *Querier, app *ChoApplication) (*ComplianceResult, error)`
    - `ComplianceResult` struct: AppName, Profile (essential/standard/regulated), Checks []ComplianceCheck
    - `ComplianceCheck` struct: Framework (CIS Controls/SOC 2/ISO 27001), ControlID, Description, Status (Pass/Fail/NotApplicable), Evidence string
    - Aggregates: Gatekeeper constraint status, kube-bench results, Tetragon status, TLS enforcement status, encryption-at-rest validation
  - CLI: `admin compliance report [--app <app>] [--format table|json|yaml]`
  - Restructure existing `admin compliance` from flat stub to `admin compliance report` subcommand
  - **Test (report):** ComplianceReport with essential profile → checks non-root enforcement, NetworkPolicy default-deny, encrypted storage. Regulated → additional Tetragon check.
  - **Test (CLI):** output includes framework/control/status columns
  - **Reuse:** web UI renders ComplianceResult as a dashboard with pass/fail indicators

- [x] **CLI-6.3 — `admin compliance status`**
  - `internal/report/compliance.go`: `ComplianceStatusSummary(result *ComplianceResult) StatusSummary`
  - CLI: `admin compliance status [--app <app>]`
  - Quick one-liner per app: compliance profile, pass/fail/total counts, worst finding
  - Lighter than `report` — for daily SRE glance
  - **Test (CLI):** output is a single summary line per app

---

## Phase CLI-7: FinOps & Capacity Planning

- [x] **CLI-7.1 — `admin finops report`**
  - `internal/report/finops.go`:
    - `FinOpsReport(ctx, q *Querier, appName string) (*FinOpsResult, error)`
    - `FinOpsResult` struct: AppName, TotalMonthlyCost, Domains []DomainCost, Sandboxes []SandboxCost
    - `DomainCost` struct: Name, Production CostEstimate, SandboxCostTotal
    - `SandboxCost` struct: Name, Domain, Owner, MonthlyCost, Idle bool
  - Queries: ChoCluster for rates, ChoSandbox for per-sandbox cost, domain resources for production estimate
  - CLI: `admin finops report [--app <app>] [--domain <domain>] [--output table|json|yaml]`
  - **Test (report):** fake ChoCluster with rates + 2 sandboxes with costs → FinOpsReport totals correctly
  - **Test (CLI):** output shows cost breakdown by domain and sandbox

- [x] **CLI-7.2 — `admin finops budget`**
  - `internal/report/finops.go`:
    - `BudgetReport(ctx, q *Querier, appName string) (*BudgetResult, error)`
    - `BudgetResult` struct: AppName, DefaultBudget, Domains []DomainBudget
    - `DomainBudget` struct: Name, Budget, CurrentSpend, Utilization float64, AtRisk bool (>80%)
  - CLI: `admin finops budget [--app <app>] [--output table|json|yaml]`
  - Highlights domains at >80% budget utilization
  - **Test (report):** domain at 85% budget → AtRisk=true. Domain at 50% → AtRisk=false.
  - **Test (CLI):** output highlights at-risk domains

- [x] **CLI-7.3 — `admin quotas`**
  - `internal/report/capacity.go`:
    - `QuotaReport(ctx, q *Querier, appName string, domainFilter string) (*QuotaResult, error)`
    - `QuotaResult` struct: Domains []DomainQuota
    - `DomainQuota` struct: Name, Namespace, CPU (used/limit), Memory (used/limit), Storage (used/limit), PodCount (used/limit)
  - Queries: ResourceQuota + resource usage from each namespace
  - CLI: `admin quotas [--app <app>] [--domain <domain>] [--output table|json|yaml]`
  - **Test (report):** fake ResourceQuota with usage → QuotaReport computes utilization percentages
  - **Test (CLI):** output shows per-domain quota utilization with usage bars or percentages

---

## Phase CLI-8: Resource Inspection & Admin Tools

- [x] **CLI-8.1 — `admin resource list`**
  - Uses `(q *Querier) ListDomainResources()` from CLI-1.5
  - `internal/report/health.go`: `ResourceListReport(resources *DomainResources) TableData` — columns: TYPE, NAME, STATUS, LIFECYCLE, SIZE, AGE
  - CLI: `admin resource list [--app <app>] [--domain <domain>] [--type database|compute|queue|cache|storage] [--archived] [--output table|json|yaml]`
  - `--archived` filter: only show resources in Archived/Deletable lifecycle state
  - **Test (CLI):** with `--archived` flag → only shows archived resources
  - **Test (report):** ResourceListReport includes lifecycle column, marks Archived resources distinctly

- [x] **CLI-8.2 — `get <type> <name>` — generic resource inspector**
  - `internal/query/resources.go`: `(q *Querier) GetResource(ctx, kind, name, namespace) (runtime.Object, error)`
  - CLI: `get <type> <name> [--namespace <ns>] [--app <app>] [--domain <domain>] [--output table|json|yaml]`
  - Types: `compute`, `database`, `queue`, `cache`, `storage`, `network`, `sandbox`, `promotion`
  - Displays: full spec + status, conditions, labels, owner references
  - **Test (CLI):** `executeCmd("get", "database", "ledger", "--domain", "payments")` → shows ChoDatabase detail
  - **Test (query):** GetResource with fake client → returns typed object

- [x] **CLI-8.3 — `admin domain set-sensitivity`**
  - CLI: `admin domain set-sensitivity <name> --app <app> --level <public|internal|confidential|restricted>`
  - Validates: sensitivity can only escalate above app compliance baseline, never weaken
  - Updates domain spec in ChoApplication
  - **Test (CLI):** set restricted on essential-profile app → succeeds. Set public on regulated app → error.

---

## Phase CLI-9: Membership Management Polish

- [x] **CLI-9.1 — Enhanced `admin member add` with proper flags**
  - CLI: `admin member add --app <app> --domain <domain> --identity <email> --role <org-admin|domain-admin|developer|viewer> [--expires-at <RFC3339>] [--source manual]`
  - Validates: `restricted` domain or `regulated` app requires `--expires-at`
  - Creates ChoDomainMembership CRD
  - **Test (CLI):** `executeCmd("admin", "member", "add", "--app", "myproduct", "--domain", "payments", "--identity", "alice@co.com", "--role", "developer")` → creates membership
  - **Test (CLI):** restricted domain without `--expires-at` → error

- [x] **CLI-9.2 — Enhanced `admin member list` with filtering**
  - `internal/query/memberships.go`:
    - `(q *Querier) ListMemberships(ctx, filters MemberFilter) ([]MemberInfo, error)`
    - `MemberFilter` struct: App, Domain, Role, IncludeExpired bool
  - `MemberInfo` struct: Identity, Role, Domain, Application, Source, ExpiresAt, Phase, DaysUntilExpiry
  - CLI: `admin member list [--app <app>] [--domain <domain>] [--role <role>] [--include-expired] [--output table|json|yaml]`
  - Columns: IDENTITY, ROLE, DOMAIN, SOURCE, EXPIRES, STATUS
  - **Test (query):** fake client with 5 memberships → filter by role=developer returns 2
  - **Test (CLI):** output shows expiry warnings for memberships expiring within 30 days

- [x] **CLI-9.3 — Enhanced `admin member audit`**
  - `internal/report/compliance.go`:
    - `MembershipAuditReport(ctx, q *Querier, appName string) (*MemberAuditResult, error)`
    - `MemberAuditResult` struct: StaleCount, ExpiredCount, ExpiringCount, Entries []MemberAuditEntry
    - `MemberAuditEntry`: Identity, Issue (expired/stale/no-expiry-on-restricted), Domain, LastSeen
  - CLI: `admin member audit [--app <app>] [--output table|json|yaml]`
  - Flags: stale (no activity), expired, missing expiry on restricted domains
  - **Test (report):** expired membership + restricted-domain membership without expiry → both flagged
  - **Test (CLI):** output includes issue type and recommended action

---

## Phase CLI-10: Automation & CI/CD Support

- [x] **CLI-10.1 — `wait` command**
  - `internal/query/resources.go`: `(q *Querier) WaitForCondition(ctx, kind, name, namespace, conditionType string, timeout time.Duration) error`
  - Uses K8s Watch API with timeout
  - CLI: `wait --for <condition> --type <resource-type> --name <name> [--namespace <ns>] [--timeout 5m]`
  - Conditions: `Ready`, `Completed`, `Approved`, `Failed`
  - Exit code 0 on condition met, 1 on timeout
  - **Test (query):** fake watcher that fires condition after 100ms → WaitForCondition returns nil
  - **Test (query):** fake watcher that never fires → WaitForCondition returns timeout error
  - **Test (CLI):** `executeCmd("wait", "--for", "Ready", "--type", "sandbox", "--name", "alice", "--timeout", "1s")` → returns expected exit

- [ ] **CLI-10.2 — `admin export-config`** *(partial: exports ChoApplication YAML; membership export skipped with TODO)*
  - CLI: `admin export-config --app <app> [--output-dir ./backup]`
  - Exports all ChoApplication + domain configs as chorister CRD YAML (not compiled output — the input CRDs)
  - Use case: backup, migration, disaster recovery
  - **Test (CLI):** fake client with app + domains + memberships → exports to directory → files are valid YAML containing CRD kind/apiVersion

---

## Implementation Notes for AI Sessions

### Session pattern

1. Read this file + `ARCHITECTURE_DECISIONS.md` for context
2. If the task creates a new `internal/query/` or `internal/report/` function, implement that first with unit tests
3. Wire the CLI command in `cmd/chorister/main.go`
4. Add CLI tests in `cmd/chorister/main_test.go`
5. Run `make test` to verify
6. Check off the item

### Dependency graph

```
CLI-0 (foundation)
  └── CLI-1 (app/domain CRUD) ← most other phases depend on these query functions
        ├── CLI-2 (cluster status)
        ├── CLI-3 (status, logs, sandbox, events) ← depends on CLI-1 query functions
        │     └── CLI-4 (promotion polish)
        ├── CLI-5 (vulnerabilities) ← depends on CLI-1 for namespace resolution
        ├── CLI-6 (audit, compliance) ← depends on CLI-1 + CLI-5
        ├── CLI-7 (finops, quotas) ← depends on CLI-1 + CLI-3
        ├── CLI-8 (resource inspection) ← depends on CLI-1
        ├── CLI-9 (membership polish) ← independent of most, but benefits from CLI-1
        └── CLI-10 (automation) ← depends on CLI-1 query patterns
```

### Client injection for testing

CLI commands that need a K8s client should accept it via cobra command context:
```go
// Set client in context (for testing)
ctx := context.WithValue(cmd.Context(), clientKey, fakeClient)

// Get client in command (prod path creates real client from kubeconfig)
func getClient(cmd *cobra.Command) (client.Client, error) {
    if c, ok := cmd.Context().Value(clientKey).(client.Client); ok {
        return c, nil
    }
    // Build real client from kubeconfig
    ...
}
```

This pattern enables both unit testing with fake clients and real cluster usage.
