package gitcred

import (
	"context"
	"testing"
)

// TestCredentialHost checks the host→credential-host mapping: GitHub's API and
// archive subdomains collapse onto github.com (case-insensitively); other hosts
// pass through unchanged.
func TestCredentialHost(t *testing.T) {
	cases := map[string]string{
		"github.com":                "github.com",
		"api.github.com":            "github.com",
		"codeload.github.com":       "github.com",
		"raw.githubusercontent.com": "github.com",
		"GitHub.com":                "github.com",
		"codeberg.org":              "codeberg.org",
		"gitlab.com":                "gitlab.com",
	}
	for in, want := range cases {
		if got := credentialHost(in); got != want {
			t.Errorf("credentialHost(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestTokenGitHubEnv covers the deterministic env path: a non-empty GITHUB_TOKEN
// wins for any github.com credential host and returns before the cached git
// subprocess fallback. An unparseable URL yields no token.
//
// The non-GitHub Token path shells out to `git credential fill` and memoizes in an
// unexported package-level cache, so it's environment-dependent and intentionally
// left unasserted.
func TestTokenGitHubEnv(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "tok")
	ctx := context.Background()

	if got := Token(ctx, "api.github.com"); got != "tok" {
		t.Errorf("Token(api.github.com) = %q, want tok", got)
	}
	if got := TokenForURL(ctx, "https://codeload.github.com/u/r/zip/refs/tags/v1"); got != "tok" {
		t.Errorf("TokenForURL(codeload) = %q, want tok", got)
	}
	if got := TokenForURL(ctx, "://bad"); got != "" {
		t.Errorf("TokenForURL(unparseable) = %q, want empty", got)
	}
}
