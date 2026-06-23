// This file holds the wire structs — one per endpoint response in docs/api-contract.md. The JSON tags
// MUST match the server's field names VERBATIM (the contract is the source of truth): the render layer
// reads these structs for tables, while -o json bypasses them entirely (verbatim passthrough), so a tag
// drift here only ever degrades a table, never the jq path. Freeform/nested blobs the CLI doesn't render
// (triage_result, injection_result, log details) stay json.RawMessage so they ride the JSON path intact.
package client

import "encoding/json"

// --- GET /api/v1/debug/whoami ------------------------------------------------------------------------

// Whoami is the resolved token scope. ProjectSlug/TenantCodename are null for an admin token, so they're
// pointers to round-trip the JSON null distinction.
type Whoami struct {
	Scope          string  `json:"scope"` // "admin" | "project"
	ProjectSlug    *string `json:"project_slug"`
	TenantCodename *string `json:"tenant_codename"`
}

// --- GET /api/v1/debug/projects ----------------------------------------------------------------------

type ProjectsResponse struct {
	Projects []Project `json:"projects"`
}

type Project struct {
	ID             string `json:"id"`
	Slug           string `json:"slug"`
	TenantCodename string `json:"tenant_codename"`
	WebhookURL     string `json:"webhook_url"`
	MailboxCount   int    `json:"mailbox_count"`
	CreatedAt      string `json:"created_at"`
}

// --- GET /api/v1/debug/projects/{slug}/threads -------------------------------------------------------

type ThreadsResponse struct {
	Threads []Thread `json:"threads"`
}

type Thread struct {
	ID               string `json:"id"`
	ExternalThreadID string `json:"external_thread_id"`
	Subject          string `json:"subject"`
	Status           string `json:"status"`
	Sender           string `json:"sender"`
	MailboxEmail     string `json:"mailbox_email"`
	Channel          string `json:"channel"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// --- GET /api/v1/debug/projects/{slug}/triage --------------------------------------------------------

type TriageResponse struct {
	Decisions []TriageDecision `json:"decisions"`
}

type TriageDecision struct {
	ThreadID      string  `json:"thread_id"`
	ReceivedAt    *string `json:"received_at"` // null when the thread has no inbound message yet
	Sender        string  `json:"sender"`
	Subject       string  `json:"subject"`
	ThreadStatus  string  `json:"thread_status"`
	ShouldProcess bool    `json:"should_process"`
	Category      string  `json:"category"`
	Confidence    float64 `json:"confidence"`
	Reason        string  `json:"reason"`
	Mailbox       string  `json:"mailbox"`
}

// --- GET /api/v1/debug/threads/{id}/trace ------------------------------------------------------------

// Trace is the full assembled thread bundle. The CLI renders `timeline` (and decomposes it to .md/.jsonl);
// the raw sub-arrays stay reachable via -o json. Nested decision blobs are RawMessage — carried, not
// reshaped.
type Trace struct {
	Thread     TraceThread     `json:"thread"`
	Mailbox    TraceMailbox    `json:"mailbox"`
	Project    TraceProject    `json:"project"`
	Messages   []TraceMessage  `json:"messages"`
	Logs       []TraceLog      `json:"logs"`
	Deliveries []TraceDelivery `json:"deliveries"`
	Drafts     []TraceDraft    `json:"drafts"`
	Notes      []TraceNote     `json:"notes"`
	Timeline   []TimelineEntry `json:"timeline"`
}

type TraceThread struct {
	ID                string          `json:"id"`
	ExternalThreadID  string          `json:"external_thread_id"`
	Subject           string          `json:"subject"`
	Status            string          `json:"status"`
	Channel           string          `json:"channel"`
	ActiveSessionID   string          `json:"active_session_id"`
	PipelineStartedAt *string         `json:"pipeline_started_at"` // null until the pipeline first runs
	PendingReentry    bool            `json:"pending_reentry"`
	TriageResult      json.RawMessage `json:"triage_result"`
	InjectionResult   json.RawMessage `json:"injection_result"`
	ErrorMessage      *string         `json:"error_message"`
	ProcessorFailure  json.RawMessage `json:"processor_failure"` // a JSON blob (or null), never a string
	DeclineReason     *string         `json:"decline_reason"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
}

