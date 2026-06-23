package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// update regenerates the golden files instead of comparing. Run: go test ./internal/cli -update
var update = flag.Bool("update", false, "update golden files")

// stubServer returns canned JSON per debug endpoint so the renderers and JSON passthrough can be pinned by
// golden tests without a live API. Each handler asserts the bearer header is present (auth wiring) and
// echoes a fixed fixture; a couple of ids drive the error paths.
func stubServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/debug/whoami", func(w http.ResponseWriter, r *http.Request) {
		requireAuth(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "whoami.json"))
	})
	mux.HandleFunc("GET /api/v1/debug/projects", func(w http.ResponseWriter, r *http.Request) {
		requireAuth(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "projects.json"))
	})
	mux.HandleFunc("GET /api/v1/debug/projects/{slug}/threads", func(w http.ResponseWriter, r *http.Request) {
		requireAuth(t, r)
		// A project-scoped token reaching across projects → 403 FORBIDDEN (drives the error-path test).
		if r.PathValue("slug") == "forbidden" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"cross-project access denied","code":"FORBIDDEN"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "threads.json"))
	})
	mux.HandleFunc("GET /api/v1/debug/projects/{slug}/triage", func(w http.ResponseWriter, r *http.Request) {
		requireAuth(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "triage.json"))
	})
	mux.HandleFunc("GET /api/v1/debug/threads/{id}/trace", func(w http.ResponseWriter, r *http.Request) {
		requireAuth(t, r)
		// An id matching a missing endpoint shape: a plain-text 405 (older server) drives the no-envelope path.
		if r.PathValue("id") == "405" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte("Method Not Allowed\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "trace.json"))
	})
	mux.HandleFunc("POST /api/v1/debug/projects/{slug}/cli-token", func(w http.ResponseWriter, r *http.Request) {
		requireAuth(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"rpc_live_minted_token_value","project_slug":"` + r.PathValue("slug") + `"}`))
	})

	return httptest.NewServer(mux)
}

// newTestEnv builds an env wired to the stub server with a fixed output mode, capturing stdout/stderr. It
// sets a static bearer (tokenOvr) so newClient resolves auth without the config store, and isolates
// XDG_CONFIG_HOME so a real ~/.config/replypen is never read or written.
func newTestEnv(t *testing.T, srv *httptest.Server, output string) (*env, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // isolate the token store + config
	t.Setenv("REPLYPEN_BASE_URL", "")        // a stray env must not override the test base URL
	t.Setenv("REPLYPEN_TOKEN", "")           // …nor the test token
	var out, errb bytes.Buffer
	e := &env{
		profile:    "default",
		output:     output,
		baseURLOvr: srv.URL,
		tokenOvr:   "test-key",
		out:        &out,
		err:        &errb,
	}
	return e, &out, &errb
}

// run executes a command line against a fresh root built on the test env, returning the error from
// Execute (so the error-path test can assert non-nil).
func run(t *testing.T, e *env, args ...string) error {
	t.Helper()
	// Cobra resets the --output-bound field during parsing, so force the mode via an explicit -o arg
	// (mirroring how a user would) rather than presetting e.output. Likewise re-pass --token/--base-url so
	// the persistent-flag fields survive cobra's reset.
	pre := []string{}
	if e.output != "" {
		pre = append(pre, "-o", e.output)
	}
	if e.tokenOvr != "" {
		pre = append(pre, "--token", e.tokenOvr)
	}
	if e.baseURLOvr != "" {
		pre = append(pre, "--base-url", e.baseURLOvr)
	}
	args = append(pre, args...)
	root := newRootCmd(e, "0.1.0-test")
	root.SetArgs(args)
	root.SetOut(e.out)
	root.SetErr(e.err)
	return root.Execute()
}

func requireAuth(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
		t.Fatalf("missing/wrong auth header: %q", got)
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// assertGolden compares got against testdata/<name>, writing it when -update is set. Goldens are stable:
// fixtures use canned timestamps, never time.Now.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", name, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	if got != string(want) {
		t.Errorf("output mismatch for %s\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// assertJSONEqual checks two JSON byte slices decode to the same value — the passthrough contract: -o json
// must round-trip the server's body (re-indentation aside), never reshape it.
func assertJSONEqual(t *testing.T, want, got []byte) {
	t.Helper()
	var wv, gv any
	if err := json.Unmarshal(want, &wv); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if err := json.Unmarshal(got, &gv); err != nil {
		t.Fatalf("unmarshal got: %v\nraw: %s", err, got)
	}
	if !reflect.DeepEqual(wv, gv) {
		t.Errorf("JSON not equal\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
