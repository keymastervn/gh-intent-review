package github

import (
	"testing"
)

func TestParsePRURL_Valid(t *testing.T) {
	tests := []struct {
		url    string
		owner  string
		repo   string
		number int
	}{
		{
			url:    "https://github.com/Thinkei/ats/pull/1037",
			owner:  "Thinkei",
			repo:   "ats",
			number: 1037,
		},
		{
			url:    "https://github.com/acme/my-repo/pull/1",
			owner:  "acme",
			repo:   "my-repo",
			number: 1,
		},
		{
			url:    "github.com/owner/repo/pull/999",
			owner:  "owner",
			repo:   "repo",
			number: 999,
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			pr, err := ParsePRURL(tt.url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pr.Owner != tt.owner {
				t.Errorf("expected owner %q, got %q", tt.owner, pr.Owner)
			}
			if pr.Repo != tt.repo {
				t.Errorf("expected repo %q, got %q", tt.repo, pr.Repo)
			}
			if pr.Number != tt.number {
				t.Errorf("expected number %d, got %d", tt.number, pr.Number)
			}
		})
	}
}

func TestParsePRURL_Invalid(t *testing.T) {
	tests := []string{
		"https://github.com/owner/repo",
		"https://github.com/owner/repo/issues/1",
		"not a url",
		"",
	}

	for _, url := range tests {
		t.Run(url, func(t *testing.T) {
			_, err := ParsePRURL(url)
			if err == nil {
				t.Errorf("expected error for URL %q", url)
			}
		})
	}
}
