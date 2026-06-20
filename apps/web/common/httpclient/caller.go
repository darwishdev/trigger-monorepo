package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/go-resty/resty/v2"
)

// ErrRequest is returned (wrapped) when the HTTP call itself fails at the
// transport layer - a network error, timeout, or DNS failure. It is distinct
// from a non-success HTTP response, which is mapped via the Caller's injected
// error mapper. This lets callers tell "we couldn't reach the API" apart from
// "the API replied with an error".
var ErrRequest = errors.New("httpclient: request failed")

// ErrorMapper translates a non-success HTTP status code and response body into
// a caller-defined error. Implementations are domain-owned (e.g.
// sales.MapError) so this package stays free of any domain knowledge.
type ErrorMapper func(status int, body []byte) error

// Caller wraps a resty.Client with reusable Get/Post helpers and an injectable
// error mapper. Any package that talks to an external HTTP API builds a Caller
// once and reuses the request/error mechanics instead of re-implementing them
// per adapter.
type Caller struct {
	http   *resty.Client
	mapErr ErrorMapper
}

// NewCaller returns a Caller. mapErr may be nil, in which case non-success
// responses are reported as a wrapped ErrRequest carrying the status code.
func NewCaller(c *resty.Client, mapErr ErrorMapper) *Caller {
	return &Caller{http: c, mapErr: mapErr}
}

// Get issues a GET request. On a success status the JSON body is unmarshaled
// into out. Any transport failure or non-success status is returned as an error.
func (c *Caller) Get(ctx context.Context, path string, params url.Values, out any) error {
	resp, err := c.http.R().
		SetContext(ctx).
		SetQueryParamsFromValues(params).
		SetResult(out).
		Get(path)
	return c.check(resp, err)
}

// Post issues a POST request. If body is non-nil it is sent as the JSON body.
// On a success status the JSON body is unmarshaled into out.
func (c *Caller) Post(ctx context.Context, path string, params url.Values, body any, out any) error {
	req := c.http.R().
		SetContext(ctx).
		SetQueryParamsFromValues(params).
		SetResult(out)
	if body != nil {
		req = req.SetBody(body)
	}
	resp, err := req.Post(path)
	return c.check(resp, err)
}

func (c *Caller) check(resp *resty.Response, err error) error {
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRequest, err)
	}
	if resp.IsSuccess() {
		return nil
	}
	if c.mapErr != nil {
		return c.mapErr(resp.StatusCode(), resp.Body())
	}
	return fmt.Errorf("%w: HTTP %d", ErrRequest, resp.StatusCode())
}
