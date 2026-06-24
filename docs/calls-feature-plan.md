# Calls Feature Plan

Implements native recording ingestion, call records, CRM activity matching, transcription,
AI enrichment, the calls page, and CRM activity completion.

Prerequisites:

- [infrastructure-plan.md](infrastructure-plan.md) is complete.
- [identity-auth-plan.md](identity-auth-plan.md) is complete.
- [security-invariants.md](security-invariants.md) applies throughout.
- Odoo activity listing and completion described in [odoo-crm-status.md](odoo-crm-status.md)
  remain available.

---

## Feature decisions

- The Android recorder uses dedicated native authentication, not the browser session
  cookie.
- Uploads use TUS and stream to private R2 storage.
- PostgreSQL stores R2 object keys, never public recording URLs.
- Transcription and enrichment run through Redis/Asynq.
- Every call record is owned by both tenant and user.
- CRM activity matches are persisted before completion.
- Completion never trusts an activity ID supplied by the browser.

---

## Step 1 — Native recorder authentication

Define a dedicated authentication flow for the Android recorder.

Required properties:

- A browser-authenticated user explicitly enrolls a device.
- Enrollment produces a one-time code or deep link.
- The recorder exchanges it for a revocable device credential.
- Only a hash of the device credential is stored.
- Device records have proper tenant and user foreign keys.
- Credentials are scoped to recorder operations and cannot access browser settings.
- Rotation, revocation, expiry, and last-used timestamps are supported.
- Native middleware resolves the same trusted internal `UserID` and `TenantID` shape used
  by the calls service.

Add `pkg/db/migrations/003_recorder_devices.sql`:

```sql
CREATE TABLE recorder_devices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    credential_hash BYTEA NOT NULL UNIQUE,
    expires_at      TIMESTAMPTZ,
    last_used_at    TIMESTAMPTZ,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (id, tenant_id, user_id)
);

CREATE INDEX recorder_devices_owner_idx
ON recorder_devices (tenant_id, user_id);
```

Final token format and hashing parameters must be documented during implementation.

Done when:

- A device can enroll, authenticate, rotate, and revoke.
- A browser session cookie is rejected as a native bearer credential.
- Cross-tenant and revoked-device tests pass.

---

## Step 2 — Calls migration

Add `pkg/db/migrations/004_calls.sql`:

```sql
CREATE TABLE call_records (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    recorder_device_id  UUID,
    phone_e164          TEXT NOT NULL,
    duration_sec        INT NOT NULL CHECK (duration_sec >= 0),
    started_at          TIMESTAMPTZ NOT NULL,
    r2_key              TEXT NOT NULL UNIQUE,
    media_type          TEXT NOT NULL,
    size_bytes          BIGINT NOT NULL CHECK (size_bytes >= 0),
    upload_id           TEXT NOT NULL UNIQUE,
    matched_activity_id TEXT,
    status              TEXT NOT NULL DEFAULT 'uploaded'
                        CHECK (status IN (
                            'uploaded',
                            'queued',
                            'transcribing',
                            'enriching',
                            'completed',
                            'failed'
                        )),
    failure_code        TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT call_record_user_tenant_fk
        FOREIGN KEY (user_id, tenant_id)
        REFERENCES users(id, tenant_id),
    CONSTRAINT call_record_device_owner_fk
        FOREIGN KEY (recorder_device_id, tenant_id, user_id)
        REFERENCES recorder_devices(id, tenant_id, user_id)
);

CREATE TABLE call_enrichments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_record_id  UUID NOT NULL UNIQUE
                    REFERENCES call_records(id) ON DELETE CASCADE,
    transcript_text TEXT,
    transcript_key  TEXT,
    summary         TEXT,
    sentiment       TEXT CHECK (
                        sentiment IS NULL OR
                        sentiment IN ('positive', 'neutral', 'negative')
                    ),
    outcome         TEXT CHECK (
                        outcome IS NULL OR
                        outcome IN (
                            'interested',
                            'not_interested',
                            'follow_up',
                            'no_answer'
                        )
                    ),
    extracted_at    TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX call_records_owner_time_idx
ON call_records (tenant_id, user_id, started_at DESC);

CREATE INDEX call_records_owner_phone_time_idx
ON call_records (tenant_id, user_id, phone_e164, started_at DESC);
```

The identity and recorder-device migrations provide the composite unique constraints
required by its ownership foreign keys.

Done when:

- FK, ownership, status, outcome, and deletion tests pass.
- A user ID cannot be combined with another tenant ID.

---

## Step 3 — Calls domain and repository

Add domain types under `common/calls`:

```go
type Status string

const (
    StatusUploaded     Status = "uploaded"
    StatusQueued       Status = "queued"
    StatusTranscribing Status = "transcribing"
    StatusEnriching    Status = "enriching"
    StatusCompleted    Status = "completed"
    StatusFailed       Status = "failed"
)

type CallRecord struct {
    ID                string
    TenantID          string
    UserID            string
    PhoneE164         string
    DurationSec       int
    StartedAt         time.Time
    R2Key             string
    MediaType         string
    SizeBytes         int64
    MatchedActivityID string
    Status            Status
    FailureCode       string
    CreatedAt         time.Time
    UpdatedAt         time.Time
    Enrichment        *Enrichment
}
```

Use sqlc through the shared store. Required repository operations:

```text
CallRecordCreateFromUpload
CallRecordFindOwned
CallRecordListOwned
CallRecordStatusTransition
CallRecordActivityMatchSet
CallRecordActivityMatchClear
CallEnrichmentUpsertTranscript
CallEnrichmentUpsertAnalysis
CallEnrichmentOutcomeUpdate
```

