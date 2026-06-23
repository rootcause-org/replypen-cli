// Package cli is the command surface: it wires cobra commands onto the client + render layers, holds the
// global flags (--profile, --base-url, --token, -o/--output), and owns the cross-cutting concerns the rest
// of the CLI must not repeat — building an authenticated client from resolved config and printing API
// errors verbatim. Each command file is a thin adapter: parse flags → one client call → render. No
// business logic lives here.
package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/rootcause-org/replypen-cli/internal/client"
	"github.com/rootcause-org/replypen-cli/internal/config"
	"github.com/rootcause-org/replypen-cli/internal/render"
	"github.com/rootcause-org/replypen-cli/internal/token"
)

// env carries the shared, testable state through commands: the global flag values plus the writers. Tests
// inject baseURL/output/token and capture out/err here instead of relying on TTY detection or a real
// token store.
type env struct {
	profile    string // --profile: which stored profile to use (default "default")
	baseURLOvr string // --base-url: explicit base URL override (also the test seam)
	tokenOvr   string // --token: explicit static bearer; bypasses the stored profile token
	output     string // "", "json", or "table" (from -o/--output)

	out io.Writer
	err io.Writer
	in  io.Reader // stdin (reserved for any future interactive prompt)
}

// Execute is the binary entrypoint. It returns the process exit code so main stays trivial; any command
// error (including a typed APIError) is printed to stderr here, once.
func Execute(version string) int {
	e := &env{out: os.Stdout, err: os.Stderr, in: os.Stdin}
	root := newRootCmd(e, version)
	if err := root.Execute(); err != nil {
		printError(e.err, err)
		return 1
	}
	return 0
}

// newRootCmd assembles the root command + global flags + subcommands. Split out so tests can build a root
// against an in-memory env and a stub server.
func newRootCmd(e *env, version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "rp",
		Short:         "replypen CLI — a scriptable static-token client over the replypen debug API",
		Version:       version,
		SilenceUsage:  true, // a runtime error isn't a usage error; don't dump help on it
		SilenceErrors: true, // Execute prints the error itself, verbatim
	}
	root.PersistentFlags().StringVar(&e.profile, "profile", "", "config profile to use (default: \"default\")")
	root.PersistentFlags().StringVar(&e.baseURLOvr, "base-url", "", "API base URL (overrides env + stored profile)")
	root.PersistentFlags().StringVar(&e.tokenOvr, "token", "", "static bearer token (overrides env + stored profile)")
	root.PersistentFlags().StringVarP(&e.output, "output", "o", "", "output format: json|table (default: auto-detect)")

	root.AddCommand(
		newLoginCmd(e),
		newLogoutCmd(e),
		newWhoamiCmd(e),
		newProjectsCmd(e),
		newThreadsCmd(e),
		newTriageCmd(e),
		newThreadCmd(e),
		newProjectCmd(e),
		newTenantCmd(e),
		newMailboxCmd(e),
		newProviderCmd(e),
		newIDCmd(e),
		newUpgradeCmd(e, version),
	)
	return root
}

// jsonOut reports whether output should be JSON for the current mode + destination.
func (e *env) jsonOut() bool { return render.IsJSON(e.mode(), e.out) }

// mode maps the -o/--output flag to a render.Mode (empty → auto-detect from the destination).
func (e *env) mode() render.Mode {
	switch e.output {
	case "json":
		return render.ModeJSON
	case "table":
		return render.ModeTable
	default:
		return render.ModeAuto
	}
}

// resolveBase returns the effective base URL: --base-url > REPLYPEN_BASE_URL > stored profile base_url >
// built-in default. It also returns whether we fell back to the built-in default (to warn).
//
// res.BaseURL already encodes env > stored profile base_url > default (see config.Load), and the stored
// profile base_url IS the token's pinned base (token.Save persists it as the same profile field). So once
// --base-url is ruled out, res.BaseURL is authoritative — re-checking tok.BaseURL here would wrongly jump
// the pinned base ahead of REPLYPEN_BASE_URL, inverting the spec's precedence.
func (e *env) resolveBase(res config.Resolved) (string, bool) {
	if e.baseURLOvr != "" {
		return e.baseURLOvr, false
	}
	return res.BaseURL, res.BaseURLFromDefault
}

