package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/rootcause-org/replypen-cli/internal/client"
	"github.com/rootcause-org/replypen-cli/internal/debugdump"
	"github.com/rootcause-org/replypen-cli/internal/render"
)

// newThreadCmd builds `rp thread` with the `trace` subcommand.
func newThreadCmd(e *env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "thread",
		Short: "Thread debugging commands",
	}
	cmd.AddCommand(newThreadTraceCmd(e))
	return cmd
}

// newThreadTraceCmd builds `rp thread trace <id>` — the full assembled trace bundle for one thread (the
// replacement for trace_thread.py). {id} accepts a replypen UUID or an external/provider id.
//
// Default: render the merged timeline as a readable table. --md writes a status-page-style markdown index,
// --jsonl writes a jq-able event log (line 1 a `{"type":"thread"…}` header, each later line one timeline
// entry keyed by `type`); both print the written file paths. -o json = the verbatim server bundle.
func newThreadTraceCmd(e *env) *cobra.Command {
	var asMD, asJSONL bool
	var outDir string
	cmd := &cobra.Command{
		Use:   "trace <id>",
		Short: "Trace a thread: the full assembled bundle (replaces trace_thread.py)",
		Long: "Fetch the full assembled trace bundle for one thread and render its merged timeline.\n\n" +
			"<id> accepts a replypen thread UUID OR an external/provider thread id.\n\n" +
			"--md writes a status-page-style markdown index, --jsonl writes a jq-able event log (one JSON " +
			"object per line). -o json emits the verbatim server bundle for piping to jq.",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			id := args[0]
			c, err := e.newClient()
			if err != nil {
				return err
			}

			// File decomposition (--md/--jsonl) needs the typed bundle; so does the table view.
			if asMD || asJSONL {
				tr, err := c.Trace(e.ctx(), id)
				if err != nil {
					return err
				}
				return writeTraceFiles(e, tr, outDir, asMD, asJSONL)
			}

			if e.jsonOut() {
				raw, err := c.Raw(e.ctx(), "GET", client.TracePath(id), nil, nil)
				if err != nil {
					return err
				}
				return render.JSON(e.out, raw)
			}

			tr, err := c.Trace(e.ctx(), id)
			if err != nil {
				return err
			}
			render.Trace(e.out, tr)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asMD, "md", false, "write a markdown index of the timeline")
	cmd.Flags().BoolVar(&asJSONL, "jsonl", false, "write a jq-able JSONL event log of the timeline")
	cmd.Flags().StringVar(&outDir, "out-dir", ".replypen/debug", "directory for --md/--jsonl output files")
	return cmd
}

// writeTraceFiles writes the requested decomposition file(s) into outDir and prints their paths (index
// first, then jsonl — matching rootcause-cli's order). Paths go to stdout so a script can capture them.
func writeTraceFiles(e *env, tr *client.Trace, outDir string, asMD, asJSONL bool) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create out-dir %s: %w", outDir, err)
	}
	if asMD {
		path := filepath.Join(outDir, debugdump.IndexName(tr))
		if err := os.WriteFile(path, []byte(debugdump.RenderIndex(tr)), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Fprintln(e.out, path)
	}
	if asJSONL {
		path := filepath.Join(outDir, debugdump.JSONLName(tr))
		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		if err := debugdump.EmitJSONL(f, tr); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		fmt.Fprintln(e.out, path)
	}
	return nil
}
