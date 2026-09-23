// Package gitcred finds a bearer token for an HTTP host so gdaddon can reach private
// repos: GITHUB_TOKEN for github.com, else the user's git credential helpers. Standard
// library only.
package gitcred

import (
	"bufio"
	"context"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
)

var (
	cacheMu sync.Mutex
	cache   = map[string]string{}
)

// TokenForURL resolves rawURL's host and returns Token for it, or "" when the URL
// can't be parsed. A convenience for callers that hold a full URL string.
func TokenForURL(ctx context.Context, rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return Token(ctx, u.Hostname())
}

// Token returns host's bearer token or "": GITHUB_TOKEN for github.com, else `git
// credential fill`. Results (empty ones too) are cached per credential host.
func Token(ctx context.Context, host string) string {
	if host == "" {
		return ""
	}
	ch := credentialHost(host)
	if ch == "github.com" && os.Getenv("GITHUB_TOKEN") != "" {
		return os.Getenv("GITHUB_TOKEN")
	}

	cacheMu.Lock()
	if tok, ok := cache[ch]; ok {
		cacheMu.Unlock()
		return tok
	}
	cacheMu.Unlock()

	tok := gitCredentialFill(ctx, ch)

	cacheMu.Lock()
	cache[ch] = tok
	cacheMu.Unlock()
	return tok
}

// credentialHost maps GitHub's API and download subdomains to github.com, where the one
// credential is stored.
func credentialHost(host string) string {
	h := strings.ToLower(host)
	if h == "github.com" || strings.HasSuffix(h, ".github.com") ||
		strings.HasSuffix(h, ".githubusercontent.com") {
		return "github.com"
	}
	return host
}

// gitCredentialFill asks git's credential helpers for host's https password, with prompts
// disabled; errors yield "". It sets the variable itself rather than using repo.GitEnv to
// keep this package stdlib-only.
func gitCredentialFill(ctx context.Context, host string) string {
	cmd := exec.CommandContext(ctx, "git", "credential", "fill")
	cmd.Stdin = strings.NewReader("protocol=https\nhost=" + host + "\n\n")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if k, v, ok := strings.Cut(line, "="); ok && k == "password" {
			return v
		}
	}
	return ""
}
