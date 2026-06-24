package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rootcause-org/replypen-cli/internal/client"
	"github.com/rootcause-org/replypen-cli/internal/config"
	"github.com/rootcause-org/replypen-cli/internal/render"
)

// newProjectCmd builds `rp project` with the `mint-token` subcommand (admin-only token minting).
func newProjectCmd(e *env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Project management commands",
	}
	cmd.AddCommand(newProjectMintTokenCmd(e), newProjectCreateCmd(e))
	return cmd
}

// newProjectMintTokenCmd builds `rp project mint-token <slug>` — mint/rotate a project-scoped CLI token
// (admin scope). The server returns it ONCE (stores only the hash), so the command prints it plainly.
func newProjectMintTokenCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "mint-token <slug>",
		Short: "Mint a project-scoped CLI token (admin only; shown once)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			slug := args[0]
			c, err := e.newClient()
			if err != nil {
				return err
			}
			if e.jsonOut() {
				raw, err := c.Raw(e.ctx(), "POST", client.MintTokenPath(slug), nil, nil)
				if err != nil {
					return err
				}
				return render.JSON(e.out, raw)
			}
			resp, err := c.MintToken(e.ctx(), slug)
			if err != nil {
				return err
			}
			fmt.Fprintf(e.out, "project: %s\n", resp.ProjectSlug)
			if resp.TenantCodename != "" {
				fmt.Fprintf(e.out, "tenant:  %s\n", resp.TenantCodename)
			}
			fmt.Fprintf(e.out, "token:   %s\n", resp.Token)
			fmt.Fprintln(e.err, "store this now — it is shown once and only the hash is kept server-side")
			return nil
		},
	}
}

// newProjectCreateCmd builds `rp project create` — POST /api/v1/projects, bearer = the tenant API token
// passed via --token. It's an onboarding wrapper over an existing endpoint, so the bearer here is the
// tenant token, not a debug token.
func newProjectCreateCmd(e *env) *cobra.Command {
	var name, webhookURL, triageModel string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a project (onboarding; --token = tenant API token)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if name == "" || webhookURL == "" {
				return fmt.Errorf("--name and --webhook-url are required")
			}
			c, err := e.newClient()
			if err != nil {
				return err
			}
			req := client.ProjectCreateRequest{Name: name, WebhookURL: webhookURL, TriageModel: triageModel}
			resp, raw, err := c.CreateProject(e.ctx(), req)
			if err != nil {
				return err
			}
			if e.jsonOut() {
				return render.JSON(e.out, raw)
			}
			fmt.Fprintf(e.out, "project: %s (%s)\n", resp.Slug, resp.Name)
			fmt.Fprintf(e.out, "webhook_url:    %s\n", resp.WebhookURL)
			fmt.Fprintf(e.out, "webhook_secret: %s\n", resp.WebhookSecret)
			fmt.Fprintln(e.err, "store the webhook_secret now — it signs the outbound/callback HMAC and is shown once")
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "project name (required)")
	cmd.Flags().StringVar(&webhookURL, "webhook-url", "", "processor webhook URL (required)")
	cmd.Flags().StringVar(&triageModel, "triage-model", "", "override the triage model")
	return cmd
}

// newTenantCmd builds `rp tenant` with the `register` + `mint-token` subcommands.
func newTenantCmd(e *env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tenant",
		Short: "Tenant management commands",
	}
	cmd.AddCommand(newTenantRegisterCmd(e), newTenantMintTokenCmd(e))
	return cmd
}

// newTenantMintTokenCmd builds `rp tenant mint-token <codename>` — mint/rotate a tenant-scoped CLI token
// (admin scope) that sees every project under the tenant. Mirrors `rp project mint-token`: the server
// returns the `rpt_live_…` token ONCE (stores only the hash), so the command prints it plainly.
func newTenantMintTokenCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "mint-token <codename>",
		Short: "Mint a tenant-scoped CLI token (admin only; shown once)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			codename := args[0]
			c, err := e.newClient()
			if err != nil {
				return err
			}
			if e.jsonOut() {
				raw, err := c.Raw(e.ctx(), "POST", client.MintTenantTokenPath(codename), nil, nil)
				if err != nil {
					return err
				}
				return render.JSON(e.out, raw)
			}
			resp, err := c.MintTenantToken(e.ctx(), codename)
			if err != nil {
				return err
			}
			fmt.Fprintf(e.out, "tenant: %s\n", resp.TenantCodename)
			fmt.Fprintf(e.out, "token:  %s\n", resp.Token)
			fmt.Fprintln(e.err, "store this now — it is shown once and only the hash is kept server-side")
			return nil
		},
	}
}

