// Package client asks a running daemon over its /v1 API and decodes the values it
// answers with (internal/api). It opens no store and has no fallback: a daemon that
// cannot be reached is an error that names it.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/token"
)

// maxBody bounds a response the client will read. The whole-person timeline is the
// largest answer, about 5 MB today; this leaves it room to grow tenfold.
const maxBody = 64 << 20

// Client is one daemon, and the token to present to it.
type Client struct {
	server  string
	token   []byte
	timeout time.Duration
	http    *http.Client
}

// New returns a client of the daemon at server. tokenFile may be empty for a daemon
// on loopback without a token. timeout bounds each call that does not set its own.
func New(server, tokenFile string, timeout time.Duration) (*Client, error) {
	c := &Client{server: strings.TrimSuffix(server, "/"), timeout: timeout, http: &http.Client{}}
	if tokenFile != "" {
		t, err := token.Read(tokenFile)
		if err != nil {
			return nil, fmt.Errorf("server %s: %w", c.server, err)
		}
		c.token = t
	}
	return c, nil
}

// Server is the daemon's URL, for messages and links.
func (c *Client) Server() string { return c.server }

// Status asks for the record's state: ledger, W, the daemon's build and its sources.
func (c *Client) Status(ctx context.Context) (api.Status, error) {
	var s api.Status
	_, err := c.do(ctx, c.timeout, http.MethodGet, "/v1/status", nil, nil, &s)
	return s, err
}

// do sends one request under its own deadline and decodes a 2xx body into out. Any
// other status is an error carrying the daemon's message. It returns the status code
// so a caller can tell a finished answer (200) from an accepted one (202).
func (c *Client) do(ctx context.Context, timeout time.Duration, method, path string, q url.Values,
	body any, out any,
) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	target := c.server + path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("server %s: encode request: %w", c.server, err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, rd)
	if err != nil {
		return 0, fmt.Errorf("server %s: %w", c.server, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != nil {
		req.Header.Set("Authorization", "Bearer "+string(c.token))
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return 0, fmt.Errorf("server %s: no answer within %s", c.server, timeout)
		}
		return 0, fmt.Errorf("server %s: %w", c.server, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("server %s: read response: %w", c.server, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.StatusCode, c.failure(resp, b, out)
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			return resp.StatusCode, fmt.Errorf("server %s: decode %s: %w", c.server, path, err)
		}
	}
	return resp.StatusCode, nil
}

// failure turns a non-2xx response into an error. A 500 for an ingest still carries
// the operation, so the body is decoded into out as well when it is one.
func (c *Client) failure(resp *http.Response, b []byte, out any) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		if c.token == nil {
			return fmt.Errorf("server %s wants a token: set token_file (ANAM_TOKEN_FILE)", c.server)
		}
		return fmt.Errorf("server %s refused the token in token_file", c.server)
	case http.StatusNotFound:
		var e api.Error
		if json.Unmarshal(b, &e) != nil || e.Error == "" {
			return fmt.Errorf("server %s has no %s: is it an older build than this CLI?", c.server, resp.Request.URL.Path)
		}
	}
	if out != nil {
		_ = json.Unmarshal(b, out) // best effort: the error below is the answer
	}
	var e api.Error
	if err := json.Unmarshal(b, &e); err == nil && e.Error != "" {
		return &Error{Server: c.server, Status: resp.StatusCode, Message: e.Error}
	}
	return &Error{Server: c.server, Status: resp.StatusCode, Message: strings.TrimSpace(string(b))}
}

// Error is a request the daemon answered and refused.
type Error struct {
	Server  string
	Status  int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("server %s: %s (%d %s)", e.Server, e.Message, e.Status, http.StatusText(e.Status))
}

// Timeline asks for a timeline read. The query carries the reader's zone.
func (c *Client) Timeline(ctx context.Context, q api.TimelineQuery) (api.Timeline, error) {
	var t api.Timeline
	_, err := c.do(ctx, c.timeout, http.MethodGet, "/v1/timeline", q.Values(), nil, &t)
	return t, err
}

// Changes asks what Anam learned in a window of anam-time.
func (c *Client) Changes(ctx context.Context, q api.ChangesQuery) (api.Changes, error) {
	var ch api.Changes
	_, err := c.do(ctx, c.timeout, http.MethodGet, "/v1/changes", q.Values(), nil, &ch)
	return ch, err
}
