# Security & Compliance Framework Mapping

How chorister maps to CIS Critical Security Controls (v8.1), SOC 2 Trust Services Criteria, ISO/IEC 27001:2022 Annex A, and CIS Kubernetes Benchmark — and what the `compliance` config in `ChoApplication` activates.

This document is the canonical source for compliance profile mappings in the repo.

---

## Table of Contents

1. [Frameworks at a Glance](#1-frameworks-at-a-glance)
2. [Compliance Model in ChoApplication](#2-compliance-model-in-choapplication)
3. [What Each Setting Activates](#3-what-each-setting-activates)
4. [CIS Controls v8.1 — Coverage Summary](#4-cis-controls-v81--coverage-summary)
5. [SOC 2 — Coverage Summary](#5-soc-2--coverage-summary)
6. [ISO 27001 — Coverage Summary](#6-iso-27001--coverage-summary)
7. [CIS Kubernetes Benchmark](#7-cis-kubernetes-benchmark)
8. [Cross-Framework Control Mapping](#8-cross-framework-control-mapping)
9. [Gap Analysis](#9-gap-analysis)
10. [Compliance Profiles](#10-compliance-profiles)

---

## 1. Frameworks at a Glance

Four frameworks, each at a different layer:

| Framework | What it answers | Scope |
|---|---|---|
| **CIS Controls v8.1** | What security capabilities does your org need? | 18 controls, 153 safeguards, 3 IGs (IG1→IG2→IG3) |
| **CIS K8s Benchmark** | How do you harden K8s specifically? | Cluster config: API server, kubelet, RBAC, pod security |
| **SOC 2** | Can you prove your security works? | 5 Trust Services Categories (Security mandatory, rest optional) |
| **ISO 27001** | Do you govern infosec as a management system? | Annex A 2022: 4 control themes, 93 controls |

They are not alternatives — they layer:
- **CIS Controls** = strategic (what capabilities)
- **CIS K8s Benchmark** = tactical (how to harden K8s, implements parts of CIS Controls)
- **SOC 2** = evidence (prove it to customers)
- **ISO 27001** = governance (manage it across the org)

---

## 2. Compliance Model in ChoApplication

The original `tier: cis-level-1` / `soc2: false` was wrong because CIS Controls use IGs, not "levels", SOC 2 is an audit framework, and they're not alternatives. The next revision exposed per-framework knobs (`cisControls.ig`, `cisKubernetes.profile`, `soc2.enabled`, `iso27001.enabled`) — accurate, but wrong for a different reason: it forced users to be compliance experts.

An opinionated platform should not ask users to compose framework selections. Users know their business context ("I'm running internal tools" vs "I'm running a regulated bank"), not which CIS Implementation Group they need. The framework mapping is **our** job.

The final model is a single field — pick one:

```yaml
apiVersion: chorister.dev/v1alpha1
kind: ChoApplication
metadata:
  name: retail-banking
spec:
  policy:
    compliance: regulated   # essential | standard | regulated
```

| Profile | When to use | What it maps to internally |
|---|---|---|
| `essential` | Internal tools, dev environments, non-customer-facing | CIS Controls IG1, CIS K8s Level 1 |
| `standard` | Production SaaS, customer-facing services | CIS Controls IG2, CIS K8s Level 1, SOC 2 (Security + Availability) |
| `regulated` | Banking, healthcare, government, sensitive data | CIS Controls IG3, CIS K8s Level 2, SOC 2 (Security + Availability + Confidentiality), ISO 27001 |

The framework details in Sections 4–8 remain the internal reference for how the controller implements each profile. Users never see them.

---

## 3. What Each Profile Activates

### `essential`

CRD asset inventory, OPA software allowlist, pod security standards (no privileged, no hostPID/IPC/Network, drop all caps, non-root), RBAC lifecycle, OIDC + namespace isolation, audit logs (Loki), automated backups (StackGres), Hubble network monitoring, encryption at rest (always on), deny-all NetworkPolicy, kube-bench Level 1 validation, promotion approval (always), egress health for high-criticality providers, promotion freeze on degradation.

**Admission enforcement:** Validating webhooks for `ChoApplication` and `ChoNetwork` enforce policy at creation/update time. The webhooks call `internal/validation/` functions including ingress auth requirements, egress allowlist validation, compliance escalation checks, and consumes/supplies consistency. Invalid resources are rejected before they enter the cluster.

### `standard`

Everything in `essential`, plus: image scanning gate before promotion (Trivy/Grype), continuous vuln scanning CronJobs, centralized log alerting, Hubble network anomaly detection, access review automation (flag stale memberships), OIDC group sync, config change alerting, weekly kube-bench re-scans, HA required for databases (`ha: true`), PodDisruptionBudget + min replicas > 1, SOC 2 evidence fields in audit events, structured change management trail.

### `regulated`

Everything in `standard`, plus: full Tetragon runtime detection (syscall, file integrity), seccomp RuntimeDefault, encrypted etcd validation, extended audit policy, AppArmor/SELinux, ChoDomainMembership expiry required, 2+ approvers for promotion, ticket reference mandatory, monthly compliance reports, image age warnings (>30d) + block on unpatched critical CVEs, `chorister admin isolate/unisolate` for incident response, TLS for all cross-domain traffic, data sensitivity required in DSL, access review cadence enforcement, Level 2 access policy for `restricted` domains.

### Internal framework mapping reference

| Profile | CIS Controls | CIS K8s Benchmark | SOC 2 | ISO 27001 |
|---|---|---|---|---|
| `essential` | IG1 | Level 1 | — | — |
| `standard` | IG2 | Level 1 | Security, Availability | — |
| `regulated` | IG3 | Level 2 | Security, Availability, Confidentiality | Enabled |

---

## 4. CIS Controls v8.1 — Coverage Summary

### Per-control coverage

| Control | Focus | ✅ Full | 🟡 Partial | ⬜ N/A or Org |
|---|---|---|---|---|
| 1. Enterprise Asset Inventory | Know what you have | 3 | 1 | 1 |
| 2. Software Asset Inventory | Authorized software | 4 | 2 | 1 |
| 3. Data Protection | Encrypt, classify, control data | 4 | 7 | 3 |
| 4. Secure Configuration | Harden configs | 7 | 1 | 4 |
| 5. Account Management | Manage user/admin accounts | 5 | 1 | 0 |
| 6. Access Control | Least privilege, MFA | 8 | 0 | 0 |
| 7. Vulnerability Management | Find and fix vulns | 2 | 5 | 0 |
| 8. Audit Log Management | Collect, retain, analyze logs | 8 | 3 | 1 |
| 9. Email/Browser | Endpoint protections | 0 | 0 | all |
| 10. Malware Defenses | Anti-malware | 1 | 4 | 2 |
| 11. Data Recovery | Backups and recovery | 3 | 1 | 1 |
| 12. Network Infrastructure | Manage network securely | 6 | 1 | 1 |
| 13. Network Monitoring | Monitor and defend network | 7 | 3 | 1 |
| 14. Security Training | Train people | 0 | 0 | all |
| 15. Service Provider Mgmt | 3rd-party risk | 0 | 4 | 3 |
| 16. Application Security | Secure dev lifecycle | 4 | 4 | 6 |
| 17. Incident Response | Detect, respond, recover | 0 | 6 | 3 |
| 18. Penetration Testing | Simulate attacks | 0 | 2 | 3 |

### By Implementation Group

| IG | Total Safeguards | ✅ Full | 🟡 Partial | ⬜ Out of Scope | Coverage (Full + Partial) |
|---|---|---|---|---|---|
| IG1 | 56 | ~32 (57%) | ~14 (25%) | ~10 (18%) | **82%** |
| IG2 | 74 additional | ~22 (30%) | ~25 (34%) | ~27 (36%) | **64%** |
| IG3 | 23 additional | ~3 (13%) | ~10 (43%) | ~10 (43%) | **57%** |
| **All** | **153** | **~57 (37%)** | **~49 (32%)** | **~47 (31%)** | **69%** |

chorister is strongest at IG1 (82%) because IG1 focuses on asset inventory, access control, secure configuration, audit logging, and network management — all areas where a K8s operator excels. Coverage drops at IG2/IG3 as controls shift to organizational concerns (training, incident response processes, vendor management, pentesting).

### Key chorister mechanisms per CIS Control

| CIS Control | How chorister covers it |
|---|---|
| 1 (Asset inventory) | All infra = K8s CRDs with `chorister.dev/*` labels. OPA blocks unauthorized resources. |
| 2 (Software inventory) | Container images declared in DSL. OPA image allowlist. Controller tracks versions in CRD status. |
| 3 (Data protection) | NetworkPolicy + RBAC = access control. TLS mandatory for external. Encryption at rest always on. `consumes/supplies` = machine-readable data flow docs. |
| 4 (Secure config) | All config in DSL → compiled with guardrails. OPA admission. kube-bench validation. |
| 5 (Account mgmt) | ChoDomainMembership = authoritative account inventory. OIDC (no passwords). 4-tier RBAC. OIDC group sync. |
| 6 (Access control) | `chorister admin member add/remove`. OIDC MFA via IdP. 4 roles with K8s RBAC enforcement. |
| 7 (Vuln mgmt) | Image scanning before promotion. Continuous re-scanning (IG2+). Promotion blocked on critical CVEs. |
| 8 (Audit logs) | Dual logs: chorister intent (Loki, synchronous) + K8s audit. Object storage retention. Structured JSON events. |
| 11 (Recovery) | StackGres automated backups + WAL archiving to object storage. LGTM on object storage. |
| 12 (Network infra) | ChoCluster reconciliation keeps stack current. DSL = the architecture. Hubble = live topology. |
| 13 (Network monitoring) | Hubble: flow monitoring, anomaly detection. CiliumNetworkPolicy: L3/L4/L7 filtering. Grafana alerting. |

---

## 5. SOC 2 — Coverage Summary

| CC Series | Focus | ✅ Full | 🟡 Partial | ⬜ Org |
|---|---|---|---|---|
| CC1 Control Environment | Governance, accountability | 0 | 2 | 3 |
| CC2 Communication | Security info sharing | 0 | 2 | 1 |
| CC3 Risk Assessment | Risk identification | 1 | 2 | 1 |
| CC4 Monitoring | Evaluate controls | 2 | 0 | 0 |
| CC5 Control Activities | Policies & procedures | 3 | 0 | 0 |
| CC6 Logical/Physical Access | AuthN, AuthZ, encryption | 4 | 2 | 2 |
| CC7 System Operations | Detect anomalies, respond | 2 | 2 | 1 |
| CC8 Change Management | Change control | 1 | 0 | 0 |
| CC9 Risk Mitigation | Vendor, disruption | 0 | 1 | 1 |
| **Security total** | | **13 (39%)** | **11 (33%)** | **9 (27%)** |
| A1 Availability | Uptime, DR | 1 | 1 | 1 |
| C1 Confidentiality | Protect confidential data | 0 | 2 | 0 |

### Key chorister mechanisms per SOC 2 CC

| SOC 2 area | How chorister covers it |
|---|---|
| CC5 (Control activities) | OPA constraints, compile-time guardrails, NetworkPolicy — all automatic, no human action. |
| CC6 (Access) | OIDC + ChoDomainMembership + RoleBindings. Deny-all network default. Egress allowlist. |
| CC7 (Operations) | Grafana LGTM monitoring. Controller re-validates on every reconciliation. OPA drift detection. |
| CC8 (Change mgmt) | Sandbox → verify → diff → promote with approval. No direct prod writes. Full audit trail. |

chorister covers 39% of SOC 2 Security controls fully and provides evidence for 33%. The remaining 27% are organizational/physical controls no infrastructure platform can address.

---

## 6. ISO 27001 — Coverage Summary

ISO/IEC 27001:2022 uses the revised Annex A structure from ISO/IEC 27002:2022: 93 controls grouped into four themes. The table below uses that current model rather than the retired 2013 14-domain layout.

| Annex A Theme | ✅ Full | 🟡 Partial | ⬜ Out of Scope |
|---|---|---|---|
| A.5 Organizational controls (37) | 6 | 10 | 21 |
| A.6 People controls (8) | 0 | 1 | 7 |
| A.7 Physical controls (14) | 0 | 0 | 14 |
| A.8 Technological controls (34) | 20 | 9 | 5 |
| **Total (93)** | **26 (28%)** | **20 (22%)** | **47 (51%)** |

### Strongest coverage areas

| ISO 27001 theme | Why chorister excels |
|---|---|
| A.8 Technological controls (20/34 full) | OIDC + ChoDomainMembership + RBAC + NetworkPolicy + logging + vulnerability controls map strongly to technical safeguards |
| A.5 Organizational controls (6/37 full) | Approval workflows, change records, access reviews, and policy-driven guardrails provide reusable governance evidence, even when wider org process is still required |

More than half of Annex A remains partially covered or out of scope because ISO 27001 is a management-system standard. Areas like people, physical security, supplier governance, and legal process still require company controls outside the platform.

---

## 7. CIS Kubernetes Benchmark

Separate from CIS Controls. Technology-specific hardening for K8s clusters.

| Category | chorister role | How |
|---|---|---|
| Control Plane (1.x, 2.x) | **Detect only** | kube-bench as Job during setup + weekly CronJob. Cannot modify managed control planes (GKE/AKS/EKS). |
| Worker Nodes (4.x) | **Detect only** | kube-bench scans. Node config is provider responsibility. |
| Policies (5.x) | **Enforce** | OPA: no privileged/hostPID/hostIPC/hostNetwork, drop caps, non-root, seccomp. RBAC: no cluster-admin for devs, own ServiceAccounts. Network: deny-all + explicit allow. |

| Level | What it adds | Impact |
|---|---|---|
| Level 1 | Production-ready hardening, minimal operational impact | Pod security, RBAC lockdown, namespace isolation, deny-all NetworkPolicy, audit logging |
| Level 2 | Defense-in-depth for sensitive environments | seccomp RuntimeDefault, encrypted etcd, extended audit policy, AppArmor/SELinux |

---

## 8. Cross-Framework Control Mapping

Where frameworks overlap, chorister satisfies multiple requirements with a single mechanism:

| chorister Feature | CIS Controls | CIS K8s | SOC 2 | ISO 27001 |
|---|---|---|---|---|
| **ChoDomainMembership → RoleBindings** | 5.1, 5.4, 6.1–6.2, 6.8 | 5.1.1 | CC6.1, CC6.2 | A.9.1–2, A.6.1.2 |
| **OIDC authentication** | 6.3–6.5, 5.2 | — | CC6.1 | A.9.3.1, A.9.4.2 |
| **deny-all NetworkPolicy + consumes/supplies** | 12.2, 13.4, 13.9 | 5.3.2 | CC6.6, CC6.7 | A.13.1.1–3 |
| **Sandbox → promote → approve** | 16.8 | — | CC8.1 | A.12.1.2, A.14.2.2 |
| **Dual audit logs (Loki + K8s)** | 8.1–8.5, 8.9, 8.10 | — | CC4.1, CC7.1 | A.12.4.1–3, A.16.1.7 |
| **OPA/Gatekeeper constraints** | 4.1, 2.5 | 5.2.1–9 | CC5.1–3 | A.14.1.1 |
| **cert-manager TLS** | 3.10 | — | CC6.7 | A.10.1.1, A.14.1.2 |
| **JWT ingress requirement** | 6.3, 13.10 | — | CC6.6 | A.9.1.2, A.14.1.2–3 |
| **Egress allowlist** | 3.3, 12.2 | — | CC6.7 | A.13.2.1 |
| **Grafana LGTM** | 8.2, 13.1, 13.6 | — | CC4.1, CC7.2 | A.12.4.1 |
| **StackGres backups** | 11.1–11.3 | — | A1.2, CC9.1 | A.12.3.1, A.17.2.1 |
| **Image scanning gates** | 7.5, 7.7, 10.1 | — | CC6.8 | A.12.2.1, A.14.2.8 |
| **CRD-based asset inventory** | 1.1, 2.1, 5.1 | — | CC3.1 | A.8.1.1–2 |
| **consumes/supplies data flow** | 3.8, 12.4 | — | — | A.13.2.1 |
| **Hubble network monitoring** | 13.3, 13.6 | — | CC7.2 | A.13.1.1 |

---

## 9. Gap Analysis

### Strengths

1. **Asset inventory & access control** (CIS 1, 2, 5, 6) — CRD-based, OIDC + RBAC + RoleBindings, MFA via IdP
2. **Network security** (CIS 12, 13) — deny-all default, consumes/supplies, egress allowlist, gateway cross-app, L7 filtering
3. **Audit logging** (CIS 8) — tamper-proof dual logs, centralized in Loki, object storage retention
4. **Change management** (CIS 16.8, SOC 2 CC8, ISO A.12/A.14) — sandbox → diff → promote → approve
5. **Secure configuration** (CIS 4) — OPA constraints, controller-managed stack
6. **Data recovery** (CIS 11) — StackGres automated backups, LGTM on object storage

### Gaps and status

| Gap | Frameworks | Severity | Status | Resolution |
|---|---|---|---|---|
| No automated access deprovisioning | CIS 5.3, SOC 2 CC6.3/CC6.5, ISO A.9.2.5 | High | ✅ Done | OIDC group sync → ChoDomainMembership lifecycle. TTL/expiry field. `chorister admin member audit`. |
| No runtime threat detection | CIS 10.7, 13.2, SOC 2 CC6.8, ISO A.12.2.1 | Medium | ✅ Done | Tetragon (same eBPF stack as Cilium). Off at IG1, network anomaly at IG2, full syscall at IG3. `restricted` domains auto-enable. |
| Vuln scanning is promotion-only | CIS 7.1–7.4 | Medium | ✅ Done | Controller generates CronJob per domain for periodic re-scanning. ChoVulnerabilityReport CRDs. Grafana alerting on new CVEs. Image age warnings at IG3. |
| No encryption at rest by default | CIS 3.11, ISO A.10.1.1, SOC 2 C1.1 | Medium | ✅ Done | Always on. Controller selects encrypted StorageClass. `chorister setup` validates. No opt-out. |
| No incident response workflow | CIS 17, SOC 2 CC7.3–5, ISO A.16 | Medium | 🟡 Partial | Service health baseline: detect degradation → flag status → freeze promotions → `isolate/unisolate` → rollback. Full incident lifecycle (breach forensics, customer comms, on-call) remains PagerDuty/Opsgenie. |
| No data classification in DSL | CIS 3.7, ISO A.8.2.1 | Low | ✅ Done | `sensitivity` field at domain level: public/internal/confidential/restricted. Controller enforces escalating protections. |
| No service provider monitoring | CIS 15.6 | Low | ✅ Done | Egress allowlist enriched with `criticality`/`expectedLatency`/`alertOnErrorRate`. Hubble metrics → Grafana alerting. `status.egressHealth`. |
| No penetration testing support | CIS 18 | Low | ⬜ Open | IG3 only. Organizational activity. Could track pentest cadence in CRDs. |

---

## 10. Compliance Profiles

Users pick one word. The controller handles the rest.

```yaml
# Internal tools
policy:
  compliance: essential

# Production SaaS
policy:
  compliance: standard

# Banking, healthcare, government
policy:
  compliance: regulated
```

See [Section 3](#3-what-each-profile-activates) for exactly what each profile enforces, and the internal framework mapping table for how profiles map to CIS Controls, CIS K8s Benchmark, SOC 2, and ISO 27001.

---

## Appendix: Framework References

| Framework | Source |
|---|---|
| CIS Critical Security Controls v8.1 | https://www.cisecurity.org/controls/cis-controls-list |
| CIS Controls Implementation Groups | https://www.cisecurity.org/controls/implementation-groups |
| CIS Kubernetes Benchmark v2.0 | https://www.cisecurity.org/benchmark/kubernetes |
| SOC 2 Trust Services Criteria (2017) | AICPA TSP Section 100 |
| ISO/IEC 27001:2022 | https://www.iso.org/standard/27001 |
| kube-bench | https://github.com/aquasecurity/kube-bench |
| OPA/Gatekeeper | https://open-policy-agent.github.io/gatekeeper/ |
