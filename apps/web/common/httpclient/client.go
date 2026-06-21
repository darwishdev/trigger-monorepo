// Package httpclient provides a reusable, resty-based HTTP client factory for
// any package that needs to talk to an external HTTP API. It centralizes sane
// defaults (timeout, retry, JSON) and is free of any domain-specific
// knowledge. CRM adapters, identity providers, storage SDKs, etc. all build on
// top of this rather than each configuring resty from scratch.
package httpclient

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

const (
	defaultTimeout      = 30 * time.Second
	defaultRetryCount   = 2
	defaultRetryWait    = 400 * time.Millisecond
	defaultRetryMaxWait = 3 * time.Second
)

// Option configures a resty.Client at construction time.
type Option func(*resty.Client)

// New returns a resty.Client configured with base defaults: JSON accept and
// content-type headers, a bounded timeout, and best-effort retry on transient
// failures (network errors and HTTP 5xx). Apply options to add auth, headers,
// or override defaults.
func New(baseURL string, opts ...Option) *resty.Client {
	c := resty.New().
		SetBaseURL(strings.TrimRight(baseURL, "/")).
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetTimeout(defaultTimeout).
		SetRetryCount(defaultRetryCount).
		SetRetryWaitTime(defaultRetryWait).
		SetRetryMaxWaitTime(defaultRetryMaxWait)
	c.AddRetryCondition(func(r *resty.Response, err error) bool {
		if err != nil {
			return true
		}
		return r.StatusCode() >= http.StatusInternalServerError
	})
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// WithBearerToken adds an "Authorization: Bearer <token>" header to every
// request.
func WithBearerToken(token string) Option {
	return func(c *resty.Client) {
		c.SetHeader("Authorization", "Bearer "+token)
	}
}

// WithTimeout overrides the default request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *resty.Client) {
		c.SetTimeout(d)
	}
}

// WithRetry overrides the default retry policy (count + backoff bounds).
func WithRetry(count int, wait, maxWait time.Duration) Option {
	return func(c *resty.Client) {
		c.SetRetryCount(count).
			SetRetryWaitTime(wait).
			SetRetryMaxWaitTime(maxWait)
	}
}

// WithHeader adds a static header to every request.
func WithHeader(key, value string) Option {
	return func(c *resty.Client) {
		c.SetHeader(key, value)
	}
}
