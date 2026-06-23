package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rootcause-org/replypen-cli/internal/dnsdetect"
	"github.com/rootcause-org/replypen-cli/internal/idutil"
)

// These commands are fully LOCAL: no token, no base URL, no network beyond DNS (for `provider detect`).
// They are pure functions over their input, so they never call newClient.

// newProviderCmd builds `rp provider detect <domain|email>` — classify the email backend from public DNS
// and report whether replypen can onboard it. The DNS resolver is the live one here; the logic lives in
// internal/dnsdetect (unit-tested offline with a fake resolver).
func newProviderCmd(e *env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Provider (channel) utilities",
	}
	detect := &cobra.Command{
		Use:   "detect <domain|email> [more…]",
		Short: "Detect a domain's email backend (google/microsoft/other) from DNS",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			r := dnsdetect.NewNetResolver()
			results := make([]dnsdetect.Result, 0, len(args))
			for _, target := range args {
				results = append(results, dnsdetect.Detect(e.ctx(), r, target))
			}
			if e.jsonOut() {
				// One result → a bare object; many → an array (mirrors the Python script's JSON shape).
				if len(results) == 1 {
					return writeJSON(e, results[0])
				}
				return writeJSON(e, results)
			}
			for i, r := range results {
				if i > 0 {
					fmt.Fprintln(e.out)
				}
				printDetect(e, r)
			}
			return nil
		},
	}
	cmd.AddCommand(detect)
	return cmd
}

func printDetect(e *env, r dnsdetect.Result) {
	mark := "NOT SUPPORTED"
	if r.Supported {
		mark = "SUPPORTED"
	}
	fmt.Fprintf(e.out, "=== %s  (%s) ===\n", r.Input, r.Domain)
	fmt.Fprintf(e.out, "  provider   : %s\n", r.Provider)
	fmt.Fprintf(e.out, "  replypen   : %s\n", mark)
	fmt.Fprintf(e.out, "  confidence : %s\n", r.Confidence)
	if len(r.MX) > 0 {
		fmt.Fprintf(e.out, "  mx         : %s\n", joinComma(r.MX))
	}
	if r.SPF != "" {
		fmt.Fprintf(e.out, "  spf        : %s\n", r.SPF)
	}
	for _, s := range r.Signals {
		fmt.Fprintf(e.out, "  - %s\n", s)
	}
	for _, n := range r.Notes {
		fmt.Fprintf(e.out, "  ! %s\n", n)
	}
}

// newIDCmd builds `rp id gmail <id>` / `rp id outlook <id>` — id translators ported from gmail_ids.py /
// outlook_ids.py. Pure math, no network.
func newIDCmd(e *env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "id",
		Short: "Translate provider message/thread ids",
	}
	cmd.AddCommand(newIDGmailCmd(e), newIDOutlookCmd(e))
	return cmd
}

func newIDGmailCmd(e *env) *cobra.Command {
	var user string
	gmail := &cobra.Command{
		Use:   "gmail <id>",
		Short: "Translate Gmail hex/decimal/thread-f: ids + build a clickable URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			res := idutil.ClassifyGmailForUser(args[0], user)
			if e.jsonOut() {
				return writeJSON(e, res)
			}
			printGmailID(e, res)
			return nil
		},
	}
	gmail.Flags().StringVar(&user, "user", "0", "Gmail /u/N/ mailbox index for the web URL")
	return gmail
}

func printGmailID(e *env, r idutil.GmailResult) {
	fmt.Fprintf(e.out, "input      : %s\n", r.Input)
	detected := r.Kind
	if r.Note != "" {
		detected += "  (" + r.Note + ")"
	}
	fmt.Fprintf(e.out, "detected   : %s\n", detected)
	if r.Hex == "" {
		return
	}
	fmt.Fprintf(e.out, "\napi hex    : %s\n", r.Hex)
	fmt.Fprintf(e.out, "decimal    : %d\n", r.Decimal)
	fmt.Fprintf(e.out, "thread-f:  : %s\n", r.ThreadF)
	fmt.Fprintf(e.out, "msg-f:     : %s\n", r.MsgF)
	fmt.Fprintf(e.out, "\nweb URL    : %s\n", r.WebURL)
}

func newIDOutlookCmd(e *env) *cobra.Command {
	outlook := &cobra.Command{
		Use:   "outlook <id>",
		Short: "Classify an Outlook/Graph id + tell you which DB column matches it",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			res := idutil.ClassifyOutlook(args[0])
			if e.jsonOut() {
				return writeJSON(e, res)
			}
			printOutlookID(e, res)
			return nil
		},
	}
	return outlook
}

func printOutlookID(e *env, r idutil.OutlookResult) {
	fmt.Fprintf(e.out, "input      : %s\n", r.Input)
	if r.URLID != "" && r.URLID != r.Input {
		fmt.Fprintf(e.out, "url id     : %s\n", r.URLID)
	}
	fmt.Fprintf(e.out, "detected   : %s\n", r.Kind)
	fmt.Fprintf(e.out, "note       : %s\n", r.Note)
	if r.EmbeddedGUID != "" {
		fmt.Fprintf(e.out, "guid       : %s\n", r.EmbeddedGUID)
	}
	if r.MatchColumn != "" {
		fmt.Fprintf(e.out, "\nlookup     : SELECT … WHERE %s = '%s'\n", r.MatchColumn, r.MatchValue)
	} else {
		fmt.Fprintln(e.out, "\nlookup     : (not offline-resolvable — see note above)")
	}
}

func joinComma(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}
