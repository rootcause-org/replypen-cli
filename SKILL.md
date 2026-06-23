---
name: replypen-cli
description: The `rp` CLI — a static-token, scriptable Go client over replypen's thin /api/v1/debug surface. It traces a thread end to end (the offline twin of the status page), audits triage decisions, lists projects/threads, mints project-scoped CLI tokens, and bundles the local helpers — DNS provider detection (can this domain onboard?), Gmail/Outlook id decoding, and the tenant→project→mailbox onboarding wrappers. Auth is a static bearer (a super-admin token = all projects, or a project-scoped `rpc_live_` token); output is a table on a TTY and JSON when piped (`-o json` passes the server body through verbatim). Use when working in this repo — adding/changing a command, the HTTP client, the config/token store, or the table/JSON render layer — or when porting one of replypen's Python debug scripts to a `rp` subcommand.
---

# replypen-cli (`rp`) — a static-token window into replypen's debug surface

`rp` is a **fat client over a thin server**: replypen exposes raw, token-scoped debug data under
`/api/v1/debug/*`; `rp` renders it (a table on a TTY, **JSON when piped**) and does the local
decomposition the server shouldn't — the `thread trace --md/--jsonl` split, the DNS detect, the id math.
It holds **no DB access**: every byte of thread data comes through `/api/v1/debug`, authed with a
**static bearer token** the server resolves into a scope on each request. `rp` is the single Go binary
that replaces a pile of replypen's Python debug scripts (see the table below).

## The command ⇄ endpoint ladder

| Command | Endpoint / source | What |
|---|---|---|
| `rp login --token … [--base-url …]` | local token store (`~/.config/replypen/config.toml`) | persist `{token, base_url}` for a profile (0600) |
| `rp logout` | local store | clear the profile's token + base URL |
| `rp whoami` | `GET /api/v1/debug/whoami` | resolve the token's scope (admin vs project) — verifies end to end |
| `rp projects` | `GET /api/v1/debug/projects` | projects visible to the token (all for admin, one for project-scoped) |
| `rp threads <slug> [--limit] [--status]` | `GET …/projects/{slug}/threads` | recent threads, newest first, optional status filter |
| `rp triage <slug> [--limit] [--csv]` | `GET …/projects/{slug}/triage` | last N inbound threads + their triage decision |
| `rp thread trace <id> [--md] [--jsonl] [--out-dir]` | `GET …/threads/{id}/trace` | the full assembled bundle; render the merged timeline / decompose to files |
| `rp project mint-token <slug>` | `POST …/projects/{slug}/cli-token` (admin) | mint/rotate a project-scoped CLI token (shown once) |
| `rp provider detect <domain\|email …>` | **local** DNS | classify the email backend + onboardability (no API) |
| `rp id gmail <id>` / `rp id outlook <id>` | **local** pure math | decode a provider id; tell you which DB column matches (no API) |
| `rp tenant register` | `POST /api/v1/register` (X-Admin-Secret) | onboarding: register a tenant |
| `rp project create` | `POST /api/v1/projects` (tenant bearer) | onboarding: create a project |
| `rp mailbox connect` | `POST /api/v1/projects/{slug}/mailboxes/connect` (tenant bearer) | onboarding: print the OAuth URL to grant a mailbox |
| `rp upgrade [--check]` | GitHub releases (self-replace) | the one command that reaches outside replypen |

### Which Python debug scripts each command replaces
- `trace_thread.py` → **`rp thread trace`** (`--md`/`--jsonl` = the offline twin of the status page).
- `triage_logs.py` → **`rp triage`** (`--csv` emits the same column order, straight to a spreadsheet).
- `gmail_ids.py` → **`rp id gmail`**; `outlook_ids.py` → **`rp id outlook`**.
- `providers/detect.py` & `detect_mail_provider.py` → **`rp provider detect`**.
- `onboard.py` → **`rp tenant register`** + **`rp project create`** + **`rp mailbox connect`**.

## Auth — static token, server-resolved scope (NO OAuth)

