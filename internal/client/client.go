// Package client is the one thin HTTP wrapper over the replypen JSON API. It sets the static bearer +
// base URL, speaks JSON, and on any non-2xx decodes the {"error","code"} envelope into a typed APIError
// (surfaced verbatim). It holds NO business logic — every method is one request mapping straight onto one
// endpoint, returning the wire struct for the render layer. The JSON-output path uses Raw instead, so
// `-o json` emits exactly what the server sent (render, don't reshape).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client is a static-bearer handle to the API. The token resolves the caller's scope server-side (admin
// vs one project), so there is no scope parameter anywhere. There is no refresh — a 401 surfaces verbatim.
type Client struct {
	baseURL string
	tokens  TokenSource
	http    *http.Client
}

// New builds a Client. baseURL is trimmed of a trailing slash so path joins stay clean.
func New(baseURL string, tokens TokenSource) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		tokens:  tokens,
		http:    &http.Client{},
	}
}

// pathOnly strips the query string from a path for error display (the query is noise when the point is
// which endpoint was missing).
func pathOnly(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		return path[:i]
	}
	return path
}

// --- path builders (exported so the command layer can pass the same path to Raw for -o json) -----------

func WhoamiPath() string   { return "/api/v1/debug/whoami" }
func ProjectsPath() string { return "/api/v1/debug/projects" }
func MintTokenPath(slug string) string {
	return "/api/v1/debug/projects/" + url.PathEscape(slug) + "/cli-token"
}

// MintTenantTokenPath builds the tenant-scoped mint endpoint (admin only).
func MintTenantTokenPath(codename string) string {
	return "/api/v1/debug/tenants/" + url.PathEscape(codename) + "/cli-token"
}

// ThreadsPath builds the project-threads path with optional limit/status filters.
func ThreadsPath(slug string, limit int, status string) string {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if status != "" {
		q.Set("status", status)
	}
	return withQuery("/api/v1/debug/projects/"+url.PathEscape(slug)+"/threads", q)
}

// TriagePath builds the project-triage path with an optional limit.
func TriagePath(slug string, limit int) string {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	return withQuery("/api/v1/debug/projects/"+url.PathEscape(slug)+"/triage", q)
}

// TracePath builds the thread-trace path. {id} accepts a replypen UUID or an external/provider id.
func TracePath(id string) string {
	return "/api/v1/debug/threads/" + url.PathEscape(id) + "/trace"
}

func withQuery(path string, q url.Values) string {
	if enc := q.Encode(); enc != "" {
		return path + "?" + enc
	}
	return path
}

// --- debug endpoints (token-scoped) ------------------------------------------------------------------

func (c *Client) Whoami(ctx context.Context) (*Whoami, error) {
	var out Whoami
	return &out, c.do(ctx, http.MethodGet, WhoamiPath(), nil, nil, &out)
}

func (c *Client) Projects(ctx context.Context) (*ProjectsResponse, error) {
	var out ProjectsResponse
	return &out, c.do(ctx, http.MethodGet, ProjectsPath(), nil, nil, &out)
}

func (c *Client) Threads(ctx context.Context, slug string, limit int, status string) (*ThreadsResponse, error) {
	var out ThreadsResponse
	return &out, c.do(ctx, http.MethodGet, ThreadsPath(slug, limit, status), nil, nil, &out)
}

func (c *Client) Triage(ctx context.Context, slug string, limit int) (*TriageResponse, error) {
	var out TriageResponse
	return &out, c.do(ctx, http.MethodGet, TriagePath(slug, limit), nil, nil, &out)
}

func (c *Client) Trace(ctx context.Context, id string) (*Trace, error) {
	var out Trace
	return &out, c.do(ctx, http.MethodGet, TracePath(id), nil, nil, &out)
}

func (c *Client) MintToken(ctx context.Context, slug string) (*MintTokenResponse, error) {
	var out MintTokenResponse
	return &out, c.do(ctx, http.MethodPost, MintTokenPath(slug), nil, nil, &out)
}

