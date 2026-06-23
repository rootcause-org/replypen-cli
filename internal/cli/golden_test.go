package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rootcause-org/replypen-cli/internal/client"
)

// --- table-mode golden tests: each pins one renderer's human output ----------------------------------

func TestWhoamiTable(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()
	e, out, _ := newTestEnv(t, srv, "table")
	if err := run(t, e, "whoami"); err != nil {
		t.Fatalf("whoami: %v", err)
	}
	assertGolden(t, "whoami.golden", out.String())
}

func TestProjectsTable(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()
	e, out, _ := newTestEnv(t, srv, "table")
	if err := run(t, e, "projects"); err != nil {
		t.Fatalf("projects: %v", err)
	}
	assertGolden(t, "projects.golden", out.String())
}

func TestThreadsTable(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()
	e, out, _ := newTestEnv(t, srv, "table")
	if err := run(t, e, "threads", "acme-support"); err != nil {
		t.Fatalf("threads: %v", err)
	}
	assertGolden(t, "threads.golden", out.String())
}

func TestTriageTable(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()
	e, out, _ := newTestEnv(t, srv, "table")
	if err := run(t, e, "triage", "acme-support"); err != nil {
		t.Fatalf("triage: %v", err)
	}
	assertGolden(t, "triage.golden", out.String())
}

// TestTriageCSV pins the --csv output: the spec's exact column order, emitted regardless of -o.
func TestTriageCSV(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()
	e, out, _ := newTestEnv(t, srv, "table")
	if err := run(t, e, "triage", "acme-support", "--csv"); err != nil {
		t.Fatalf("triage --csv: %v", err)
	}
	assertGolden(t, "triage.csv.golden", out.String())
}

func TestTraceTable(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()
	e, out, _ := newTestEnv(t, srv, "table")
	if err := run(t, e, "thread", "trace", "aaaaaaaa-1111-1111-1111-111111111111"); err != nil {
		t.Fatalf("thread trace: %v", err)
	}
	assertGolden(t, "trace.golden", out.String())
}

func TestMintTokenTable(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()
	e, out, _ := newTestEnv(t, srv, "table")
	if err := run(t, e, "project", "mint-token", "acme-support"); err != nil {
		t.Fatalf("project mint-token: %v", err)
	}
	assertGolden(t, "mint_token.golden", out.String())
}

// --- JSON-mode passthrough: -o json must emit the canned server body verbatim ------------------------

func TestWhoamiJSONPassthrough(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()
	e, out, _ := newTestEnv(t, srv, "json")
	if err := run(t, e, "whoami"); err != nil {
		t.Fatalf("whoami -o json: %v", err)
	}
	assertJSONEqual(t, fixture(t, "whoami.json"), out.Bytes())
}

func TestProjectsJSONPassthrough(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()
	e, out, _ := newTestEnv(t, srv, "json")
	if err := run(t, e, "projects"); err != nil {
		t.Fatalf("projects -o json: %v", err)
	}
	assertJSONEqual(t, fixture(t, "projects.json"), out.Bytes())
}

func TestTraceJSONPassthrough(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()
	e, out, _ := newTestEnv(t, srv, "json")
	if err := run(t, e, "thread", "trace", "aaaaaaaa-1111-1111-1111-111111111111"); err != nil {
		t.Fatalf("trace -o json: %v", err)
	}
	assertJSONEqual(t, fixture(t, "trace.json"), out.Bytes())
}

// --- thread-trace decomposer: the cross-tool .md + .jsonl seam ---------------------------------------

// TestTraceDecompose locks the --md/--jsonl decomposer's two output files against goldens. The printed
// PATHS are non-deterministic (a temp out-dir), so we golden the FILE CONTENTS, not stdout.
func TestTraceDecompose(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()
	outDir := t.TempDir()
	e, out, _ := newTestEnv(t, srv, "table")
	if err := run(t, e, "thread", "trace", "aaaaaaaa-1111-1111-1111-111111111111", "--md", "--jsonl", "--out-dir", outDir); err != nil {
		t.Fatalf("thread trace --md --jsonl: %v", err)
	}
	// stdout carries the two written paths (index first, then jsonl).
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 printed paths, got %d: %q", len(lines), out.String())
	}

	base := "aaaaaaaa-acme-support"
	idx, err := os.ReadFile(filepath.Join(outDir, base+".md"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	assertGolden(t, "trace.md.golden", string(idx))

	jsonl, err := os.ReadFile(filepath.Join(outDir, base+".jsonl"))
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}
	assertGolden(t, "trace.jsonl.golden", string(jsonl))

	// Contract checks on the JSONL: a type:thread header then one line per timeline entry, keyed by `type`.
	jl := strings.Split(strings.TrimRight(string(jsonl), "\n"), "\n")
	if len(jl) != 7 { // 1 header + 6 timeline entries
		t.Fatalf("expected 7 JSONL lines, got %d", len(jl))
	}
	var head map[string]any
	if err := json.Unmarshal([]byte(jl[0]), &head); err != nil || head["type"] != "thread" {
		t.Fatalf("first line not a thread header: %v (%v)", head["type"], err)
	}
	// The triage_result blob must ride through verbatim (a nested object, not a boolean/string).
	if _, ok := head["triage_result"].(map[string]any); !ok {
		t.Errorf("triage_result not carried as an object: %v", head["triage_result"])
	}
	types := map[string]int{}
	for _, ln := range jl[1:] {
		var ev map[string]any
		if err := json.Unmarshal([]byte(ln), &ev); err != nil {
			t.Fatalf("timeline line not JSON: %v", err)
		}
		types[ev["type"].(string)]++
	}
	for _, want := range []string{"message", "log", "delivery", "draft", "note"} {
		if types[want] == 0 {
			t.Errorf("missing a %q timeline entry in JSONL", want)
		}
	}
}

