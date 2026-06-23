# replypen-cli ⇄ replypen API contract (v1)

The CLI (`rp`) is a **fat client over a thin server**: the server exposes raw, token-scoped
debug data under `/api/v1/debug/*`; the CLI renders it (table on a TTY, JSON when piped, and
local `--md`/`--jsonl` decomposition for thread traces). Field names in the CLI's
`internal/client/types.go` MUST match the server JSON below verbatim.

## Auth — static bearer token (NO OAuth)

`Authorization: Bearer <token>`. Two token kinds, resolved server-side into a **scope**:

- **admin** — the token equals the server's `REPLYPEN_CLI_ADMIN_TOKEN` env value (constant-time
  compare). Scope = ALL tenants/projects. If the env var is unset, the whole `/api/v1/debug/*`
  surface 404s (fail closed, mirrors `adminSecretGate`).
- **project** — SHA-256 of the token matches a `projects.cli_token_hash`. Scope = that one project.

Middleware lives in `internal/clidebug/` (new package). It attaches a `Scope{Admin bool,
ProjectID uuid, ProjectSlug, TenantCodename}` to the request context. Project-scoped callers may
only read their own project's threads/data; cross-project access → `403 FORBIDDEN`. Missing/bad
token → `401 UNAUTHORIZED`. Reuse `web.Error`/`web.JSON` and the `{"error","code"}` envelope.

Token format for project tokens: `rpc_live_<32+ rand>`; hashed with the existing
`tenant.HashToken` (SHA-256) pattern, stored in new nullable column `projects.cli_token_hash`.

## Endpoints

### `GET /api/v1/debug/whoami`
Returns the resolved scope so `rp whoami` can verify a token.
```json
{ "scope": "admin", "project_slug": null, "tenant_codename": null }
{ "scope": "project", "project_slug": "acme-support", "tenant_codename": "acme" }
```

### `GET /api/v1/debug/projects`
admin → all projects; project-scoped → just the caller's project.
```json
{ "projects": [
  { "id": "uuid", "slug": "acme-support", "tenant_codename": "acme",
    "webhook_url": "https://…", "mailbox_count": 2, "created_at": "RFC3339" }
] }
```

### `GET /api/v1/debug/projects/{slug}/threads?limit=20&status=`
Recent threads for a project (newest first). `status` optional filter.
```json
{ "threads": [
  { "id": "uuid", "external_thread_id": "…", "subject": "…", "status": "webhook_sent",
    "sender": "a@b.com", "mailbox_email": "support@acme.com", "channel": "google",
    "created_at": "RFC3339", "updated_at": "RFC3339" }
] }
```

### `GET /api/v1/debug/projects/{slug}/triage?limit=20`
Replacement for `triage_logs.py`: last N inbound threads + their triage decision.
```json
{ "decisions": [
  { "thread_id": "uuid", "received_at": "RFC3339|null", "sender": "a@b.com", "subject": "…",
    "thread_status": "triage_classified", "should_process": true, "category": "support",
    "confidence": 0.91, "reason": "…", "mailbox": "support@acme.com" }
] }
```

### `GET /api/v1/debug/threads/{id}/trace`
Replacement for `trace_thread.py`. `{id}` accepts a replypen thread UUID **or** an
external/provider thread id (resolve via `GetThreadByExternalIDAny`). Project-scoped tokens may
only trace threads in their project. Returns the full assembled bundle; the CLI renders the
`timeline` to a status-page-style `.md` and a jq-able `.jsonl`.
```json
{
  "thread":  { "id","external_thread_id","subject","status","channel",
               "active_session_id","pipeline_started_at":"RFC3339|null","pending_reentry",
               "triage_result":{…}|null, "injection_result":{…}|null,
               "error_message":"…|null", "processor_failure":{…}|null, "decline_reason":"…|null",
               "created_at","updated_at" },
  "mailbox": { "id","email","provider","status" },
  "project": { "id","slug","tenant_codename","webhook_url" },
  "messages":   [ { "id","external_message_id","direction","is_draft","from","to","cc",
                    "subject","date","body_cleaned_len","labels":[…] } ],
  "logs":       [ { "step","status","details":{…}|null,"duration_ms":int|null,"created_at" } ],
  "deliveries": [ { "id","direction","session_id","turn_index","url",
                    "response_status":int|null,"attempt","created_at" } ],
  "drafts":     [ { "id","status","proposed_at","placed_at":"RFC3339|null",
                    "deletion_reason":"…|null","external_draft_id":"…|null" } ],
  "notes":      [ { "id","code","kind","status","body_markdown","created_at","deleted_at":"RFC3339|null" } ],
  "timeline":   [ { "ts":"RFC3339","type":"message|log|delivery|draft|note","label":"…","detail":"…" } ]
}
```
`timeline` is the server-merged, timestamp-sorted interleave the CLI prints/decomposes. Each
entry's `type` keys the JSONL line; the CLI also writes a thin `.md` index. The raw sub-arrays
stay reachable via `rp thread trace <id> -o json`.

> **Null-ability (verified against the server's `clidebug` structs).** `processor_failure` is a JSON
> **blob** (or `null`) — never a string. `pipeline_started_at`, `received_at` (triage), draft
> `placed_at`/`deletion_reason`/`external_draft_id`, note `deleted_at`, log `duration_ms`, and delivery
> `response_status` are all nullable. `labels` is always present (currently the server emits `[]`).

### `POST /api/v1/debug/projects/{slug}/cli-token`  (admin scope only)
Mint/rotate the project-scoped CLI token. Returns it once (stores only the hash).
```json
{ "token": "rpc_live_…", "project_slug": "acme-support" }  // shown once
```

## CLI command ⇄ endpoint ladder

| Command | Endpoint / source |
|---|---|
| `rp login --token … [--base-url …]` | local token store (`~/.config/replypen/`) |
| `rp whoami` | `GET /api/v1/debug/whoami` |
| `rp projects` | `GET /api/v1/debug/projects` |
| `rp threads <slug> [--limit] [--status]` | `GET …/projects/{slug}/threads` |
| `rp triage <slug> [--limit] [--csv]` | `GET …/projects/{slug}/triage` |
| `rp thread trace <id> [--md] [--jsonl] [--out-dir]` | `GET …/threads/{id}/trace` |
| `rp project mint-token <slug>` | `POST …/projects/{slug}/cli-token` (admin) |
| `rp provider detect <domain|email>` | **local** DNS (no API) |
| `rp id gmail <id>` / `rp id outlook <id>` | **local** pure math (no API) |
| `rp tenant register` / `rp project create` / `rp mailbox connect` | existing `/api/v1/register`, `/api/v1/projects`, `/api/v1/projects/{slug}/mailboxes/connect` |
| `rp upgrade` | GitHub releases (self-replace) |

## Output rules (mirror rootcause-cli)
- `render.IsJSON(mode,w)`: `-o json|table` wins; else JSON unless stdout is a TTY.
- JSON mode = verbatim pretty-print of the server body (re-indent only) so `jq` sees the true shape.
- Errors: decode `{"error":…,"code":…}` → typed `APIError`; print `CODE: message` to stderr, exit 1.
- No DB access in the CLI. Local-only commands (`provider detect`, `id …`) need no token/base-url.
