// Package client is a thin HTTP wrapper around Klaviyo's REST API.
//
// It sets authentication and API-revision headers, retries transient
// rate-limit (HTTP 429) responses while honoring `Retry-After`, and
// returns the raw response so callers can decode the JSON:API body
// themselves. The package intentionally stops short of modeling
// Klaviyo's resources — that belongs in each resource's Go file.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// DefaultBaseURL is the production Klaviyo API base URL.
const DefaultBaseURL = "https://a.klaviyo.com"

// defaultUserAgent is sent if the caller doesn't override it.
const defaultUserAgent = "terraform-provider-klaviyo"

// maxRetries bounds the number of 429 retries before we give up and
// return the response to the caller. Each retry waits for the value of
// the `Retry-After` header (or defaultRetryAfter if absent or invalid).
const maxRetries = 3

// defaultRetryAfter is the wait used when a 429 omits `Retry-After`.
// Klaviyo's per-endpoint limits are small (single-digit RPS) so 5s is
// a conservative but not painful default.
const defaultRetryAfter = 5 * time.Second

// Client talks to Klaviyo's REST API.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	revision   string
	userAgent  string
}

// Option mutates a Client at construction time.
type Option func(*Client)

// WithHTTPClient lets tests inject an http.Client (e.g. one pointed at
// a httptest.Server).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithBaseURL overrides the default https://a.klaviyo.com base URL.
// Mainly useful for tests and for users routing through a proxy.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = u }
}

// WithUserAgent overrides the default User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// New returns a configured Client. apiKey and revision are required;
// passing empty strings is a programmer error and panics — callers
// should validate user input before reaching this point.
func New(apiKey, revision string, opts ...Option) *Client {
	if apiKey == "" || revision == "" {
		panic("client.New: apiKey and revision are required")
	}
	c := &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    DefaultBaseURL,
		apiKey:     apiKey,
		revision:   revision,
		userAgent:  defaultUserAgent,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Do builds and sends a request to `path` (which should begin with `/`),
// retrying once per 429 up to `maxRetries`. On non-429 responses, even
// errors, the response is returned and the caller decides how to react.
//
// If body is non-nil it is JSON-encoded and sent with content-type
// `application/vnd.api+json`.
func (c *Client) Do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
	}

	var resp *http.Response
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := c.newRequest(ctx, method, path, encoded)
		if err != nil {
			return nil, err
		}
		resp, err = c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", method, path, err)
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}
		// Drain and close so the connection can be reused.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if attempt == maxRetries {
			break
		}
		if err := sleepCtx(ctx, retryAfter(resp)); err != nil {
			return nil, err
		}
	}
	// Final attempt was still a 429; return it so the caller can decide.
	return resp, nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Klaviyo-API-Key "+c.apiKey)
	req.Header.Set("revision", c.revision)
	req.Header.Set("Accept", "application/vnd.api+json")
	req.Header.Set("User-Agent", c.userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/vnd.api+json")
	}
	return req, nil
}

// retryAfter parses the `Retry-After` header. Klaviyo returns an
// integer number of seconds. If the header is missing or unparseable we
// fall back to defaultRetryAfter.
func retryAfter(resp *http.Response) time.Duration {
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return defaultRetryAfter
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs < 0 {
		return defaultRetryAfter
	}
	return time.Duration(secs) * time.Second
}

// sleepCtx waits for d or until ctx is cancelled, whichever comes
// first. Returns ctx.Err() on cancellation.
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// ErrUnexpectedStatus is returned by DecodeError when the response
// status indicates failure but the body could not be parsed as a
// JSON:API error document.
var ErrUnexpectedStatus = errors.New("unexpected HTTP status")
