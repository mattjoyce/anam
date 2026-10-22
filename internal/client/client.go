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
	"strconv"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/frame"
	"github.com/mattjoyce/anam/internal/token"
)

// timeoutHeader is the daemon's daemon.TimeoutHeader, spelled here so the client does
// not import the daemon. It is an invariant: both ends of the wire must spell it alike.
const timeoutHeader = "Anam-Timeout"

// Client is one daemon, and the token to present to it.
type Client struct {
	server  string
	token   []byte
	timeout time.Duration
	// margin is how much longer the client waits for an answer than the daemon is
	// asked to take: the trip (network_margin).
	margin time.Duration
	// maxBody bounds a response the client will read (max_response_bytes).
	maxBody int
	http    *http.Client
}

// New returns a client of the daemon at server. tokenFile may be empty for a daemon
// on loopback without a token. timeout is what the daemon is asked to answer within,
// for each call that does not set its own; the client waits that plus margin. maxBody
// bounds what it reads of one answer.
func New(server, tokenFile string, timeout, margin time.Duration, maxBody int) (*Client, error) {
	c := &Client{
		server: strings.TrimSuffix(server, "/"), timeout: timeout, margin: margin, maxBody: maxBody,
		http: &http.Client{},
	}
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
	ctx, cancel := context.WithTimeout(ctx, timeout+c.margin)
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
			return 0, fmt.Errorf("server %s: no answer within %s", c.server, timeout+c.margin)
		}
		// The url.Error repeats the method and URL; the server is already named.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return 0, fmt.Errorf("server %s unreachable: %w", c.server, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, int64(c.maxBody)+1))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("server %s: read response: %w", c.server, err)
	}
	if len(b) > c.maxBody {
		return resp.StatusCode, fmt.Errorf("server %s: %s answered more than %d MB; narrow the read",
			c.server, path, c.maxBody>>20)
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

// failure turns a non-2xx response into an error. The body is decoded into out as
// well, best effort, for a refusal that carries an answer.
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
		return &Error{Server: c.server, Status: resp.StatusCode, Message: e.Error, Code: e.Code, TooLarge: e.TooLarge}
	}
	return &Error{Server: c.server, Status: resp.StatusCode, Message: strings.TrimSpace(string(b))}
}

