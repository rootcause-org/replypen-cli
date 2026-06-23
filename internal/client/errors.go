// This file carries the API's error envelope through to the user verbatim. The contract is that the
// server owns the vocabulary (codes, messages); the CLI surfaces code+message exactly as sent and exits
// non-zero — it never paraphrases or invents an error. ReplyPen's envelope is the flat
// `{"error":"message","code":"CODE"}` shape (see CLAUDE.md "Standard error response").
package client

import "fmt"

// errorEnvelope is the decode target for a non-2xx body: {"error":"message","code":"CODE"}.
type errorEnvelope struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// APIError carries the server's verbatim error so the command layer can print code/message to stderr and
// exit 1. A zero Code means a non-2xx with no decodable envelope (a plain-text 404/405 from a proxy or an
// older server); Method/Path/BaseURL then give the user enough to see WHAT was hit WHERE.
type APIError struct {
	Status  int    // HTTP status, for the no-envelope fallback
	Code    string // server error code, verbatim (e.g. FORBIDDEN)
	Message string // server message, verbatim
	Method  string // request method, for the no-envelope fallback
	Path    string // request path, for the no-envelope fallback
	BaseURL string // base URL the request went to, so the user can spot a wrong/default host
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("HTTP %d", e.Status)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
