// Package gitcred resolves a bearer token for an HTTP host so gdaddon can reach
// private repositories. It prefers the GITHUB_TOKEN environment variable (for
// github.com) and otherwise falls back to the user's git credential helpers via
// `git credential fill`, which read ~/.gitconfig (osxkeychain, gh, store, …). It
// names no gdaddon type and imports only the standard library, so any package can
// depend on it.
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

// Token returns a bearer token for host, or "" when none is available. The host
// is first normalized to its credential host (see credentialHost) so GitHub's API
// and archive subdomains share github.com's credential. For github.com a non-empty
// GITHUB_TOKEN wins; otherwise it runs `git credential fill` for protocol=https and
// returns the resolved password. Results (including empty ones) are cached per
// credential host since the fill spawns a git subprocess.
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

// credentialHost maps a request host to the host its credentials are stored under.
// GitHub serves the API (api.github.com) and archive downloads (codeload.github.com,
// *.githubusercontent.com) from sibling subdomains, but a single token / keychain
// entry lives under github.com — so all of them resolve there.
func credentialHost(host string) string {
	h := strings.ToLower(host)
	if h == "github.com" || strings.HasSuffix(h, ".github.com") ||
		strings.HasSuffix(h, ".githubusercontent.com") {
		return "github.com"
	}
	return host
}

// gitCredentialFill asks git's configured credential helpers for host's https
// password. GIT_TERMINAL_PROMPT=0 keeps it from blocking on an interactive prompt
// when no helper can answer; any error yields "".
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