// Error is a request the daemon answered and refused. Code and TooLarge are the
// daemon's own, when it gave them (api.Error).
type Error struct {
	Server   string
	Status   int
	Message  string
	Code     string
	TooLarge *api.TooLarge
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

// Publish asks the daemon to publish the timeline read q describes, through its
// projection, and answers with the entries and their receipt.
func (c *Client) Publish(ctx context.Context, q api.TimelineQuery) (api.Published, error) {
	var p api.Published
	_, err := c.do(ctx, c.timeout, http.MethodPost, "/v1/receipts", q.Values(), nil, &p)
	return p, err
}

// Query asks the daemon to run a Datalog program through a projection and answer with
// a receipt (card 173).
func (c *Client) Query(ctx context.Context, q api.QueryRequest) (api.QueryAnswer, error) {
	var a api.QueryAnswer
	_, err := c.do(ctx, c.timeout, http.MethodPost, "/v1/query", nil, q, &a)
	return a, err
}

// QueryCatalogue asks what a query can be written against: relations, attributes
// per type and version, the instant lexeme, the bundled rules.
func (c *Client) QueryCatalogue(ctx context.Context) (api.QueryCatalogue, error) {
	var cat api.QueryCatalogue
	_, err := c.do(ctx, c.timeout, http.MethodGet, "/v1/query", nil, nil, &cat)
	return cat, err
}

// Changes asks what Anam learned in a window of anam-time.
func (c *Client) Changes(ctx context.Context, q api.ChangesQuery) (api.Changes, error) {
	var ch api.Changes
	_, err := c.do(ctx, c.timeout, http.MethodGet, "/v1/changes", q.Values(), nil, &ch)
	return ch, err
}

// Receipts lists the daemon's receipts that pass q, newest first.
func (c *Client) Receipts(ctx context.Context, q api.ReceiptsQuery) (api.Receipts, error) {
	var r api.Receipts
	_, err := c.do(ctx, c.timeout, http.MethodGet, "/v1/receipts", q.Values(), nil, &r)
	return r, err
}

// Receipt reads one receipt and the whole value it served, from the artifacts alone.
// The daemon answers a receipt a page at a time (card 92); Receipt reads every page
// in order and joins them, so it answers what it always has, the whole value, and a
// facet too large for any page is the daemon's refusal. Each page is cut from the
// same artifact, so the join is the receipt's value whatever the ledger has learned
// between pages.
func (c *Client) Receipt(ctx context.Context, hash string) (api.ReceiptShown, error) {
	shown, err := c.ReceiptPage(ctx, hash, "")
	if err != nil || shown.Page == nil {
		return shown, err
	}
	var frames []frame.Frame
	if shown.Frame != nil {
		frames = append(frames, *shown.Frame)
	}
	for next := shown.Page.NextCursor; next != ""; {
		page, err := c.ReceiptPage(ctx, hash, next)
		if err != nil {
			return api.ReceiptShown{}, err
		}
		if page.Page == nil {
			return api.ReceiptShown{}, fmt.Errorf("server %s: a page of %s came without its place in the receipt", c.server, hash)
		}
		shown.Entries = append(shown.Entries, page.Entries...)
		if page.Frame != nil {
			frames = append(frames, *page.Frame)
		}
		next = page.Page.NextCursor
	}
	if frames != nil {
		fr, err := frame.Join(frames...)
		if err != nil {
			return api.ReceiptShown{}, fmt.Errorf("server %s: %w", c.server, err)
		}
		shown.Frame = &fr
	}
	shown.Page = &api.Page{ReturnedCount: shown.Page.TotalCount, TotalCount: shown.Page.TotalCount}
	return shown, nil
}

// ReceiptPage reads one page of receipt hash's value: the page cursor names, a
// next_cursor the daemon gave for the same receipt, or the first page when it is
// empty (card 92). Only the first page carries the receipt.
func (c *Client) ReceiptPage(ctx context.Context, hash, cursor string) (api.ReceiptShown, error) {
	var q url.Values
	if cursor != "" {
		q = url.Values{"cursor": {cursor}}
	}
	var r api.ReceiptShown
	_, err := c.do(ctx, c.timeout, http.MethodGet, "/v1/receipts/"+url.PathEscape(hash), q, nil, &r)
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

// Projections lists the templates the daemon holds, bundled and registered.
func (c *Client) Projections(ctx context.Context) (api.Projections, error) {
	var p api.Projections
	_, err := c.do(ctx, c.timeout, http.MethodGet, "/v1/projections", nil, nil, &p)
	return p, err
}

// Projection reads one template's manifest as the daemon holds it: the newest
// version when version is 0, else exactly that one.
func (c *Client) Projection(ctx context.Context, name string, version int) (json.RawMessage, error) {
	path := "/v1/projections/" + url.PathEscape(name)
	if version > 0 {
		path += "/" + strconv.Itoa(version)
	}
	var raw json.RawMessage
	_, err := c.do(ctx, c.timeout, http.MethodGet, path, nil, nil, &raw)
	return raw, err
}

// RegisterProjection registers a template manifest under the operator's token. The
// same body again is not new; a different body at a held version is refused.
func (c *Client) RegisterProjection(ctx context.Context, manifest []byte) (api.RegisteredProjection, error) {
	var reg api.RegisteredProjection
	_, err := c.do(ctx, c.timeout, http.MethodPost, "/v1/projections", nil, json.RawMessage(manifest), &reg)
	return reg, err
}

// Coverage asks what the record holds, type by type, at the W q names.
func (c *Client) Coverage(ctx context.Context, q api.CoverageQuery) (api.Coverage, error) {
	var cv api.Coverage
	_, err := c.do(ctx, c.timeout, http.MethodGet, "/v1/coverage", q.Values(), nil, &cv)
	return cv, err
}
