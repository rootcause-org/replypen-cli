package client

import "context"

// TokenSource supplies the bearer token for each request. ReplyPen tokens are STATIC (no OAuth, no
// refresh), so the source is trivial — it returns a fixed string. The interface is kept (rather than
// passing a bare string) only so the client signature stays stable and a future rotating source could
// slot in without touching the HTTP wrapper.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// staticTokenSource is a fixed bearer with no refresh.
type staticTokenSource string

func (s staticTokenSource) Token(context.Context) (string, error) { return string(s), nil }

// StaticToken wraps a fixed bearer token as a TokenSource.
func StaticToken(tok string) TokenSource { return staticTokenSource(tok) }
