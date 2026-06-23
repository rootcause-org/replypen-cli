package cli

import (
	"encoding/csv"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/rootcause-org/replypen-cli/internal/client"
	"github.com/rootcause-org/replypen-cli/internal/render"
)

// newProjectsCmd builds `rp projects` — list the projects visible to the token (all for admin, the caller's
// one for a project-scoped token).
func newProjectsCmd(e *env) *cobra.Command {
	return &cobra.Command{
		Use:   "projects",
		Short: "List projects visible to this token",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			c, err := e.newClient()
			if err != nil {
				return err
			}
			if e.jsonOut() {
				raw, err := c.Raw(e.ctx(), "GET", client.ProjectsPath(), nil, nil)
				if err != nil {
					return err
				}
				return render.JSON(e.out, raw)
			}
			resp, err := c.Projects(e.ctx())
			if err != nil {
				return err
			}
			render.Projects(e.out, resp)
			return nil
		},
	}
}

// newThreadsCmd builds `rp threads <slug>` — recent threads for a project, newest first, with an optional
// status filter.
func newThreadsCmd(e *env) *cobra.Command {
	var limit int
	var status string
	cmd := &cobra.Command{
		Use:   "threads <slug>",
		Short: "List recent threads for a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			slug := args[0]
			c, err := e.newClient()
			if err != nil {
				return err
			}
			if e.jsonOut() {
				raw, err := c.Raw(e.ctx(), "GET", client.ThreadsPath(slug, limit, status), nil, nil)
				if err != nil {
					return err
				}
				return render.JSON(e.out, raw)
			}
			resp, err := c.Threads(e.ctx(), slug, limit, status)
			if err != nil {
				return err
			}
			render.Threads(e.out, resp)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "max threads to return")
	cmd.Flags().StringVar(&status, "status", "", "filter by thread status")
	return cmd
}

// newTriageCmd builds `rp triage <slug>` — the last N inbound threads + their triage decision (the
// replacement for triage_logs.py). --csv emits the decisions as CSV (the spec's exact column order)
// regardless of -o, so it pipes straight to a spreadsheet.
func newTriageCmd(e *env) *cobra.Command {
	var limit int
	var asCSV bool
	cmd := &cobra.Command{
		Use:   "triage <slug>",
		Short: "Recent inbound threads + their triage decision (replaces triage_logs.py)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			slug := args[0]
			c, err := e.newClient()
			if err != nil {
				return err
			}
			// --csv needs the typed rows; everything else can passthrough on -o json.
			if e.jsonOut() && !asCSV {
				raw, err := c.Raw(e.ctx(), "GET", client.TriagePath(slug, limit), nil, nil)
				if err != nil {
					return err
				}
				return render.JSON(e.out, raw)
			}
			resp, err := c.Triage(e.ctx(), slug, limit)
			if err != nil {
				return err
			}
			if asCSV {
				return writeTriageCSV(e, resp)
			}
			render.Triage(e.out, resp)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "max decisions to return")
	cmd.Flags().BoolVar(&asCSV, "csv", false, "emit the decisions as CSV (spec column order)")
	return cmd
}

// writeTriageCSV writes the decisions in the spec's exact column order:
// received_at,sender,subject,thread_status,should_process,category,confidence,reason,thread_id,mailbox.
func writeTriageCSV(e *env, resp *client.TriageResponse) error {
	w := csv.NewWriter(e.out)
	if err := w.Write([]string{
		"received_at", "sender", "subject", "thread_status", "should_process",
		"category", "confidence", "reason", "thread_id", "mailbox",
	}); err != nil {
		return err
	}
	for _, d := range resp.Decisions {
		if err := w.Write([]string{
			derefStr(d.ReceivedAt), d.Sender, d.Subject, d.ThreadStatus,
			strconv.FormatBool(d.ShouldProcess), d.Category,
			strconv.FormatFloat(d.Confidence, 'f', -1, 64), d.Reason, d.ThreadID, d.Mailbox,
		}); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("write csv: %w", err)
	}
	return nil
}

// derefStr renders a nullable string field as its value, empty for null.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
