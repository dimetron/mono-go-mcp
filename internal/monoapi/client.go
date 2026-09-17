// Package monoapi is a thin HTTP client for the monobank open API
// (https://api.monobank.ua/docs/index.html, spec version v250818).
//
// Authentication: personal token from https://api.monobank.ua/ sent as
// the X-Token header. Rate limits: 1 request per 60 seconds for
// /personal/client-info and /personal/statement.
package monoapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// BaseURL is the monobank open API base URL.
	BaseURL = "https://api.monobank.ua"
	// StatementMaxRangeSeconds is the maximum statement window
	// (31 days + 1 hour).
	StatementMaxRangeSeconds = 2_682_000
	// RateLimitInterval is the documented limit for
	// /personal/* statement and client-info calls: 1 per 60s.
	RateLimitInterval = 60 * time.Second
)

// Client makes calls against the monobank open API.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
	cache   *responseCache
}

// NewClient returns a Client that talks to BaseURL with the given
// personal token. Responses of the rate-limited /personal/* endpoints
// are cached in-process for 65 s to respect the 60 s API limit.
func NewClient(token, baseURL string) *Client {
	if baseURL == "" {
		baseURL = BaseURL
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
		cache:   newResponseCache(),
	}
}

// APIError describes a non-2xx monobank response. Monobank returns
// {"errorDescription": "..."} bodies; the HTTP status code is what
// should be analyzed programmatically. Error text is sanitized: the
// raw body is truncated and control characters stripped, so an
// upstream response can neither leak unbounded content nor inject
// terminal escape sequences into CLI logs.
type APIError struct {
	StatusCode        int
	Status            string
	Endpoint          string
	RateLimitResetSec int
	Body              string
}

func (e *APIError) Error() string {
	msg := strings.TrimSpace(e.Body)
	if msg == "" {
		msg = e.Status
	}
	s := fmt.Sprintf("monobank API %s: %s", e.Endpoint, msg)
	if e.RateLimitResetSec > 0 {
		s += fmt.Sprintf(" (rate limit resets in %ds)", e.RateLimitResetSec)
	}
	return s
}

// IsRateLimited reports whether the error is HTTP 429.
func (e *APIError) IsRateLimited() bool { return e.StatusCode == http.StatusTooManyRequests }

// get performs a GET request and decodes the JSON body into out.
func (c *Client) get(ctx context.Context, endpoint string, out any) error {
	return c.do(ctx, http.MethodGet, endpoint, nil, out)
}

// post performs a POST request with a JSON body.
func (c *Client) post(ctx context.Context, endpoint string, body, out any) error {
	return c.do(ctx, http.MethodPost, endpoint, body, out)
}

func (c *Client) do(ctx context.Context, method, endpoint string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, rd)
	if err != nil {
		return err
	}
	req.Header.Set("X-Token", c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20+1))
	if err != nil {
		return err
	}
	if len(data) > 10<<20 {
		return fmt.Errorf("%s %s: response body exceeds 10 MiB limit", method, endpoint)
	}
	if resp.StatusCode >= 300 {
		apiErr := &APIError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Endpoint:   endpoint,
			Body:       sanitizeBody(string(data)),
		}
		if s := resp.Header.Get("X-Auth-Interval-Expires"); s != "" {
			if n := resetSeconds(s); n > 0 {
				apiErr.RateLimitResetSec = n
			}
		}
		return apiErr
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode %s response: %w", endpoint, err)
	}
	return nil
}

// resetSeconds parses the X-Auth-Interval-Expires header value, an
// unsigned integer count of seconds. Malformed values (empty, non-digit,
// negative) yield 0.
func resetSeconds(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// sanitizeBody bounds and cleans an upstream response body before it
// becomes error text: control characters (which could inject terminal
// escape sequences into CLI logs) are dropped and the result is capped
// at 1 KiB — enough for monobank's {"errorDescription": "..."} JSON.
func sanitizeBody(s string) string {
	clean := strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f {
			return -1
		}
		return r
	}, strings.TrimSpace(s))
	const max = 1024
	r := []rune(clean)
	if len(r) > max {
		r = append(r[:max], []rune("…")...)
	}
	return string(r)
}
