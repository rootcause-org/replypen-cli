// Package token is the on-disk credential store for ReplyPen's STATIC bearer tokens:
// ~/.config/replypen/config.toml (0600), one `{ token, base_url }` per profile. Unlike rootcause's OAuth
// store there is no refresh, no expiry — a token is a fixed string the server resolves into a scope
// (super-admin or project) on every request. This package owns the file's 0600 mode and the per-profile
// keying; the TOML shape itself lives in internal/config (shared so `config.SaveFile` can write it).
package token

import (
	"github.com/rootcause-org/replypen-cli/internal/config"
)

// Token is one profile's stored credential: a static bearer + the base URL it was minted against (pinned
// so a later command hits the same server even if the ambient base URL drifted).
type Token struct {
	Token   string
	BaseURL string
}

// Load returns the token stored for profile, with ok=false when there is none (no file, no such profile,
// or an empty token). A malformed config surfaces as an error rather than silently looking "logged out".
func Load(profile string) (Token, bool, error) {
	f, _, err := config.LoadFile()
	if err != nil {
		return Token{}, false, err
	}
	p := f.ProfileNamed(profile)
	if p.Token == "" {
		return Token{}, false, nil
	}
	return Token{Token: p.Token, BaseURL: p.BaseURL}, true, nil
}

// Save writes (or replaces) the token + base URL for profile, preserving every other profile's entry, at
// 0600 (the file holds a live bearer). Used by `rp login`.
func Save(profile string, t Token) error {
	f, _, err := config.LoadFile()
	if err != nil {
		return err
	}
	p := config.Profile{Token: t.Token, BaseURL: t.BaseURL}
	if profile == config.DefaultProfile {
		f.Default = p
	} else {
		if f.Profiles == nil {
			f.Profiles = map[string]config.Profile{}
		}
		f.Profiles[profile] = p
	}
	return config.SaveFile(f)
}

// Delete clears the token for profile (a no-op if absent). Used by `rp logout`. The profile entry's
// base_url is dropped too — logout removes the whole credential.
func Delete(profile string) error {
	f, exists, err := config.LoadFile()
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if profile == config.DefaultProfile {
		f.Default = config.Profile{}
	} else {
		if _, ok := f.Profiles[profile]; !ok {
			return nil
		}
		delete(f.Profiles, profile)
	}
	return config.SaveFile(f)
}

// Path is the resolved config.toml location (exported for diagnostics/messages).
func Path() (string, error) {
	return config.ConfigPath(), nil
}
