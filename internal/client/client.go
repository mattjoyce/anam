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

// networkMargin is how much longer the client waits for an answer than the daemon is
// asked to take. The daemon bounds each request by the timeout the client sends; this
// covers the trip.
const networkMargin = 10 * time.Second

// timeoutHeader is the daemon's daemon.TimeoutHeader, spelled here so the client does
// not import the daemon.
const timeoutHeader = "Anam-Timeout"

// Client is one daemon, and the token to present to it.
type Client struct {
	server  string
	token   []byte
	timeout time.Duration
	http    *http.Client
}

// New returns a client of the daemon at server. tokenFile may be empty for a daemon
// on loopback without a token. timeout is what the daemon is asked to answer within,
// for each call that does not set its own; the client waits that plus a margin.
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
	ctx, cancel := context.WithTimeout(ctx, timeout+networkMargin)
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
	req.Header.Set(timeoutHeader, timeout.String())
	if c.token != nil {
		req.Header.Set("Authorization", "Bearer "+string(c.token))
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return 0, fmt.Errorf("server %s: no answer within %s", c.server, timeout+networkMargin)
		}
		// The url.Error repeats the method and URL; the server is already named.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return 0, fmt.Errorf("server %s unreachable: %w", c.server, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("server %s: read response: %w", c.server, err)
	}
	if len(b) > maxBody {
		return resp.StatusCode, fmt.Errorf("server %s: %s answered more than %d MB; narrow the read",
			c.server, path, maxBody>>20)
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

// Receipts lists the daemon's receipts, newest first.
func (c *Client) Receipts(ctx context.Context) (api.Receipts, error) {
	var r api.Receipts
	_, err := c.do(ctx, c.timeout, http.MethodGet, "/v1/receipts", nil, nil, &r)
	return r, err
}

// Receipt reads one receipt and the value it served, from the artifacts alone.
func (c *Client) Receipt(ctx context.Context, hash string) (api.ReceiptShown, error) {
	var r api.ReceiptShown
	_, err := c.do(ctx, c.timeout, http.MethodGet, "/v1/receipts/"+url.PathEscape(hash), nil, nil, &r)
	return r, err
}

// VerifyReceipt asks the daemon to prove one receipt against its ledger and its
// artifacts.
func (c *Client) VerifyReceipt(ctx context.Context, hash string) (api.ReceiptVerified, error) {
	var r api.ReceiptVerified
	_, err := c.do(ctx, c.timeout, http.MethodGet, "/v1/receipts/"+url.PathEscape(hash)+"/verify", nil, nil, &r)
	return r, err
}

// Checkpoints lists every checkpoint the daemon's operational store has logged.
func (c *Client) Checkpoints(ctx context.Context) (api.Checkpoints, error) {
	var cp api.Checkpoints
	_, err := c.do(ctx, c.timeout, http.MethodGet, "/v1/checkpoints", nil, nil, &cp)
	return cp, err
}

// Mint asks the daemon to mint a checkpoint per changed bucket, or with DryRun to
// derive and report without publishing; Full folds the whole ledger.
func (c *Client) Mint(ctx context.Context, mq api.MintQuery) (api.Mint, error) {
	var m api.Mint
	q := url.Values{}
	if mq.DryRun {
		q.Set("dry_run", "true")
	}
	if mq.Full {
		q.Set("full", "true")
	}
	_, err := c.do(ctx, c.timeout, http.MethodPost, "/v1/checkpoints", q, nil, &m)
	return m, err
}

// Ingest asks the daemon to run its imports and waits up to wait for the run to
// finish. It returns the operation as it stands: succeeded, failed, or still going
// when the wait ran out. A run that failed is an answer, not an error.
func (c *Client) Ingest(ctx context.Context, wait time.Duration) (api.Operation, error) {
	var op api.Operation
	q := url.Values{"wait": {wait.String()}}
	_, err := c.do(ctx, wait, http.MethodPost, "/v1/ingest", q, nil, &op)
	var refused *Error
	if errors.As(err, &refused) && refused.Status == http.StatusInternalServerError && op.ID != "" {
		return op, nil
	}
	return op, err
}
