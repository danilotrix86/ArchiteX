// Package github provides a minimal GitHub REST client used by `architex
// comment` to upsert a sticky PR comment. It depends only on the standard
// library so the binary keeps a small, auditable network surface.
//
// The client is intentionally small: it covers exactly the endpoints needed
// to (1) list issue comments on a PR with pagination and (2) create or
// update a single issue comment. Anything beyond that is out of scope.
//
// Trust model: this package is the ONLY place in architex that performs
// outbound network calls. The analysis pipeline (parser/graph/delta/risk/
// interpreter) never imports it. See llm.md design decision 27.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.github.com"
	acceptHeader   = "application/vnd.github+json"
	apiVersion     = "2022-11-28"
	userAgent      = "architex-action"
)

// Comment is the subset of the GitHub Issue Comment object architex needs.
// Field names mirror the REST API response.
type Comment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
	URL  string `json:"html_url"`
}

// Client is a minimal GitHub REST client. Construct with New; override
// BaseURL and HTTPClient in tests.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      string
}

// New returns a Client preconfigured with a 30s timeout and the public
// api.github.com base URL. Pass an empty token to make unauthenticated
// requests (rate-limited; not useful in CI).
func New(token string) *Client {
	return &Client{
		BaseURL:    defaultBaseURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Token:      token,
	}
}

// ListIssueComments returns every comment on an issue or PR, following the
// Link: rel="next" header for pagination. Returns an empty slice (never nil)
// when there are no comments.
func (c *Client) ListIssueComments(ctx context.Context, owner, repo string, issueNum int) ([]Comment, error) {
	out := make([]Comment, 0)
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments?per_page=100", c.BaseURL, owner, repo, issueNum)
	for url != "" {
		var page []Comment
		next, err := c.do(ctx, http.MethodGet, url, nil, &page)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		url = next
	}
	return out, nil
}

// CreateIssueComment posts a new comment with the given body.
func (c *Client) CreateIssueComment(ctx context.Context, owner, repo string, issueNum int, body string) (Comment, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.BaseURL, owner, repo, issueNum)
	var got Comment
	payload := map[string]string{"body": body}
	if _, err := c.do(ctx, http.MethodPost, url, payload, &got); err != nil {
		return Comment{}, err
	}
	return got, nil
}

// UpdateIssueComment replaces the body of an existing comment by ID.
func (c *Client) UpdateIssueComment(ctx context.Context, owner, repo string, commentID int64, body string) (Comment, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/comments/%d", c.BaseURL, owner, repo, commentID)
	var got Comment
	payload := map[string]string{"body": body}
	if _, err := c.do(ctx, http.MethodPatch, url, payload, &got); err != nil {
		return Comment{}, err
	}
	return got, nil
}

// UpsertStickyComment finds the first existing comment whose body contains
// marker and updates it; otherwise it creates a new comment. The body is
// posted verbatim -- no marker injection here. Callers are responsible for
// embedding the marker in body (the Markdown formatter already does).
//
// Returns the resulting Comment so callers can log its URL.
func UpsertStickyComment(ctx context.Context, c *Client, owner, repo string, issueNum int, marker, body string) (Comment, error) {
	if marker == "" {
		return Comment{}, fmt.Errorf("github: empty sticky marker would match every comment")
	}
	if !strings.Contains(body, marker) {
		return Comment{}, fmt.Errorf("github: body does not contain sticky marker %q -- refusing to post a comment that cannot be updated later", marker)
	}
	existing, err := c.ListIssueComments(ctx, owner, repo, issueNum)
	if err != nil {
		return Comment{}, fmt.Errorf("github: list comments: %w", err)
	}
	for _, cm := range existing {
		if strings.Contains(cm.Body, marker) {
			updated, err := c.UpdateIssueComment(ctx, owner, repo, cm.ID, body)
			if err != nil {
				return Comment{}, fmt.Errorf("github: update comment %d: %w", cm.ID, err)
			}
			return updated, nil
		}
	}
	created, err := c.CreateIssueComment(ctx, owner, repo, issueNum, body)
	if err != nil {
		return Comment{}, fmt.Errorf("github: create comment: %w", err)
	}
	return created, nil
}

// do executes an authenticated request, optionally JSON-encoding payload and
// JSON-decoding into out. Returns the next-page URL when the response carries
// a Link header with rel="next", otherwise "".
func (c *Client) do(ctx context.Context, method, url string, payload, out any) (string, error) {
	var bodyReader io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return "", fmt.Errorf("marshal payload: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", acceptHeader)
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("github: %s %s: HTTP %d: %s", method, url, resp.StatusCode, truncate(string(respBody), 256))
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return "", fmt.Errorf("decode response: %w", err)
		}
	}
	return parseNextLink(resp.Header.Get("Link")), nil
}

// linkNextRE matches a single <url>; rel="next" entry in an RFC 5988 Link
// header (the format the GitHub API uses for pagination).
var linkNextRE = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func parseNextLink(header string) string {
	if header == "" {
		return ""
	}
	m := linkNextRE.FindStringSubmatch(header)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
