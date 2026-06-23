// Package render is the output layer: it turns wire structs into either a human table or the raw API
// JSON, and decides which by default. The INTENT is pipe-first ergonomics — a TTY gets a readable table,
// a pipe/redirect gets JSON so `| jq` always works — with a global -o/--output flag to force either. JSON
// mode is a VERBATIM pretty-print (no reshaping), so the CLI can never invent or drop a field on the jq
// path.
package render

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
)

// Mode is the output format. ModeAuto resolves to table on a TTY, JSON otherwise.
type Mode string

const (
	ModeAuto  Mode = ""      // unset: detect from the destination
	ModeTable Mode = "table" // human, columnar
	ModeJSON  Mode = "json"  // raw API JSON, pretty-printed
)

// IsJSON resolves Mode against the destination: an explicit flag wins; otherwise JSON unless w is a TTY.
func IsJSON(mode Mode, w io.Writer) bool {
	switch mode {
	case ModeJSON:
		return true
	case ModeTable:
		return false
	}
	return !isTerminal(w)
}

// IsTerminal reports whether w is a real TTY (exported for callers gating transient progress output).
func IsTerminal(w io.Writer) bool { return isTerminal(w) }

// isTerminal reports whether w is a character device (a TTY). A non-*os.File writer (a test buffer) is
// treated as "not a terminal" → JSON, the safe scriptable default.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// JSON pretty-prints raw API bytes to w (2-space indent, trailing newline), emitted as the server sent
// them — re-indenting is the only transform, so jq sees the true response shape. It re-indents the RAW
// token stream via json.Indent (never decoding into a Go value), so a 64-bit integer in a nested blob —
// a Gmail thread/message id, for instance — is reproduced digit-for-digit. Round-tripping through `any`
// would coerce every number to float64 and silently lose precision past 2^53, breaking the verbatim
// contract on the exact ids this CLI exists to debug.
func JSON(w io.Writer, raw json.RawMessage) error {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		// Not valid JSON (a plain-text body slipped through): emit it as-is rather than mangling it.
		_, werr := w.Write(append([]byte(raw), '\n'))
		return werr
	}
	buf.WriteByte('\n')
	_, err := w.Write(buf.Bytes())
	return err
}

// Value pretty-prints an arbitrary Go value as JSON (used for synthesized JSON output like the local
// detect/id commands, which have no server body to pass through verbatim).
func Value(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
