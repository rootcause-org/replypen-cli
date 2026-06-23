// Package config resolves the two things every API-backed command needs before it can authenticate: a
// base URL and a PROFILE NAME (the key the token store is keyed by). The INTENT is a single, documented
// precedence so behavior is predictable across `--profile`, env, and the stored config.
//
// ReplyPen uses STATIC TOKEN auth (no OAuth, no brain markers): a profile is just `{ token, base_url }`
// persisted in ~/.config/replypen/config.toml. This package owns the base-URL precedence and the profile
// selection; the token VALUE lives in the same file but is read through internal/token.
//
// Precedence for the profile name (the store key):
//
//	explicit --profile <name>   → that profile
//	otherwise:                    "default"
//
// Precedence for the base URL (per field, flag/env win in the command layer; here env > stored > default):
//
//	REPLYPEN_BASE_URL > [profiles.<name>] base_url > built-in default
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	// DefaultBaseURL is the built-in fallback when neither config nor env sets one.
	DefaultBaseURL = "http://localhost:8080"

	// DefaultProfile is the profile used when no --profile is given.
	DefaultProfile = "default"

	envBaseURL = "REPLYPEN_BASE_URL"
)

// Resolved is the effective config for one invocation. Profile is the store key the command authenticates
// with. BaseURL is always non-empty; BaseURLFromDefault is true when nothing set one and we fell back to
// DefaultBaseURL (the command layer warns in that case).
type Resolved struct {
	Profile            string
	BaseURL            string
	BaseURLFromDefault bool
}

// Load resolves config for one invocation. profileName comes from --profile (empty → "default").
func Load(profileName string) (Resolved, error) {
	f, err := loadFile()
	if err != nil {
		return Resolved{}, err
	}
	name := profileName
	if name == "" {
		name = DefaultProfile
	}
	prof := f.ProfileNamed(name)
	res := Resolved{Profile: name}
	res.BaseURL, res.BaseURLFromDefault = resolveBaseURL(prof.BaseURL)
	return res, nil
}

// resolveBaseURL picks the first non-empty URL: the env override, then the given candidates in order, then
// the built-in default (with the from-default flag set so the command layer can warn).
func resolveBaseURL(candidates ...string) (string, bool) {
	if v := os.Getenv(envBaseURL); v != "" {
		return v, false
	}
	for _, u := range candidates {
		if u != "" {
			return u, false
		}
	}
	return DefaultBaseURL, true
}

// ConfigDir is the resolved ~/.config/replypen directory (XDG-style; honors XDG_CONFIG_HOME). The token
// store and the profile config share this dir, so it's exported for internal/token.
func ConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "replypen"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	return filepath.Join(home, ".config", "replypen"), nil
}

// ConfigPath is ~/.config/replypen/config.toml (exported for diagnostics/messages).
func ConfigPath() string {
	p, _ := configPath()
	return p
}

func configPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// --- the config.toml shape (shared with internal/token, which owns the secret read/write) ---------------

// Profile is one [default] / [profiles.<name>] block: a static bearer token + a base URL, both optional.
type Profile struct {
	Token   string `toml:"token"`
	BaseURL string `toml:"base_url"`
}

// File mirrors ~/.config/replypen/config.toml: a [default] profile plus named [profiles.<name>].
type File struct {
	Default  Profile            `toml:"default"`
	Profiles map[string]Profile `toml:"profiles"`
}

// ProfileNamed returns the named profile (the [default] block for "default"), or a zero Profile when
// absent. Exported so internal/token can read a profile's stored token without re-parsing the file.
func (f File) ProfileNamed(name string) Profile {
	if name == DefaultProfile {
		return f.Default
	}
	return f.Profiles[name]
}

func loadFile() (File, error) {
	f, _, err := LoadFile()
	return f, err
}

// LoadFile reads config.toml, returning the parsed file and whether it existed. A missing file is fine
// (the common pre-login case); a malformed one is an error so the user isn't silently mis-scoped.
func LoadFile() (File, bool, error) {
	path, err := configPath()
	if err != nil {
		return File{}, false, err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return File{}, false, nil
	}
	var f File
	if _, err := toml.DecodeFile(path, &f); err != nil {
		return File{}, false, fmt.Errorf("parse %s: %w", path, err)
	}
	return f, true, nil
}

// SaveFile writes config.toml atomically at 0600 (it holds the static bearer token), creating the config
// dir if needed. The rename keeps a concurrent reader from ever seeing a half-written file.
func SaveFile(f File) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	tmp := path + ".tmp"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := toml.NewEncoder(out).Encode(f); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("encode config: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil { // re-assert in case of a pre-existing looser temp
		_ = os.Remove(tmp)
		return fmt.Errorf("chmod %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