// newClient resolves config + the static token and builds an authenticated client. The token precedence is
// --token > REPLYPEN_TOKEN > stored profile token; it errors with a "run `rp login`" prompt when none
// resolves. The base URL follows resolveBase.
func (e *env) newClient() (*client.Client, error) {
	res, err := config.Load(e.profile)
	if err != nil {
		return nil, err
	}

	// Explicit --token wins and needs no store.
	if e.tokenOvr != "" {
		base, fromDefault := e.resolveBase(res)
		e.warnDefaultBase(base, fromDefault)
		return client.New(base, client.StaticToken(e.tokenOvr)), nil
	}

	// REPLYPEN_TOKEN env is the second source.
	if envTok := os.Getenv("REPLYPEN_TOKEN"); envTok != "" {
		base, fromDefault := e.resolveBase(res)
		e.warnDefaultBase(base, fromDefault)
		return client.New(base, client.StaticToken(envTok)), nil
	}

	tok, ok, err := token.Load(res.Profile)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, notLoggedIn(res.Profile)
	}
	base, fromDefault := e.resolveBase(res)
	e.warnDefaultBase(base, fromDefault)
	return client.New(base, client.StaticToken(tok.Token)), nil
}

// warnDefaultBase warns (to stderr, never stdout — so piped output stays clean) when a command is about to
// hit the localhost default because nothing set a base URL.
func (e *env) warnDefaultBase(base string, fromDefault bool) {
	if fromDefault {
		fmt.Fprintf(e.err, "warning: no base URL set; defaulting to %s — set REPLYPEN_BASE_URL or run `rp login --base-url <url>`\n", base)
	}
}

// notLoggedIn is the clear "no token for this profile" error naming the one-command fix.
func notLoggedIn(profile string) error {
	return fmt.Errorf("not logged in (profile %q) — run `rp login --token <tok> [--base-url <url>]`", profile)
}

// ctx is the per-command context — one place to add a timeout/signal later without touching each command.
func (e *env) ctx() context.Context { return context.Background() }

// printError renders any command error to stderr. A JSON-envelope APIError is surfaced verbatim
// (CODE: message). A no-envelope APIError (a plain-text non-2xx) gets method + path + status text + base
// URL so the user sees WHAT was hit WHERE, with a pointed hint for the common 404/405.
func printError(w io.Writer, err error) {
	var apiErr *client.APIError
	if asAPIError(err, &apiErr) {
		if apiErr.Code == "" {
			printNonEnvelopeHTTPError(w, apiErr)
			return
		}
		fmt.Fprintf(w, "%s: %s\n", apiErr.Code, apiErr.Message)
		return
	}
	fmt.Fprintf(w, "error: %s\n", err.Error())
}

// printNonEnvelopeHTTPError renders a non-2xx with no decodable error envelope.
func printNonEnvelopeHTTPError(w io.Writer, e *client.APIError) {
	statusText := http.StatusText(e.Status)
	if e.Method != "" && e.Path != "" {
		fmt.Fprintf(w, "error: %s %s → HTTP %d %s\n", e.Method, e.Path, e.Status, statusText)
	} else {
		fmt.Fprintf(w, "error: HTTP %d %s\n", e.Status, statusText)
	}
	switch e.Status {
	case http.StatusMethodNotAllowed:
		fmt.Fprintln(w, "  endpoint not available on this server — it may be older than this CLI")
	case http.StatusNotFound:
		fmt.Fprintln(w, "  not found — check the id/path, or the endpoint may not be available on this server")
	}
	if e.BaseURL != "" {
		fmt.Fprintf(w, "  base URL: %s\n", e.BaseURL)
	}
}