A bearer is a **fixed string**; there is no refresh, no expiry, no login flow. The server resolves it
into a **scope** on every request:

- **admin** — the token equals the server's `REPLYPEN_CLI_ADMIN_TOKEN`. Scope = **all** tenants/projects.
  Required to mint project tokens and to trace across projects.
- **project** — a `rpc_live_<rand>` token whose SHA-256 matches a `projects.cli_token_hash`. Scope =
  that **one** project; cross-project reads → `403 FORBIDDEN`.

`rp login` simply persists `{token, base_url}` (no server call). `rp whoami` is the one that asks the
server which scope a token has.

**Token precedence** (in `env.newClient`): `--token` > `REPLYPEN_TOKEN` env > stored profile token.
None resolved → a clear "run `rp login`" error naming the profile. `--token` and `REPLYPEN_TOKEN` need
no store, so a one-off `rp --token … whoami` works with nothing on disk.

**Base-URL precedence** (in `env.resolveBase` + `config.resolveBaseURL`): `--base-url` >
`REPLYPEN_BASE_URL` > stored profile `base_url` > built-in default (`http://localhost:8080`). When the
built-in default is hit, `rp` warns on **stderr** (so piped stdout stays clean). The stored `base_url` is
the same field `rp login` pins, so a token always hits the server it was minted against.

> Two onboarding commands break the bearer pattern deliberately: `rp tenant register` authenticates with
> the **`X-Admin-Secret`** header (`--admin-secret` / `REPLYPEN_ADMIN_SECRET`), and so builds a
> token-less client (`env.adminClient`) that bypasses the "not logged in" gate. `rp project create` /
> `rp mailbox connect` carry the **tenant API token** via `--token` (not a debug token).

## Architecture — four thin layers, no logic

```
cmd/rp/main.go            → cli.Execute(version)
internal/cli/             cobra commands; one file per area (root/auth/debug/thread/onboard/local/upgrade).
                          A command = parse flags → one client call → render. root.go owns the global flags
                          (--profile/--base-url/--token/-o), newClient (token + base-URL precedence), and
                          printError (verbatim {code,message}; method+path+base for an envelope-less 404/405).
internal/client/          the ONE http wrapper (client.go: static bearer, JSON in/out, NO 401-retry) + the
                          TokenSource (auth.go) + the wire contract (types.go) + APIError (errors.go). One
                          method per endpoint; the -o json path uses client.Raw to passthrough verbatim.
internal/config/          base-URL + profile-name resolution; owns the ~/.config/replypen/config.toml shape.
internal/token/           the static-token store (same config.toml, 0600), one {token, base_url} per profile.
internal/render/          render.go (TTY-detect + JSON passthrough) + table.go (one renderer per view).
internal/debugdump/       the `thread trace` decomposer: thin markdown index + jq-able JSONL.
internal/dnsdetect/       `provider detect` — MX/SPF/autodiscover classification + onboardability.
internal/idutil/          `id gmail|outlook` — base conversion + DB-column mapping, pure + offline.
```

### Output: pipe-first, TTY-aware
`render.IsJSON(mode, w)` — `-o json`/`-o table` wins; else **JSON unless stdout is a terminal**. So a TTY
gets a table; a pipe/redirect gets JSON (`rp threads acme | jq …` always works). JSON mode is a
**verbatim pretty-print of the server body** (`client.Raw` → `render.JSON`, re-indent only), so jq sees
the true response shape — the CLI can't invent or drop a field. The local commands (`provider detect`,
`id …`) have no server body, so their JSON is a synthesized `render.Value` of the result struct
(`detect` emits a bare object for one arg, an array for many — matching the Python script).

### The `thread trace` decomposer (`internal/debugdump`)
`rp thread trace <id>` fetches the full bundle (`GET …/threads/{id}/trace`). `<id>` accepts a replypen
thread UUID **or** an external/provider thread id (server resolves either). Three output modes:

- **default** → render the server-merged, timestamp-sorted `timeline` as a readable table.
- **`-o json`** → the verbatim server bundle (the raw `messages`/`logs`/`deliveries`/`drafts`/`notes`
  sub-arrays stay reachable for jq).