type TraceMailbox struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

type TraceProject struct {
	ID             string `json:"id"`
	Slug           string `json:"slug"`
	TenantCodename string `json:"tenant_codename"`
	WebhookURL     string `json:"webhook_url"`
}

type TraceMessage struct {
	ID                string   `json:"id"`
	ExternalMessageID string   `json:"external_message_id"`
	Direction         string   `json:"direction"`
	IsDraft           bool     `json:"is_draft"`
	From              string   `json:"from"`
	To                []string `json:"to"`
	Cc                []string `json:"cc"`
	Subject           string   `json:"subject"`
	Date              string   `json:"date"`
	BodyCleanedLen    int      `json:"body_cleaned_len"`
	Labels            []string `json:"labels"`
}

type TraceLog struct {
	Step       string          `json:"step"`
	Status     string          `json:"status"`
	Details    json.RawMessage `json:"details"`
	DurationMs *int            `json:"duration_ms"` // null when the step recorded no duration
	CreatedAt  string          `json:"created_at"`
}

type TraceDelivery struct {
	ID             string `json:"id"`
	Direction      string `json:"direction"`
	SessionID      string `json:"session_id"`
	TurnIndex      int    `json:"turn_index"`
	URL            string `json:"url"`
	ResponseStatus *int   `json:"response_status"` // null for an outbound leg with no response yet
	Attempt        int    `json:"attempt"`
	CreatedAt      string `json:"created_at"`
}

type TraceDraft struct {
	ID              string  `json:"id"`
	Status          string  `json:"status"`
	ProposedAt      string  `json:"proposed_at"`
	PlacedAt        *string `json:"placed_at"`         // null until the draft lands in the mailbox
	DeletionReason  *string `json:"deletion_reason"`   // null unless the draft was retired
	ExternalDraftID *string `json:"external_draft_id"` // provider draft id once placed; null otherwise
}

type TraceNote struct {
	ID           string  `json:"id"`
	Code         string  `json:"code"`
	Kind         string  `json:"kind"`
	Status       string  `json:"status"`
	BodyMarkdown string  `json:"body_markdown"`
	CreatedAt    string  `json:"created_at"`
	DeletedAt    *string `json:"deleted_at"` // null unless the note was deleted
}

// TimelineEntry is one row of the server-merged, timestamp-sorted interleave. `type` keys the JSONL line.
type TimelineEntry struct {
	TS     string `json:"ts"`
	Type   string `json:"type"` // message | log | delivery | draft | note
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

// --- POST /api/v1/debug/projects/{slug}/cli-token ----------------------------------------------------

type MintTokenResponse struct {
	Token       string `json:"token"`
	ProjectSlug string `json:"project_slug"`
}

// --- onboarding wrappers over existing replypen endpoints --------------------------------------------

// RegisterRequest is the body of POST /api/v1/register (tenant registration; X-Admin-Secret header).
type RegisterRequest struct {
	Codename string `json:"codename"`
}

// RegisterResponse carries whatever the register endpoint returns; the load-bearing field is the tenant's
// API token. Extra fields ride through via -o json (the command renders the token + codename).
type RegisterResponse struct {
	TenantID string `json:"tenant_id"`
	Codename string `json:"codename"`
	APIToken string `json:"api_token"`
}

// ProjectCreateRequest is the body of POST /api/v1/projects (bearer = the tenant API token).
type ProjectCreateRequest struct {
	Name        string `json:"name"`
	WebhookURL  string `json:"webhook_url"`
	TriageModel string `json:"triage_model,omitempty"`
}

// ProjectCreateResponse carries the created project + its one-time webhook_secret.
type ProjectCreateResponse struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	WebhookURL    string `json:"webhook_url"`
	WebhookSecret string `json:"webhook_secret"`
}

// MailboxConnectRequest is the body of POST /api/v1/projects/{slug}/mailboxes/connect.
type MailboxConnectRequest struct {
	Email    string `json:"email,omitempty"`
	Provider string `json:"provider"`
}

// MailboxConnectResponse carries the OAuth URL the operator opens to grant mailbox access.
type MailboxConnectResponse struct {
	OAuthURL string `json:"oauth_url"`
}
