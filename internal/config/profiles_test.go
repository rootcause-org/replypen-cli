package config

import (
	"os"
	"path/filepath"
	"testing"
)

// isolate points the config dir at a temp dir and clears the base-URL env so each test sees a clean slate.
func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv(envBaseURL, "")
	return filepath.Join(dir, "replypen")
}

// TestLoadDefaultsToBuiltinBaseURL: with no config and no env, the default profile resolves to the
// built-in localhost default and flags it as from-default (so the command layer can warn).
func TestLoadDefaultsToBuiltinBaseURL(t *testing.T) {
	isolate(t)
	res, err := Load("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if res.Profile != DefaultProfile {
		t.Errorf("profile = %q, want %q", res.Profile, DefaultProfile)
	}
	if res.BaseURL != DefaultBaseURL || !res.BaseURLFromDefault {
		t.Errorf("base = %q fromDefault=%v, want %q/true", res.BaseURL, res.BaseURLFromDefault, DefaultBaseURL)
	}
}

// TestBaseURLPrecedence: env > stored profile base_url > default.
func TestBaseURLPrecedence(t *testing.T) {
	isolate(t)
	// Stored profile base_url is used when env is unset.
	if err := SaveFile(File{Default: Profile{Token: "t", BaseURL: "https://stored.example"}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	res, _ := Load("")
	if res.BaseURL != "https://stored.example" || res.BaseURLFromDefault {
		t.Errorf("stored base not used: %q (fromDefault=%v)", res.BaseURL, res.BaseURLFromDefault)
	}

	// Env wins over the stored value.
	t.Setenv(envBaseURL, "https://env.example")
	res, _ = Load("")
	if res.BaseURL != "https://env.example" {
		t.Errorf("env base not honored: %q", res.BaseURL)
	}
}

// TestNamedProfileResolution: a named profile reads its own [profiles.<name>] block, not [default].
func TestNamedProfileResolution(t *testing.T) {
	isolate(t)
	f := File{
		Default:  Profile{Token: "default-tok", BaseURL: "https://default.example"},
		Profiles: map[string]Profile{"staging": {Token: "stg-tok", BaseURL: "https://staging.example"}},
	}
	if err := SaveFile(f); err != nil {
		t.Fatalf("save: %v", err)
	}
	res, _ := Load("staging")
	if res.Profile != "staging" || res.BaseURL != "https://staging.example" {
		t.Errorf("named profile mis-resolved: %+v", res)
	}
}

// TestSaveFileIs0600: the config file holds a static bearer, so it must be owner-only.
func TestSaveFileIs0600(t *testing.T) {
	cfgDir := isolate(t)
	if err := SaveFile(File{Default: Profile{Token: "secret"}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(filepath.Join(cfgDir, "config.toml"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config perm = %o, want 600", perm)
	}
}