- **`--md` / `--jsonl`** (to `--out-dir`, default `.replypen/debug/`) → the **offline twin of the status
  page**: a **thin markdown index** (header + outcome line + the timeline table + example jq calls) and a
  **jq-able JSONL** (line 1 a `{"type":"thread",…}` header carrying the full triage/injection blobs +
  project/mailbox; every later line one timeline entry keyed by `type` =
  `message|log|delivery|draft|note`). The agent reads the index, then jqs the JSONL — `rp` never
  pre-summarizes the whole thread into context. Both modes print the written paths to stdout.

### The fully-local commands
`provider detect` and `id gmail|outlook` are pure functions over their input — **no token, no base URL,
no network beyond DNS** for detect — so they never touch the client/config/token layers:

- **`provider detect <domain|email …>`** classifies the email backend from public DNS (MX → SPF
  tiebreaker → autodiscover → rDNS color) and reports whether replypen can onboard it. ReplyPen has
  exactly two email adapters — **google** and **microsoft**; everything else is `NOT SUPPORTED`. Accepts
  many targets at once.
- **`id gmail <id> [--user N]`** decodes a Gmail id (legacy `f:`/decimal/hex/web-opaque/local), prints the
  API hex + decimal + `thread-f:`/`msg-f:` forms, and builds a clickable `/u/N/` web URL.
- **`id outlook <id>`** classifies a Graph/OWA id (`owa-web`/`graph-message`/`conversation`/`unknown`) and
  tells you the matching DB column (`messages.external_message_id` / `threads.external_thread_id`) — or
  that a web-only id isn't offline-resolvable.

### Errors
Any non-2xx → the client decodes `{"error","code"}` into a typed `APIError`. A code-bearing envelope
prints `CODE: message` to stderr (exit 1). An **envelope-less** non-2xx (a proxy's plain-text 404/405, or
an older server) prints `METHOD PATH → HTTP <status>` + the base URL + a pointed hint, so a request that
silently went to the localhost default instead of the intended host is obvious. There is **no 401-retry**
— replypen tokens are static, so a 401 is a real auth failure, surfaced as-is.

## Working on it
- **Toolchain:** Go 1.25 via `mise` (`mise.toml` pins it). `cobra`+`pflag`, `BurntSushi/toml`. Build/run from the repo dir so mise selects go 1.25.
- **Before finishing any change:** `go build ./...`, `go vet ./...`, `go test ./...`, `gofmt -w`.
- **Tests** (`internal/cli/`): golden-file tests for each table renderer + the JSON-passthrough round-trip, driven by an `httptest` stub returning canned fixtures (`testdata/*.json` → `*.golden`), plus the `thread trace` decomposer (golden index + JSONL), the triage `--csv` shape, the API-error path (verbatim + exit), and the not-logged-in error. The upgrade pure helpers (version compare, asset name, checksum parse, brew-path detection) are unit-tested; the dnsdetect/idutil packages have offline unit tests. Tests bypass the store via `--token`/`--base-url`. Regenerate goldens with `go test ./internal/cli -update`; fixtures use **canned** timestamps, never `time.Now`.
- **Adding a command for a new endpoint:** add the wire struct to `internal/client/types.go` (match the server JSON exactly), a client method + a path builder (exported, so the `-o json` path can pass the same path to `client.Raw`), a render function (+ golden fixture/test), and a cobra command. Keep the endpoint thin and the `-o json` body verbatim.

## Scope guards (push back if asked)
**No DB access in the CLI** — data comes only through `/api/v1/debug`, and those endpoints stay thin (raw
token-scoped rows, not server-computed views), with `-o json` always exposing them verbatim. **Auth is
static-token only** — no OAuth, no new grant types, no token minting beyond the standard admin
`mint-token` flow. The fully-local commands stay token-less. `rp upgrade` is the **only** command that
reaches outside replypen (GitHub releases + self-replace; it refuses on a Homebrew install and defers to
`brew upgrade rp`). No interactive TUI — scriptable, pipe-first, headless.
