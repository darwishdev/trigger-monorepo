# Calls Feature Plan

Covers everything from R2 upload (Android APK integration) through the calls page
merge logic, extraction pipeline, and completing the activity cycle back to the CRM.

Prerequisite: auth plan must be complete — every step here reads `AuthUser` from context.

---

## Step 1 — Call Records Migration

**Goal:** `call_records` and `call_enrichments` tables exist in the DB.

### `pkg/db/migrations/003_calls.sql`

```sql
CREATE TABLE call_records (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id),
    user_id      UUID NOT NULL REFERENCES users(id),
    phone        TEXT NOT NULL,
    duration_sec INT  NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL,
    r2_url       TEXT NOT NULL,
    r2_key       TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'uploaded',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE call_enrichments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    call_record_id  UUID NOT NULL UNIQUE REFERENCES call_records(id),
    transcript_text TEXT,
    transcript_url  TEXT,
    summary         TEXT,
    sentiment       TEXT,
    outcome         TEXT,
    extracted_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON call_records (tenant_id, user_id, started_at DESC);
CREATE INDEX ON call_records (phone, started_at DESC);
```

Call status values: `uploaded` → `transcribing` → `transcribed` | `failed`

---

## Step 2 — Calls Domain Types

**Goal:** Clean domain types for call records and enrichments — no DB or R2 details.

### `common/calls/calls.go`

```go
package calls

import "time"

type Status string
const (
    StatusUploaded     Status = "uploaded"
    StatusTranscribing Status = "transcribing"
    StatusTranscribed  Status = "transcribed"
    StatusFailed       Status = "failed"
)

type CallRecord struct {
    ID          string
    TenantID    string
    UserID      string
    Phone       string
    DurationSec int
    StartedAt   time.Time
    R2URL       string
    R2Key       string
    Status      Status
    CreatedAt   time.Time
    Enrichment  *Enrichment // nil until extraction runs
}

type Enrichment struct {
    ID             string
    CallRecordID   string
    TranscriptText string
    TranscriptURL  string
    Summary        string
    Sentiment      string // positive | neutral | negative
    Outcome        string // interested | not_interested | follow_up | no_answer
    ExtractedAt    time.Time
}

// MergedCall is one row on the calls page.
// Activity is nil for unscheduled calls (record exists, no CRM task found).
// Record is nil for unmatched CRM tasks (no recording found).
type MergedCall struct {
    Activity *sales.Activity // from CRM
    Record   *CallRecord     // from Trigger DB
}
```

---

## Step 3 — Call Records Repo

**Goal:** Typed DB query functions for call_records and call_enrichments.

### `app/calls/repo/repo.go`

```go
package callsrepo

type Repo struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repo
```

### Functions to implement

```go
// write path (called after R2 upload)
CreateCallRecord(ctx, tenantID, userID, phone string,
                 durationSec int, startedAt time.Time,
                 r2URL, r2Key string) (calls.CallRecord, error)

// status updates (used by extraction pipeline)
UpdateCallStatus(ctx, id string, status calls.Status) error

// read path (called by merge usecase)
ListCallRecords(ctx, tenantID, userID string,
                since time.Time) ([]calls.CallRecord, error)

GetCallRecord(ctx, id string) (calls.CallRecord, error)

// enrichment write (called by extraction pipeline)
UpsertEnrichment(ctx, callRecordID, transcriptText, transcriptURL,
                 summary, sentiment, outcome string) (calls.Enrichment, error)
```

### Done when

Integration tests for all functions pass.

---

## Step 4 — Cloudflare R2 Client

**Goal:** Go can upload files to R2; returns a public URL and internal key.

### `pkg/r2/r2.go`

R2 is S3-compatible. Use `github.com/aws/aws-sdk-go-v2/service/s3`.

```go
package r2

import "context"

type Client struct{ /* s3 client, bucket name */ }

func New(accountID, accessKey, secretKey, bucket string) (*Client, error)

// Upload streams a file to R2 and returns the public URL + internal key.
// Key format: {tenantID}/{userID}/{timestamp}-{filename}
func (c *Client) Upload(ctx context.Context,
                        key string,
                        r io.Reader,
                        contentType string,
                        size int64) (url string, err error)
```

### Config additions

```
R2_ACCOUNT_ID=...
R2_ACCESS_KEY=...
R2_SECRET_KEY=...
R2_BUCKET=trigger-calls
```

### Done when

`curl` upload of a test audio file creates a readable object in R2.

---

## Step 5 — Upload Endpoint (Android APK integration)

**Goal:** The Android APK can upload a recorded call; a `call_record` row is created.

### `api/upload.go`

```
POST /calls/upload
  Auth:    SessionMiddleware (APK sends the user's session token as Bearer)
  Content: multipart/form-data
  Fields:
    file         audio file (mp3 / m4a / wav)
    phone        caller or callee number, E.164
    duration_sec integer seconds
    started_at   RFC3339 timestamp from the device clock
```

Handler flow:
1. Read `AuthUser` from ctx (tenantID, userID)
2. Parse multipart form
3. Build R2 key: `{tenantID}/{userID}/{started_at_unix}-{phone}.{ext}`
4. Stream file to R2 via `r2.Client.Upload`
5. `callsRepo.CreateCallRecord(...)` with returned URL + key
6. Return `{"id": "...", "r2_url": "..."}` JSON

### Done when

`curl -F "file=@test.mp3" -F "phone=+201001234567" -F "duration_sec=120" -F "started_at=..." /calls/upload`
creates a DB row and the file is accessible in R2.

---

## Step 6 — Merge Usecase

**Goal:** Combine CRM call activities with call records into a single merged list.

