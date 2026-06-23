# AGENTS.md — replypen-cli (`rp`)

Start here, then open the doc the task needs. This file is a router, not a manual — the detail lives in
the two docs below, the API contract, and the code.

## What this is (one line)
`rp` is a **scriptable, static-token Go client** over replypen's thin `/api/v1/debug` surface: it traces
threads, audits triage, mints project tokens, and bundles the local DNS/id/onboarding helpers — rendered
as a table on a TTY or **JSON when piped**.

## Where to read
- **[README.md](README.md)** — user-facing: install, configure (`rp login`, profiles, env vars), every command with an example + a `jq` pipe, auth & scopes, releasing.
- **[SKILL.md](SKILL.md)** — architecture & intent: the command⇄endpoint ladder, the static-token auth model, the four thin layers, pipe-first output, the `thread trace` decomposer, the local commands. **Read before changing code.**
- **[docs/api-contract.md](docs/api-contract.md)** — the wire contract: every `/api/v1/debug/*` endpoint, the auth/scope rules, and the command ladder. `internal/client/types.go` field names MUST match it verbatim.

## Code map (detail in SKILL.md) — four thin layers + local utils + debugdump
- `cmd/rp/main.go` — entrypoint → `cli.Execute(version)`.
- `internal/cli/` — one cobra file per area (`root`/`auth`/`debug`/`thread`/`onboard`/`local`/`upgrade`); a command = parse flags → one client call → render. `root.go` holds the global flags + `newClient` (token/base-URL precedence) + verbatim error printing.
- `internal/client/` — the ONE HTTP wrapper (`client.go`, static bearer, no refresh) + the `TokenSource` (`auth.go`) + the wire contract (`types.go`, fields match the server) + `APIError` (`errors.go`).
- `internal/config/` — base-URL + profile-name resolution (`profiles.go`); owns the `~/.config/replypen/config.toml` shape.
- `internal/token/` — the static-token store (same `config.toml`, 0600), one `{token, base_url}` per profile.
- `internal/render/` — TTY-detect + JSON passthrough (`render.go`) + per-view table renderers (`table.go`).
- `internal/debugdump/` — the `thread trace --md/--jsonl` decomposer: thin markdown index + jq-able JSONL.
- `internal/dnsdetect/`, `internal/idutil/` — the fully-local commands (DNS provider detect; Gmail/Outlook id math), pure functions, unit-tested offline.

## Working on it
- **Toolchain:** Go 1.25 via `mise` (pinned in `mise.toml`); `cobra`+`pflag`, `BurntSushi/toml`. Run from the repo dir so mise selects go 1.25.
- **Before finishing any change:** `go build ./...`, `go vet ./...`, `go test ./...`, and `gofmt -w`.
- **Golden tests** live in `internal/cli/` (fixtures `testdata/*.json` → `*.golden`), driven by an `httptest` stub returning canned bundles; regenerate with `go test ./internal/cli -update`. Fixtures use **canned** timestamps — never `time.Now`. Tests bypass the store via `--token`/`--base-url` against the stub.
- **Adding a command for a new endpoint:** wire the struct in `internal/client/types.go` (match server JSON) → client method + path builder → render fn (+ golden) → cobra command. A command is 1:1 with one endpoint; the `-o json` path emits the server body **verbatim** via `client.Raw`, so jq sees the true shape.

## Scope guards (push back if asked to cross them)
- **No DB access in the CLI.** All data comes through `/api/v1/debug` — keep those server endpoints **thin** (raw token-scoped rows, no server-computed views).
- **Static-token auth only — no OAuth, no token minting beyond the standard flows.** A bearer is a fixed string the server resolves into a scope (admin vs one project); there is no refresh. `rp login` just persists `{token, base_url}` to the 0600 store.
- **Local-only commands need no token/base-url:** `provider detect`, `id gmail|outlook` are pure functions (DNS / base-conversion). They never call `newClient`.
- **`rp upgrade` is the only command that reaches outside replypen** (GitHub releases, self-replace). Keep it that way.
