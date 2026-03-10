package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"

	ghAPI "github.com/cli/go-gh/v2/pkg/api"
)

// PullRequest represents a parsed PR reference.
type PullRequest struct {
	Owner  string
	Repo   string
	Number int
}

var prURLRegex = regexp.MustCompile(`github\.com/([^/]+)/([^/]+)/pull/(\d+)`)

// ParsePRURL extracts owner, repo, and PR number from a GitHub PR URL.
func ParsePRURL(url string) (*PullRequest, error) {
	matches := prURLRegex.FindStringSubmatch(url)
	if matches == nil {
		return nil, fmt.Errorf("invalid PR URL: %s (expected format: https://github.com/owner/repo/pull/123)", url)
	}

	num, _ := strconv.Atoi(matches[3])
	return &PullRequest{
		Owner:  matches[1],
		Repo:   matches[2],
		Number: num,
	}, nil
}

// Client wraps the GitHub API client.
type Client struct {
	rest *ghAPI.RESTClient
}

// NewClient creates a new GitHub API client using gh's authentication.
func NewClient() (*Client, error) {
	rest, err := ghAPI.DefaultRESTClient()
	if err != nil {
		return nil, fmt.Errorf("creating REST client (are you logged in with 'gh auth login'?): %w", err)
	}
	return &Client{rest: rest}, nil
}

// GetPRDiff fetches the unified diff for a pull request.
//
// go-gh's REST client always JSON-decodes responses, so we use `gh api` directly
// with a diff Accept header to get raw text without going through JSON unmarshalling.
func (c *Client) GetPRDiff(pr *PullRequest) (string, error) {
	out, err := exec.Command(
		"gh", "api",
		"--header", "Accept: application/vnd.github.v3.diff",
		fmt.Sprintf("repos/%s/%s/pulls/%d", pr.Owner, pr.Repo, pr.Number),
	).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("fetching diff for %s/%s#%d: %s", pr.Owner, pr.Repo, pr.Number, string(exitErr.Stderr))
		}
		return "", fmt.Errorf("fetching diff for %s/%s#%d: %w", pr.Owner, pr.Repo, pr.Number, err)
	}
	return string(out), nil
}

// GetPRHeadSHA fetches the head commit SHA for a pull request.
func (c *Client) GetPRHeadSHA(pr *PullRequest) (string, error) {
	var response struct {
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
	}
	err := c.rest.Get(
		fmt.Sprintf("repos/%s/%s/pulls/%d", pr.Owner, pr.Repo, pr.Number),
		&response,
	)
	if err != nil {
		return "", err
	}
	return response.Head.SHA, nil
}

// PostPRLineComment posts a line-specific review comment on the pull request.
// path must be the file path relative to the repo root (no "b/" prefix).
// line is the line number in the new file (right side of the diff).
// commitSHA must be the head commit SHA of the PR.
func (c *Client) PostPRLineComment(pr *PullRequest, commitSHA, path string, line int, body string) error {
	payload, err := json.Marshal(map[string]interface{}{
		"body":      body,
		"commit_id": commitSHA,
		"path":      path,
		"line":      line,
		"side":      "RIGHT",
	})
	if err != nil {
		return fmt.Errorf("encoding comment payload: %w", err)
	}

	cmd := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/%s/pulls/%d/comments", pr.Owner, pr.Repo, pr.Number),
		"--method", "POST",
		"--input", "-",
	)
	cmd.Stdin = bytes.NewReader(payload)
	if out, err := cmd.Output(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("posting line comment: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("posting line comment: %w", err)
	} else {
		_ = out
	}
	return nil
}

// PRInfo holds metadata about a pull request.
type PRInfo struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	State string `json:"state"`
	Head  string `json:"head"`
	Base  string `json:"base"`
}

// GetPRInfo fetches metadata about a pull request.
func (c *Client) GetPRInfo(pr *PullRequest) (*PRInfo, error) {
	var response struct {
		Title string `json:"title"`
		Body  string `json:"body"`
		State string `json:"state"`
		Head  struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}

	err := c.rest.Get(
		fmt.Sprintf("repos/%s/%s/pulls/%d", pr.Owner, pr.Repo, pr.Number),
		&response,
	)
	if err != nil {
		return nil, err
	}

	return &PRInfo{
		Title: response.Title,
		Body:  response.Body,
		State: response.State,
		Head:  response.Head.Ref,
		Base:  response.Base.Ref,
	}, nil
}
