# Copilot Instructions

## Documentation governance

- Treat `ARCHITECTURE_DECISIONS.md` as the canonical source for command syntax, production safety guarantees, and target architecture.
- Treat `SECURITY_COMPLIANCE.md` as the canonical source for compliance profile mappings and framework coverage.
- Keep `README.md` as a high-level summary only. Do not introduce stronger guarantees, alternate command verbs, or alternate compliance mappings there.
- Treat `ROADMAP.md` as the implementation sequence, not the product contract. If roadmap slices intentionally differ from the end-state architecture, label them clearly as MVP-only or interim.
- Until the input format is formally decided, treat HCL-like DSL snippets in the docs as illustrative pseudo-syntax, not a committed surface.
- When changing command verbs, production safety rules, DSL format decisions, or compliance mappings, update the canonical document first and then sync summaries in the other docs.