// --- error paths -------------------------------------------------------------------------------------

// TestAPIErrorPath asserts a {"error","code"} envelope is surfaced verbatim (CODE: message) to stderr and
// Execute returns non-nil (→ exit 1).
func TestAPIErrorPath(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()
	e, _, errb := newTestEnv(t, srv, "table")
	err := run(t, e, "threads", "forbidden")
	if err == nil {
		t.Fatal("expected error from FORBIDDEN, got nil")
	}
	printError(errb, err)
	if got := errb.String(); !strings.Contains(got, "FORBIDDEN: cross-project access denied") {
		t.Errorf("missing verbatim code/message: %q", got)
	}
}

// TestErrorIsTyped confirms the client returns a typed *APIError carrying code+message — the load-bearing
// contract for verbatim surfacing.
func TestErrorIsTyped(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()
	e, _, _ := newTestEnv(t, srv, "table")
	err := run(t, e, "threads", "forbidden")
	var apiErr *client.APIError
	if !asAPIError(err, &apiErr) {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.Code != "FORBIDDEN" {
		t.Errorf("code = %q, want FORBIDDEN", apiErr.Code)
	}
}

// TestNonEnvelopeHTTPError asserts a plain-text non-2xx (a 405 from an older server) is rendered with
// method + path + status text + base URL — not a bare "HTTP 405".
func TestNonEnvelopeHTTPError(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()
	e, _, errb := newTestEnv(t, srv, "table")
	err := run(t, e, "thread", "trace", "405")
	if err == nil {
		t.Fatal("expected error from 405, got nil")
	}
	printError(errb, err)
	got := errb.String()
	for _, want := range []string{
		"GET /api/v1/debug/threads/405/trace → HTTP 405 Method Not Allowed",
		"base URL: " + srv.URL,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestNotLoggedIn asserts a clear "run `rp login`" error when no token resolves (no token store, no
// --token, no env).
func TestNotLoggedIn(t *testing.T) {
	srv := stubServer(t)
	defer srv.Close()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("REPLYPEN_BASE_URL", "")
	t.Setenv("REPLYPEN_TOKEN", "")
	var out, errb strings.Builder
	// No tokenOvr: newClient consults the (empty, isolated) store.
	e := &env{profile: "default", output: "table", baseURLOvr: srv.URL, out: &out, err: &errb}
	root := newRootCmd(e, "0.1.0-test")
	root.SetArgs([]string{"-o", "table", "--base-url", srv.URL, "whoami"})
	root.SetOut(&out)
	root.SetErr(&errb)
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("expected a not-logged-in error, got %v", err)
	}
}

// --- local commands (no token / no network) ----------------------------------------------------------

func TestProviderDetectJSON(t *testing.T) {
	// Local command: no server. A bogus TLD won't resolve, so the verdict is a clean unknown — exercising
	// the synthesized-JSON path (writeJSON) without network dependence on a real domain's records.
	var out, errb strings.Builder
	e := &env{profile: "default", output: "json", out: &out, err: &errb}
	root := newRootCmd(e, "0.1.0-test")
	root.SetArgs([]string{"-o", "json", "provider", "detect", "no-such-domain.invalid"})
	root.SetOut(&out)
	root.SetErr(&errb)
	if err := root.Execute(); err != nil {
		t.Fatalf("provider detect: %v", err)
	}
	var r map[string]any
	if err := json.Unmarshal([]byte(out.String()), &r); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	if r["provider"] != "unknown" || r["supported"] != false {
		t.Errorf("expected unknown/unsupported for an unresolvable domain, got %v", r)
	}
}

func TestIDGmailRoundTrip(t *testing.T) {
	var out, errb strings.Builder
	e := &env{profile: "default", output: "json", out: &out, err: &errb}
	root := newRootCmd(e, "0.1.0-test")
	root.SetArgs([]string{"-o", "json", "id", "gmail", "thread-f:1866870901077892614"})
	root.SetOut(&out)
	root.SetErr(&errb)
	if err := root.Execute(); err != nil {
		t.Fatalf("id gmail: %v", err)
	}
	var r map[string]any
	if err := json.Unmarshal([]byte(out.String()), &r); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if r["hex"] != "19e875318442de06" {
		t.Errorf("hex = %v, want 19e875318442de06", r["hex"])
	}
}
