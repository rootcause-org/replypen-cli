// Package debugdump turns one thread's /trace bundle into TWO local files built for progressive
// disclosure — a JSONL event log (the jq drill-down target) and a THIN markdown index (where to look).
// It mirrors rootcause-cli's debugdump: the calling agent reads the index, then jqs the JSONL, so the CLI
// never pre-summarizes a whole thread into an LLM's context.
//
// The JSONL contract is the load-bearing seam: line 1 is a {"type":"thread",…} header (thread metadata +
// the full triage/injection blobs + project/mailbox), every later line is one timeline entry keyed by its
// `type` (message|log|delivery|draft|note). Existing jq recipes (`select(.type=="delivery")`) keep working.
package debugdump

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/rootcause-org/replypen-cli/internal/client"
)

// Base returns the thread-derived file stem `<id8>-<slug>` used for both output filenames.
func Base(tr *client.Trace) string {
	id := tr.Thread.ID
	if len(id) > 8 {
		id = id[:8]
	}
	slug := tr.Project.Slug
	if slug == "" {
		slug = "thread"
	}
	return id + "-" + slug
}

// JSONLName / IndexName are the two output filenames for a bundle.
func JSONLName(tr *client.Trace) string { return Base(tr) + ".jsonl" }
func IndexName(tr *client.Trace) string { return Base(tr) + ".md" }

// EmitJSONL writes the drill-down event log: a {"type":"thread"} header line (thread metadata + the full
// triage/injection blobs + project/mailbox), followed by one line per timeline entry, every field FULL and
// untruncated. Header rollups are `thread_`-prefixed so entry-space jq queries never match the header.
func EmitJSONL(w io.Writer, tr *client.Trace) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	th := tr.Thread
	header := map[string]any{
		"type":                "thread",
		"thread_id":           th.ID,
		"external_thread_id":  emptyNil(th.ExternalThreadID),
		"subject":             emptyNil(th.Subject),
		"status":              th.Status,
		"channel":             emptyNil(th.Channel),
		"active_session_id":   emptyNil(th.ActiveSessionID),
		"pipeline_started_at": ptrNil(th.PipelineStartedAt),
		"pending_reentry":     th.PendingReentry,
		"triage_result":       rawOrNil(th.TriageResult),
		"injection_result":    rawOrNil(th.InjectionResult),
		"error_message":       ptrNil(th.ErrorMessage),
		"processor_failure":   rawOrNil(th.ProcessorFailure),
		"decline_reason":      ptrNil(th.DeclineReason),
		"created_at":          emptyNil(th.CreatedAt),
		"updated_at":          emptyNil(th.UpdatedAt),
		"project":             projectJSON(tr.Project),
		"mailbox":             mailboxJSON(tr.Mailbox),
		"counts": map[string]int{
			"messages":   len(tr.Messages),
			"logs":       len(tr.Logs),
			"deliveries": len(tr.Deliveries),
			"drafts":     len(tr.Drafts),
			"notes":      len(tr.Notes),
		},
	}
	if err := enc.Encode(header); err != nil {
		return err
	}
	for _, e := range tr.Timeline {
		line := map[string]any{
			"ts":     emptyNil(e.TS),
			"type":   e.Type,
			"label":  emptyNil(e.Label),
			"detail": emptyNil(e.Detail),
		}
		if err := enc.Encode(line); err != nil {
			return err
		}
	}
	return nil
}

// RenderIndex builds the THIN markdown index: the thread header, an outcome line (error/decline/draft
// status), the merged timeline table, and a Drill-down block of example jq calls. It is deliberately
// small — the JSONL is where the full detail lives; this file only says WHERE to look.
func RenderIndex(tr *client.Trace) string {
	th := tr.Thread
	jsonlName := JSONLName(tr)

	var L []string
	add := func(s ...string) { L = append(L, s...) }

	add(fmt.Sprintf("# Thread %s — %s · %s · %s", short8(th.ID), orQ(tr.Project.Slug), th.Status, orQ(th.Channel)), "")
	add(fmt.Sprintf("- **Thread ID:** `%s`", th.ID))
	if th.ExternalThreadID != "" {
		add(fmt.Sprintf("- **External id:** `%s`", th.ExternalThreadID))
	}
	if th.Subject != "" {
		add(fmt.Sprintf("- **Subject:** %s", cell(th.Subject, 120)))
	}
	if tr.Mailbox.Email != "" {
		add(fmt.Sprintf("- **Mailbox:** `%s` (%s)", tr.Mailbox.Email, orQ(tr.Mailbox.Provider)))
	}
	add(fmt.Sprintf("- **Session:** `%s`", orQ(th.ActiveSessionID)))
	add(fmt.Sprintf("- **Created / Updated:** %s / %s", orQ(th.CreatedAt), orQ(th.UpdatedAt)))
	add(fmt.Sprintf("- **Counts:** %d messages · %d logs · %d deliveries · %d drafts · %d notes",
		len(tr.Messages), len(tr.Logs), len(tr.Deliveries), len(tr.Drafts), len(tr.Notes)))
	add(fmt.Sprintf("- **Timeline (full, queryable):** `%s` — one JSON object per entry; jq it (see Drill down).", jsonlName), "")

	add("## Outcome", "")
	add(renderOutcome(tr)...)

	add("", "## Timeline", "")
	if len(tr.Timeline) == 0 {
		add("_(no timeline entries)_")
	} else {
		add("| ts | type | label | detail |", "|---|---|---|---|")
		for _, e := range tr.Timeline {
			add(fmt.Sprintf("| %s | %s | %s | %s |",
				cell(e.TS, 30), cell(e.Type, 12), cell(e.Label, 40), cell(e.Detail, 80)))
		}
	}

	add("", fmt.Sprintf("## Drill down — `%s`", jsonlName), "",
		"One JSON object per line: line 1 is the thread header, the rest are timeline entries keyed by `type`. "+
			"Pull the FULL detail with jq:", "",
		fence(strings.Join([]string{
			fmt.Sprintf(`jq -r 'select(.type=="delivery")' %s   # every outbound/callback delivery`, jsonlName),
			fmt.Sprintf(`jq -r 'select(.type=="log") | .label + " " + .detail' %s   # pipeline steps`, jsonlName),
			fmt.Sprintf(`jq -r 'select(.type=="draft" or .type=="note")' %s   # placed artifacts`, jsonlName),
			fmt.Sprintf(`jq -r 'select(.type=="thread").triage_result' %s   # the triage decision blob`, jsonlName),
		}, "\n"), "sh"))
	add("")
	return strings.Join(L, "\n")
}

