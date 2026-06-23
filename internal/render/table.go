// This file holds the human-facing table renderers — one per command view. They are pure functions of the
// wire structs (no I/O beyond the passed writer, no clock) so golden tests pin them exactly. Timestamps
// are shown as the server sent them (never time.Now), keeping goldens stable.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/rootcause-org/replypen-cli/internal/client"
)

// Whoami prints the resolved token scope: admin (all projects) or a single project.
func Whoami(w io.Writer, who *client.Whoami) {
	fmt.Fprintf(w, "Scope: %s\n", who.Scope)
	if who.Scope == "project" {
		fmt.Fprintf(w, "Project: %s\n", deref(who.ProjectSlug))
		fmt.Fprintf(w, "Tenant:  %s\n", deref(who.TenantCodename))
	}
}

// Projects renders the projects table: slug, tenant, mailboxes, webhook_url, created.
func Projects(w io.Writer, resp *client.ProjectsResponse) {
	if len(resp.Projects) == 0 {
		fmt.Fprintln(w, "No projects.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SLUG\tTENANT\tMAILBOXES\tWEBHOOK_URL\tCREATED")
	for _, p := range resp.Projects {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n",
			p.Slug, p.TenantCodename, p.MailboxCount, p.WebhookURL, p.CreatedAt)
	}
	tw.Flush()
}

// Threads renders the recent-threads table for a project (newest first).
func Threads(w io.Writer, resp *client.ThreadsResponse) {
	if len(resp.Threads) == 0 {
		fmt.Fprintln(w, "No threads.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tCHANNEL\tSENDER\tSUBJECT\tUPDATED")
	for _, t := range resp.Threads {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			shortID(t.ID), t.Status, t.Channel, t.Sender, truncate(t.Subject, 50), t.UpdatedAt)
	}
	tw.Flush()
}

// Triage renders the triage-decision table: the last N inbound threads + their triage verdict.
func Triage(w io.Writer, resp *client.TriageResponse) {
	if len(resp.Decisions) == 0 {
		fmt.Fprintln(w, "No triage decisions.")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RECEIVED\tSENDER\tSUBJECT\tPROCESS\tCATEGORY\tCONF\tREASON")
	for _, d := range resp.Decisions {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%.2f\t%s\n",
			deref(d.ReceivedAt), d.Sender, truncate(d.Subject, 40), yesNo(d.ShouldProcess),
			d.Category, d.Confidence, truncate(d.Reason, 50))
	}
	tw.Flush()
}

// Trace renders the merged thread timeline as a readable table preceded by a small header block (the
// thread's id/subject/status and any error/decline). The raw sub-arrays remain reachable via -o json.
func Trace(w io.Writer, tr *client.Trace) {
	th := tr.Thread
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Thread:\t%s\n", th.ID)
	if th.Subject != "" {
		fmt.Fprintf(tw, "Subject:\t%s\n", th.Subject)
	}
	fmt.Fprintf(tw, "Status:\t%s\n", th.Status)
	fmt.Fprintf(tw, "Channel:\t%s\n", th.Channel)
	if tr.Mailbox.Email != "" {
		fmt.Fprintf(tw, "Mailbox:\t%s (%s)\n", tr.Mailbox.Email, tr.Mailbox.Provider)
	}
	if tr.Project.Slug != "" {
		fmt.Fprintf(tw, "Project:\t%s\n", tr.Project.Slug)
	}
	if th.ActiveSessionID != "" {
		fmt.Fprintf(tw, "Session:\t%s\n", th.ActiveSessionID)
	}
	if s := deref(th.ErrorMessage); s != "" {
		fmt.Fprintf(tw, "Error:\t%s\n", s)
	}
	if s := rawStr(th.ProcessorFailure); s != "" {
		fmt.Fprintf(tw, "Processor failure:\t%s\n", oneLine(s))
	}
	if s := deref(th.DeclineReason); s != "" {
		fmt.Fprintf(tw, "Declined:\t%s\n", s)
	}
	tw.Flush()

	fmt.Fprintf(w, "\nTimeline (%d):\n", len(tr.Timeline))
	if len(tr.Timeline) == 0 {
		fmt.Fprintln(w, "  (no timeline entries)")
		return
	}
	ttw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(ttw, "  TS\tTYPE\tLABEL\tDETAIL")
	for _, e := range tr.Timeline {
		fmt.Fprintf(ttw, "  %s\t%s\t%s\t%s\n", e.TS, e.Type, e.Label, truncate(oneLine(e.Detail), 70))
	}
	ttw.Flush()
}

// --- small formatting helpers -------------------------------------------------------------------------

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// rawStr renders a json.RawMessage blob as a compact one-liner for the table (empty for null/absent).
func rawStr(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func oneLine(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", "")
}

// truncate clamps s to at most max runes, appending an ellipsis when it had to cut.
func truncate(s string, max int) string {
	s = oneLine(s)
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimRight(string(r[:max]), " ") + "…"
}