// newTenantRegisterCmd builds `rp tenant register` — POST /api/v1/register with the X-Admin-Secret header.
// This endpoint takes the admin secret (not a bearer), so it bypasses the token store entirely: the secret
// comes from --admin-secret or REPLYPEN_ADMIN_SECRET. The base URL still follows the normal precedence.
func newTenantRegisterCmd(e *env) *cobra.Command {
	var codename, adminSecret string
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a tenant (onboarding; X-Admin-Secret auth)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if codename == "" {
				return fmt.Errorf("--codename is required")
			}
			if adminSecret == "" {
				adminSecret = os.Getenv("REPLYPEN_ADMIN_SECRET")
			}
			if adminSecret == "" {
				return fmt.Errorf("--admin-secret (or REPLYPEN_ADMIN_SECRET) is required")
			}
			// The register endpoint authenticates with the admin secret header, not a bearer — build a
			// token-less client so newClient's "not logged in" gate doesn't fire.
			c := e.adminClient()
			resp, raw, err := c.Register(e.ctx(), adminSecret, client.RegisterRequest{Codename: codename})
			if err != nil {
				return err
			}
			if e.jsonOut() {
				return render.JSON(e.out, raw)
			}
			fmt.Fprintf(e.out, "tenant: %s\n", resp.Codename)
			fmt.Fprintf(e.out, "api_token: %s\n", resp.APIToken)
			fmt.Fprintln(e.err, "store the api_token now — it is the tenant's bearer for `rp project create`")
			return nil
		},
	}
	cmd.Flags().StringVar(&codename, "codename", "", "tenant codename (required)")
	cmd.Flags().StringVar(&adminSecret, "admin-secret", "", "admin secret (or REPLYPEN_ADMIN_SECRET)")
	return cmd
}

// newMailboxCmd builds `rp mailbox` with the `connect` subcommand.
func newMailboxCmd(e *env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mailbox",
		Short: "Mailbox management commands",
	}
	cmd.AddCommand(newMailboxConnectCmd(e))
	return cmd
}

// newMailboxConnectCmd builds `rp mailbox connect` — POST /api/v1/projects/{slug}/mailboxes/connect, bearer
// = the tenant API token via --token. It prints the returned oauth_url the operator opens to grant access.
func newMailboxConnectCmd(e *env) *cobra.Command {
	var slug, email, provider string
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Connect a mailbox to a project (onboarding; prints the oauth_url)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if slug == "" || provider == "" {
				return fmt.Errorf("--slug and --provider are required")
			}
			switch provider {
			case "google", "microsoft", "intercom":
			default:
				return fmt.Errorf("--provider must be one of google|microsoft|intercom (got %q)", provider)
			}
			c, err := e.newClient()
			if err != nil {
				return err
			}
			req := client.MailboxConnectRequest{Email: email, Provider: provider}
			resp, raw, err := c.ConnectMailbox(e.ctx(), slug, req)
			if err != nil {
				return err
			}
			if e.jsonOut() {
				return render.JSON(e.out, raw)
			}
			fmt.Fprintf(e.out, "oauth_url: %s\n", resp.OAuthURL)
			fmt.Fprintln(e.err, "open the oauth_url to grant mailbox access; the connection completes server-side")
			return nil
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "project slug (required)")
	cmd.Flags().StringVar(&email, "email", "", "mailbox email (optional)")
	cmd.Flags().StringVar(&provider, "provider", "", "provider: google|microsoft|intercom (required)")
	return cmd
}

// adminClient builds a token-less client for the admin-secret-authenticated register endpoint. The base URL
// follows --base-url, else config.Load's resolution (REPLYPEN_BASE_URL > stored profile base_url > default).
// No token is needed: the admin secret rides in the X-Admin-Secret header.
func (e *env) adminClient() *client.Client {
	base := e.baseURLOvr
	if base == "" {
		if res, err := config.Load(e.profile); err == nil {
			base = res.BaseURL // always non-empty (falls back to the built-in default)
		} else {
			base = config.DefaultBaseURL
		}
	}
	return client.New(base, client.StaticToken(""))
}