// renderOutcome summarizes the headline "what happened": an error/decline/processor-failure line, then the
// latest draft + the notes count.
func renderOutcome(tr *client.Trace) []string {
	th := tr.Thread
	var out []string
	if s := ptrStr(th.ErrorMessage); s != "" {
		out = append(out, fmt.Sprintf("- **Errored:** %s", cell(s, 200)))
	}
	if s := rawStr(th.ProcessorFailure); s != "" {
		out = append(out, fmt.Sprintf("- **Processor failure:** %s", cell(s, 200)))
	}
	if s := ptrStr(th.DeclineReason); s != "" {
		out = append(out, fmt.Sprintf("- **Declined:** %s", cell(s, 200)))
	}
	if len(tr.Drafts) > 0 {
		d := tr.Drafts[len(tr.Drafts)-1]
		line := fmt.Sprintf("- **Draft:** %s", orQ(d.Status))
		if dr := ptrStr(d.DeletionReason); dr != "" {
			line += fmt.Sprintf(" (deleted: %s)", cell(dr, 80))
		}
		out = append(out, line)
	} else {
		out = append(out, "- **Draft:** none")
	}
	if len(tr.Notes) > 0 {
		out = append(out, fmt.Sprintf("- **Notes:** %d injected", len(tr.Notes)))
	}
	return out
}

// --- small JSON/format helpers ------------------------------------------------------------------------

func emptyNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func ptrNil(p *string) any {
	if p == nil || *p == "" {
		return nil
	}
	return *p
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// rawStr renders a json.RawMessage blob as a compact string for the markdown index (empty for null/absent).
func rawStr(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	return s
}

// rawOrNil carries a nested JSON blob through verbatim (so triage_result/injection_result keep their true
// shape in the JSONL), nilling an absent/empty/null one.
func rawOrNil(raw json.RawMessage) any {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil
	}
	return raw
}

func projectJSON(p client.TraceProject) map[string]any {
	return map[string]any{
		"id": emptyNil(p.ID), "slug": emptyNil(p.Slug),
		"tenant_codename": emptyNil(p.TenantCodename), "webhook_url": emptyNil(p.WebhookURL),
	}
}

func mailboxJSON(m client.TraceMailbox) map[string]any {
	return map[string]any{
		"id": emptyNil(m.ID), "email": emptyNil(m.Email),
		"provider": emptyNil(m.Provider), "status": emptyNil(m.Status),
	}
}

// cell renders one markdown table cell: single line, pipes escaped, truncated.
func cell(text string, limit int) string {
	line := strings.ReplaceAll(strings.Join(strings.Fields(text), " "), "|", "\\|")
	return truncate(line, limit)
}

func truncate(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit-1]) + "…"
}

// fence wraps content in a backtick fence, bumping the run so embedded backticks can't break out.
func fence(text, lang string) string {
	body := strings.TrimRight(text, "\n")
	if body == "" {
		return "_(empty)_"
	}
	longest := 0
	for _, run := range backtickRuns(body) {
		if run > longest {
			longest = run
		}
	}
	ticks := strings.Repeat("`", maxInt(3, longest+1))
	return ticks + lang + "\n" + body + "\n" + ticks
}

func backtickRuns(s string) []int {
	var runs []int
	run := 0
	for _, ch := range s {
		if ch == '`' {
			run++
		} else if run > 0 {
			runs = append(runs, run)
			run = 0
		}
	}
	if run > 0 {
		runs = append(runs, run)
	}
	return runs
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func short8(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func orQ(s string) string {
	if s == "" {
		return "?"
	}
	return s
}