func (c *Client) MintTenantToken(ctx context.Context, codename string) (*MintTokenResponse, error) {
	var out MintTokenResponse
	return &out, c.do(ctx, http.MethodPost, MintTenantTokenPath(codename), nil, nil, &out)
}

// --- onboarding wrappers over existing replypen endpoints --------------------------------------------

// Register posts POST /api/v1/register with the X-Admin-Secret header (NOT a bearer): the admin secret is
// the only credential the register endpoint accepts, so this is the one method that doesn't carry a token.
func (c *Client) Register(ctx context.Context, adminSecret string, req RegisterRequest) (*RegisterResponse, json.RawMessage, error) {
	hdr := http.Header{}
	if adminSecret != "" {
		hdr.Set("X-Admin-Secret", adminSecret)
	}
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodPost, "/api/v1/register", hdr, req, &raw); err != nil {
		return nil, nil, err
	}
	var out RegisterResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, nil, fmt.Errorf("decode register response: %w", err)
	}
	return &out, raw, nil
}

// CreateProject posts POST /api/v1/projects (bearer = the tenant API token, carried by the client).
func (c *Client) CreateProject(ctx context.Context, req ProjectCreateRequest) (*ProjectCreateResponse, json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodPost, "/api/v1/projects", nil, req, &raw); err != nil {
		return nil, nil, err
	}
	var out ProjectCreateResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, nil, fmt.Errorf("decode project response: %w", err)
	}
	return &out, raw, nil
}

// ConnectMailbox posts POST /api/v1/projects/{slug}/mailboxes/connect (bearer = the tenant API token).
func (c *Client) ConnectMailbox(ctx context.Context, slug string, req MailboxConnectRequest) (*MailboxConnectResponse, json.RawMessage, error) {
	var raw json.RawMessage
	path := "/api/v1/projects/" + url.PathEscape(slug) + "/mailboxes/connect"
	if err := c.do(ctx, http.MethodPost, path, nil, req, &raw); err != nil {
		return nil, nil, err
	}
	var out MailboxConnectResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, nil, fmt.Errorf("decode mailbox response: %w", err)
	}
	return &out, raw, nil
}

// Raw returns the response BODY bytes for JSON passthrough, so `-o json` emits exactly what the server
// sent (the CLI renders; it never reshapes for jq). extraHeaders carries the X-Admin-Secret for the
// register passthrough; it's nil for bearer-only endpoints.
func (c *Client) Raw(ctx context.Context, method, path string, extraHeaders http.Header, body any) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.do(ctx, method, path, extraHeaders, body, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// do issues one request: static bearer auth, JSON body in/out, and on non-2xx decodes the {"error","code"}
// envelope into a typed APIError. out may be a *json.RawMessage to capture the body unparsed for
// passthrough. There is no 401-retry — ReplyPen tokens are static, so a 401 is a real auth failure.
func (c *Client) do(ctx context.Context, method, path string, extraHeaders http.Header, body any, out any) error {
	var reqBody []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = b
	}

	token, err := c.tokens.Token(ctx)
	if err != nil {
		return err
	}

	var r io.Reader
	if reqBody != nil {
		r = bytes.NewReader(reqBody)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, vs := range extraHeaders {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// Connection-level failure: include the base URL so a request that silently went to the localhost
		// default instead of the intended host is obvious.
		return fmt.Errorf("request %s %s (base %s): %w", method, path, c.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{Status: resp.StatusCode}
		var env errorEnvelope
		if json.Unmarshal(data, &env) == nil && env.Code != "" {
			apiErr.Code = env.Code
			apiErr.Message = env.Error
		} else {
			// No decodable envelope (plain-text proxy 404/405, or an older server): keep method/path/base so
			// the user sees WHAT was hit WHERE rather than a bare "HTTP 405".
			apiErr.Method = method
			apiErr.Path = pathOnly(path)
			apiErr.BaseURL = c.baseURL
		}
		return apiErr
	}

	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
