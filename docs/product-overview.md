# Trigger AI — Product Overview

## Purpose

Trigger is an AI-powered sales assistant that turns customer calls into operational CRM
work. Its principle is “actions over summaries”: recordings and transcripts should produce
follow-ups, CRM completion, coaching signals, and reliable customer history.

## Users

- Sales representatives record and review calls, complete activities, and create follow-up
  work.
- Sales managers review outcomes, coaching signals, and team execution.
- Tenant administrators manage users and CRM connectivity.
- Business owners review pipeline and productivity outcomes.

## Product boundary

Trigger sits above an external CRM.

- The CRM owns leads, contacts, pipeline state, and scheduled activities.
- Trigger owns recordings, transcripts, enrichments, and call-processing state.
- Trigger accesses CRM data through a provider-neutral API layer.
- Odoo 18 is the initial CRM provider.
- Additional providers, including Engaz CRM, can implement the same interface.

## Tenancy and identity

- WorkOS provides B2B authentication, SSO, and organization identity.
- One WorkOS organization maps to one Trigger tenant.
- One WorkOS user belongs to one Trigger tenant.
- Browser authentication is server-side and cookie-based.
- The native Android recorder uses separate revocable device authentication.
- Each tenant configures its CRM connection.
- A user may provide a personal CRM token that overrides the tenant key.

## User experience

The primary web application is a server-rendered HTMX PWA:

- no SPA build pipeline;
- installable browser experience;
- responsive activity and calls pages;
- incremental filtering and infinite scrolling;
- background processing status updates through polling initially or SSE later.

Native clients:

- Android TWA wrapper for the web product where useful.
- Native Android recorder for call capture, resumable upload, offline recovery, and device
  lifecycle.
- iOS web wrapper remains a future distribution option; native call recording capabilities
  remain platform-constrained.

## Recording workflow

```text
record call
→ resumable TUS upload
→ private R2 object
→ Trigger call record
→ persistent processing job
→ transcription
→ Claude enrichment
→ CRM activity match
→ user review and activity completion
```

The recorder can recover from connectivity loss and resume uploads. Processing is durable
across server restarts.

## Core capabilities

### Conversation intelligence

- Recording ingestion.
- Transcription and speaker-aware analysis.
- Summary, sentiment, outcome, objections, and action-item extraction.

### CRM copilot

- Match recordings to scheduled CRM activities.
- Complete activities with feedback.
- Create follow-up work.
- Draft structured CRM updates.

### Sales productivity

- Automatic call notes.
- Customer history.
- Follow-up reminders.
- Reduced manual CRM administration.

### Sales intelligence

- Team execution dashboards.
- Outcome and objection trends.
- Coaching signals.
- Pipeline progression and forecasting support.

## Technical direction

- Go 1.25 web server.
- HTMX and `html/template`.
- PostgreSQL for Trigger-owned state.
- Redis for cache and Asynq jobs.
- WorkOS AuthKit for browser identity.
- Private Cloudflare R2 for recordings and transcript artifacts.
- TUS for resumable native uploads.
- Deepgram initially for transcription.
- Anthropic Claude initially for enrichment, behind a replaceable service boundary.
- Structured logs, trace IDs, and Sentry for observability.

## Product success measures

- Less manual CRM data entry.
- Higher follow-up completion.
- Faster conversion from call to actionable CRM state.
- Better visibility into call outcomes and objections.
- Increased salesperson productivity and opportunity conversion.

## Delivery sequence

1. Shared platform infrastructure.
2. Identity, authentication, and tenant CRM configuration.
3. Native call ingestion and calls workflow.
4. Persistent enrichment and CRM completion.
5. Analytics and coaching.

Detailed implementation work is tracked in the corresponding plan documents listed in
[README.md](README.md).