### `app/calls/usecase/calls.go`

```go
func (u *Usecase) ListMergedCalls(ctx context.Context,
                                  req CallListReq) ([]calls.MergedCall, error) {
    user := auth.MustFromCtx(ctx)

    // 1. fetch CRM call activities (type=Call, keyset paged)
    crm, _ := u.reg.Build(sales.CRMConfig{...from user...})
    page, _ := crm.ActivityList(ctx, sales.ActivityFilter{Type: "Call", ...})

    // 2. fetch call_records for this user, within the time window
    records, _ := u.callsRepo.ListCallRecords(ctx, user.TenantID, user.UserID, since)

    // 3. in-memory merge: match on phone + time window
    return merge(page.Results, records), nil
}
```

### Merge algorithm

```
index records by phone → map[phone][]CallRecord

for each activity in CRM results:
    lead_phone = activity.Lead.Phone  // may be empty
    if lead_phone == "":
        → MergedCall{Activity: &activity, Record: nil}  // unrecordable
        continue
    candidates = index[normalise(lead_phone)]
    match = first candidate where |candidate.StartedAt - activity.Deadline| < 4h
    → MergedCall{Activity: &activity, Record: match}  // match may be nil

for each record not consumed by any activity:
    → MergedCall{Activity: nil, Record: &record}  // unscheduled call
```

Phone numbers are normalised to E.164 before comparison.
The 4-hour window is a constant for now; make it configurable if needed.

### Done when

A seeded `call_record` whose phone matches a CRM lead's phone appears as a merged row.

---

## Step 7 — Calls Page

**Goal:** Single page renders the merged call list with appropriate states per row.

### `api/calls.go`

```
GET /calls   keyset-paged; same HX-Request / scroll_token pattern as activities
```

### `templates/calls.html`

Three row states:

**Matched** (CRM activity + recording):
```
[Call badge] [Lead name]  [Phone]  [Duration]  [Deadline]  [Extract] [Complete]
```

**Unmatched CRM task** (activity, no recording):
```
[Call badge] [Lead name]  [Phone]  [Deadline]  [Not recorded]
```

**Orphan record** (recording, no CRM task):
```
[Recording badge]  [Phone]  [Duration]  [Started at]  [Unscheduled]
```

Infinite scroll and live counter follow the same sentinel + OOB pattern as the activity list.

---

## Step 8 — Extraction Pipeline

**Goal:** User clicks Extract on a matched call; transcript and AI enrichment are added.

### Job runner (`pkg/jobs/jobs.go`)

Simple in-process goroutine pool — no external queue yet:

```go
package jobs

type Pool struct{ sem chan struct{} }

func New(concurrency int) *Pool
func (p *Pool) Submit(fn func(ctx context.Context))
```

### Extraction steps (sequential, run in background goroutine)

```
1. fetch audio from R2 (presigned URL, 1h expiry)
2. POST audio to Deepgram → raw transcript JSON
3. extract transcript text + save to call_enrichments.transcript_text
4. save raw transcript file to R2 → save URL to call_enrichments.transcript_url
5. POST transcript to Claude:
      prompt → {summary, sentiment, outcome}
6. save enrichment fields
7. callsRepo.UpdateCallStatus(id, StatusTranscribed)
```

If any step fails: `UpdateCallStatus(id, StatusFailed)` + log error.

### `api/calls.go` — extraction endpoints

```
POST /calls/:id/extract
  validates ownership (call_record.user_id == AuthUser.UserID)
  sets status = transcribing
  submits background job
  returns 202 Accepted with HTMX fragment swapping the Extract button to a spinner

GET /calls/:id/status
  returns current status as JSON
  HTMX polls this every 3s while status == transcribing
  when status == transcribed → HTMX swaps the row with the enriched view
```

### Template polling pattern

```html
<!-- spinner state, returned by POST /calls/:id/extract -->
<div id="call-{{.ID}}-status"
     hx-get="/calls/{{.ID}}/status"
     hx-trigger="every 3s"
     hx-swap="outerHTML">
  <span class="spinner">Extracting…</span>
</div>
```

When `GET /calls/:id/status` returns `transcribed`, the server returns the enriched
row fragment (summary, sentiment badge, outcome) which replaces the spinner.

### Config additions

```
DEEPGRAM_API_KEY=...
ANTHROPIC_API_KEY=...
```

### Done when

Full cycle in the browser: Upload (via curl) → matched row appears → click Extract →
spinner shows → enrichment appears with summary, sentiment badge, and outcome.

---

## Step 9 — Complete Activity Cycle

**Goal:** User completes a CRM activity through Trigger (not directly in Odoo).

### `api/calls.go`

```
POST /calls/:id/complete
  Body: {feedback string, outcome string}
  1. load call_record by id (validate ownership)
  2. get matched activity ID from request body or record metadata
  3. crm.ActivityComplete(ctx, activityID, feedback)  // calls Odoo, Odoo deletes it
  4. if enrichment exists: update outcome field
  5. return HTMX fragment: removes Complete button, shows outcome badge
```

### Template

```html
<form hx-post="/calls/{{.Record.ID}}/complete" hx-swap="outerHTML" hx-target="closest article">
  <input name="feedback" placeholder="Add a note…">
  <select name="outcome">
    <option value="interested">Interested</option>
    <option value="not_interested">Not interested</option>
    <option value="follow_up">Follow up</option>
    <option value="no_answer">No answer</option>
  </select>
  <button type="submit">Complete</button>
</form>
```

### Done when

Completing via Trigger removes the activity from Odoo's open activity list.
The calls page row shows the outcome badge and no longer has a Complete button.
