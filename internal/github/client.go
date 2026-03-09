package github

import (
	"fmt"
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
func (c *Client) GetPRDiff(pr *PullRequest) (string, error) {
	// Use the GitHub API with Accept header for diff format
	var diff string
	err := c.rest.Do("GET",
		fmt.Sprintf("repos/%s/%s/pulls/%d", pr.Owner, pr.Repo, pr.Number),
		nil,
		&diff,
	)
	if err != nil {
		// Fallback: use gh CLI to get the diff
		return c.getPRDiffViaCLI(pr)
	}
	return diff, nil
}

// getPRDiffViaCLI uses the gh CLI to fetch the diff.
func (c *Client) getPRDiffViaCLI(pr *PullRequest) (string, error) {
	// The REST client doesn't support custom Accept headers easily,
	// so we use a raw request approach
	var diff []byte
	err := c.rest.Do("GET",
		fmt.Sprintf("repos/%s/%s/pulls/%d.diff", pr.Owner, pr.Repo, pr.Number),
		nil,
		&diff,
	)
	if err != nil {
		return "", fmt.Errorf("fetching diff for %s/%s#%d: %w", pr.Owner, pr.Repo, pr.Number, err)
	}
	return string(diff), nil
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
