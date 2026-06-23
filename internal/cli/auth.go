package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rootcause-org/replypen-cli/internal/client"
	"github.com/rootcause-org/replypen-cli/internal/config"
	"github.com/rootcause-org/replypen-cli/internal/render"
	"github.com/rootcause-org/replypen-cli/internal/token"
)

// asAPIError unwraps err into a *client.APIError, reporting whether it was one. The verbatim-surfacing
// path (printError) depends on this typed carrier.
func asAPIError(err error, target **client.APIError) bool {
	return errors.As(err, target)
}

// writeJSON pretty-prints a synthesized value for the -o json path (used by whoami and the local
// detect/id commands, which have no server body to pass through verbatim).
func writeJSON(e *env, v any) error {
	return render.Value(e.out, v)
}

// newLoginCmd builds `rp login` — writes a static-token profile. ReplyPen tokens are fixed strings (a
// super-admin token or a project-scoped `rpc_live_…`); the server decides the scope. The token + base URL
// are stored under the resolved profile (0600); every later `rp` reads them. --token is required here (it's
// the credential), so the persistent --token flag is reused as the value.
func newLoginCmd(e *env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store a static API token (and base URL) for a profile",
		Long: "Store a replypen static token. The token is either a super-admin token or a project-scoped " +
			"`rpc_live_…` token — the server resolves the scope on each request; the CLI just carries the bearer.\n\n" +
			"Stored per profile in " + tokenStorePath() + " (0600).",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if e.tokenOvr == "" {
				return fmt.Errorf("--token is required: rp login --token <tok> [--base-url <url>] [--profile <name>]")
			}
			res, err := config.Load(e.profile)
			if err != nil {
				return err
			}
			base := e.baseURLOvr // empty → the profile keeps whatever resolveBase would default to at use time
			if err := token.Save(res.Profile, token.Token{Token: e.tokenOvr, BaseURL: base}); err != nil {
				return err
			}
			fmt.Fprintf(e.out, "logged in — token stored for profile %q\n", res.Profile)
			if base != "" {
				fmt.Fprintf(e.err, "base URL: %s\n", base)
			}
			return nil
		},
	}
	return cmd
}

// newLogoutCmd builds `rp logout` — clears the profile's stored token + base URL.
func newLogoutCmd(e *env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Clear this profile's stored token",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			res, err := config.Load(e.profile)
			if err != nil {
				return err
			}
			_, ok, err := token.Load(res.Profile)
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintf(e.out, "already logged out (profile %q)\n", res.Profile)
				return nil
			}
			if err := token.Delete(res.Profile); err != nil {
				return err
			}
			fmt.Fprintf(e.out, "logged out (profile %q)\n", res.Profile)
			return nil
		},
	}
	return cmd
}

// newWhoamiCmd builds `rp whoami` — asks the SERVER to resolve the token's scope (admin vs project). Unlike
// rootcause's local-only whoami, ReplyPen exposes a /debug/whoami endpoint, so this verifies the token end
// to end.
func newWhoamiCmd(e *env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the resolved token scope (admin or project) — calls the server",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			c, err := e.newClient()
			if err != nil {
				return err
			}
			if e.jsonOut() {
				raw, err := c.Raw(e.ctx(), "GET", client.WhoamiPath(), nil, nil)
				if err != nil {
					return err
				}
				return render.JSON(e.out, raw)
			}
			who, err := c.Whoami(e.ctx())
			if err != nil {
				return err
			}
			render.Whoami(e.out, who)
			return nil
		},
	}
	return cmd
}

// tokenStorePath is the config path for help text (degrades to a generic label if unresolved).
func tokenStorePath() string {
	if p, err := token.Path(); err == nil {
		return p
	}
	return "~/.config/replypen/config.toml"
}
