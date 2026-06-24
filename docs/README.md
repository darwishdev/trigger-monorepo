# Trigger Documentation

The `docs` directory is intentionally flat. File names identify each document’s role.

## Documents

| File | Type | Purpose |
|------|------|---------|
| [product-overview.md](product-overview.md) | Product | Product goals, users, capabilities, and high-level technical direction |
| [architecture-reference.md](architecture-reference.md) | Reference | Permanent architecture, boundaries, dependencies, and implementation patterns |
| [security-invariants.md](security-invariants.md) | Reference | Mandatory security rules shared by every plan |
| [infrastructure-plan.md](infrastructure-plan.md) | Plan | Shared database, migrations, sqlc, Redis, encryption, HTTP security, and jobs |
| [identity-auth-plan.md](identity-auth-plan.md) | Plan | WorkOS identity, sessions, authorization, CRM credentials, and settings |
| [calls-feature-plan.md](calls-feature-plan.md) | Plan | Recording ingestion, call records, matching, extraction, and CRM completion |
| [odoo-crm-status.md](odoo-crm-status.md) | Status | Implemented and remaining Odoo integration work |

## Implementation order

```text
infrastructure-plan.md
        ↓
identity-auth-plan.md
        ↓
calls-feature-plan.md
```

The Odoo integration is partially implemented already. Remaining auth-aware resolution is
completed through the identity plan before the calls plan consumes it.

## Document rules

- Product documents describe intended behavior, not implementation steps.
- Architecture documents describe durable rules, not branch work.
- Security invariants apply to all plans and are not duplicated in full.
- Plans contain ordered, verifiable implementation work.
- Status documents report what exists and link to plans for remaining work.
- When implementation changes architecture, update the architecture reference in the same
  branch.
- When a plan is completed, update affected status documents instead of leaving stale
  “remaining” sections.
