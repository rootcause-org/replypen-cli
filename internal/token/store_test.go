package token

import (
	"os"
	"path/filepath"
	"testing"
)

func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return filepath.Join(dir, "replypen", "config.toml")
}

// TestSaveLoadRoundTrip: a saved token (with its pinned base URL) loads back intact.
func TestSaveLoadRoundTrip(t *testing.T) {
	isolate(t)
	want := Token{Token: "rpc_live_abc", BaseURL: "https://api.example"}
	if err := Save("default", want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, ok, err := Load("default")
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if got != want {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, want)
	}
}

// TestLoadMissingIsLoggedOut: no file, or an empty token, reads as ok=false (not an error).
func TestLoadMissingIsLoggedOut(t *testing.T) {
	isolate(t)
	if _, ok, err := Load("default"); ok || err != nil {
		t.Fatalf("expected logged-out, got ok=%v err=%v", ok, err)
	}
}

// TestSavePreservesOtherProfiles: saving one profile leaves the others untouched.
func TestSavePreservesOtherProfiles(t *testing.T) {
	isolate(t)
	if err := Save("default", Token{Token: "def"}); err != nil {
		t.Fatal(err)
	}
	if err := Save("staging", Token{Token: "stg", BaseURL: "https://staging.example"}); err != nil {
		t.Fatal(err)
	}
	d, ok, _ := Load("default")
	if !ok || d.Token != "def" {
		t.Errorf("default clobbered: %+v ok=%v", d, ok)
	}
	s, ok, _ := Load("staging")
	if !ok || s.Token != "stg" {
		t.Errorf("staging not saved: %+v ok=%v", s, ok)
	}
}

// TestDeleteLogsOut: delete clears the profile; a later load is logged-out.
func TestDeleteLogsOut(t *testing.T) {
	isolate(t)
	if err := Save("default", Token{Token: "def"}); err != nil {
		t.Fatal(err)
	}
	if err := Delete("default"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := Load("default"); ok {
		t.Error("token still present after delete")
	}
}

// TestStoreIs0600: the store holds a live bearer, so the file must be owner-only.
func TestStoreIs0600(t *testing.T) {
	path := isolate(t)
	if err := Save("default", Token{Token: "secret"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("store perm = %o, want 600", perm)
	}
}