Every read and write accepts trusted tenant and user IDs. State transitions use guarded
updates so duplicate workers cannot regress status.

Done when:

- Repository integration tests cover ownership and idempotent transitions.
- No method fetches a call record by ID alone for user-facing operations.

---

## Step 4 — R2 and TUS ingestion

Add `pkg/r2` for private-object operations and use a maintained TUS server implementation
with an S3-compatible datastore.

Native endpoints:

```text
POST   /api/recorder/uploads
HEAD   /api/recorder/uploads/:uploadID
PATCH  /api/recorder/uploads/:uploadID
```

Metadata allowlist:

```text
phone
duration_sec
started_at
media_type
original_filename
trace_id
```

Server behavior:

1. Authenticate the recorder device.
2. Validate metadata and declared upload size before accepting bytes.
3. Normalize the phone number to E.164.
4. Build the R2 key from authenticated tenant/user IDs and server-generated identifiers.
5. Stream chunks directly to private R2 storage.
6. On TUS finalization, create `call_records` idempotently using `upload_id`.
7. Enqueue processing only after the record and object are complete.

Do not return a public R2 URL. API responses return the call record ID and processing
status.

Apply size, media-type, timeout, and rate limits. Never use the original filename as an
unvalidated object-key path component.

Done when:

- Interrupted upload resumes from the correct offset.
- Duplicate finalization creates one record and one processing task.
- Cross-device and cross-tenant upload access is rejected.

---

## Step 5 — CRM activity matching

The merge usecase loads:

- CRM call activities through the auth-aware CRM registry.
- Call records scoped by current tenant and user.

Matching inputs:

- normalized E.164 phone;
- activity deadline;
- recording start time;
- configurable matching window, initially four hours.

Algorithm:

1. Index unmatched owned records by normalized phone.
2. For each CRM call activity, choose the nearest eligible record within the window.
3. Persist `matched_activity_id` using a guarded update.
4. Return matched activity/record rows.
5. Return unmatched CRM activities.
6. Return unconsumed recording rows.

Persisting the match makes subsequent extraction and completion deterministic. Re-matching
must not silently overwrite an existing match; provide an explicit corrective operation
if manual rematching is later required.

Done when:

- Deterministic unit tests cover multiple candidates, equal times, missing phones,
  already-matched records, and tenant isolation.

---

## Step 6 — Calls page

Route:

```text
GET /calls
```

Use the established HTMX content-swap, opaque-cursor, sentinel, and out-of-band counter
patterns.

Row states:

- Matched activity and recording.
- CRM activity without recording.
- Recording without CRM activity.
- Processing.
- Completed.
- Failed with a safe retry action where appropriate.

Recording playback/download obtains a short-lived presigned R2 URL from an authenticated
owned-resource endpoint. The template never embeds a permanent public URL.

Done when:

- Pagination does not duplicate or omit rows under stable input.
- Every recording action enforces tenant and user ownership.

---

## Step 7 — Durable extraction pipeline

Define Asynq task:

```text
calls.process
```

Payload:

```text
call_record_id
tenant_id
user_id
trace_id
```

The payload contains identifiers only.

Handler:

1. Reload the owned call record.
2. Exit successfully if already completed.
3. Transition `queued → transcribing`.
4. Generate a short-lived R2 read URL or stream the object.
5. Submit audio to Deepgram.
6. Store transcript text and raw transcript object key.
7. Transition `transcribing → enriching`.
8. Submit transcript to the configured enrichment service.
9. Validate structured summary, sentiment, and outcome.
10. Upsert enrichment.
11. Transition to `completed`.

Failures:

- Retry transient provider/network failures through Asynq.
- Record a stable `failure_code` after terminal failure.
- Never store provider keys in task payloads.
- All writes are idempotent.

Browser endpoints:

```text
POST /calls/:id/process
GET  /calls/:id/status
```

The POST route uses session authentication and strict Origin validation. It verifies
ownership and enqueues an idempotent task. Status may use bounded HTMX polling initially;
SSE can replace polling later without changing job semantics.

Done when:

- Worker restart and Redis retry do not duplicate enrichment.
- Terminal failure is visible and retryable according to policy.

---

## Step 8 — Complete the CRM activity

Route:

```text
POST /calls/:id/complete
```

Flow:

1. Load the call by ID, tenant ID, and user ID.
2. Require a persisted `matched_activity_id`.
3. Validate feedback and outcome.
4. Resolve the user-scoped CRM client.
5. Call `ActivityComplete` with the persisted activity ID.
6. Store the selected outcome and completion state idempotently.
7. Return the updated HTMX row.

The request body never supplies the authoritative activity ID.

Failure semantics:

- A CRM timeout leaves the local record retryable.
- An already-completed CRM activity is reconciled idempotently where provider semantics
  permit.
- Local state is not marked complete before the CRM confirms completion.

Done when:

- Completing a matched call removes the open Odoo activity.
- Tampered call IDs, activity IDs, cross-origin requests, and cross-tenant access fail.

---

## Step 9 — End-to-end verification

Required scenario:

```text
device enrollment
→ interrupted/resumed TUS upload
→ private R2 object
→ call record creation
→ Asynq processing
→ CRM activity match
→ enriched calls row
→ CRM activity completion
```

Verify:

- tenant and user isolation;
- device revocation;
- idempotent upload finalization;
- idempotent task retries;
- private object access;
- strict Origin checks on browser mutations;
- structured trace propagation from recorder through worker and CRM.
