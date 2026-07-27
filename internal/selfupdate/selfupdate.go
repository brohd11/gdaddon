// Package selfupdate implements gdaddon's self-update: check GitHub for a newer
// release and, when one exists, install it by running the repo's own install.sh —
// the same script the README tells users to curl — with BIN_DIR pointed at the
// destination's directory so the update lands in place, and --no-modify-path so an
// installed binary is never asked about PATH again.
package selfupdate

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"gdaddon/internal/installer"
)

// Repo is the GitHub repository the releases and install.sh are fetched from.
const Repo = "brohd11/gdaddon"

// latestReleaseURL is a var so tests can point it at an httptest server.
var latestReleaseURL = "https://github.com/" + Repo + "/releases/latest"

// installScriptURL is the raw install.sh the README's curl command pipes to sh.
const installScriptURL = "https://raw.githubusercontent.com/" + Repo + "/main/install.sh"

// Info is the outcome of Check: the running version, the latest release tag,
// and whether the release is newer.
type Info struct {
	Current   string
	LatestTag string
	Available bool
}

// Check resolves the latest release tag via the /releases/latest redirect —
// github.com/<repo>/releases/latest answers 302 to /releases/tag/<tag>, which
// gives the tag without touching the rate-limited API — and reports whether it
// is newer than current. A "dev" build is never comparable, hence never
// offered an update.
func Check(ctx context.Context, current string) (Info, error) {
	info := Info{Current: current}
	tag, err := latestTag(ctx)
	if err != nil {
		return info, err
	}
	info.LatestTag = tag
	info.Available = newer(normalize(current), tag)
	return info, nil
}

// Apply installs info.LatestTag by downloading install.sh and running it with
// BIN_DIR set to dest's directory and VERSION pinned to the checked tag, so it
// installs exactly what Check saw. Overwriting the running binary is safe:
// install.sh stages in a temp dir and mv -f's into place. The script's output
// is streamed line-by-line to report. Returns the installed binary's path.
func Apply(ctx context.Context, info Info, dest installer.Dest, report func(string, ...any)) (string, error) {
	if report == nil {
		report = func(string, ...any) {}
	}
	dir, err := dest.Dir()
	if err != nil {
		return "", err
	}

	tmp, err := os.MkdirTemp("", "gdaddon-selfupdate-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	script := filepath.Join(tmp, "install.sh")

	// Download to a file rather than curling into sh: the env vars and the
	// --no-modify-path flag are needed either way, and this keeps a failed
	// download distinct from a failed install.
	dl := exec.CommandContext(ctx, "curl", "-fsSL", "-o", script, installScriptURL)
	if out, err := dl.CombinedOutput(); err != nil {
		return "", fmt.Errorf("downloading install.sh: %w\n%s", err, out)
	}

	cmd := exec.CommandContext(ctx, "sh", script, "--no-modify-path")
	cmd.Env = append(os.Environ(),
		"BIN_DIR="+dir,
		"VERSION="+info.LatestTag,
	)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	scanDone := make(chan struct{})
	go func() {
		defer close(scanDone)
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			report("%s", sc.Text())
		}
	}()

	runErr := cmd.Run()
	pw.Close()
	<-scanDone
	if runErr != nil {
		return "", fmt.Errorf("running install.sh: %w", runErr)
	}
	return filepath.Join(dir, installer.ExeName()), nil
}

// DefaultDest is the install target self-update uses without an explicit choice: the
// managed location the running binary already occupies, falling back to the
// gdaddon-home location the Godot plugin launches when the running binary isn't a
// managed install.
func DefaultDest() installer.Dest {
	if d, ok := installer.CurrentDest(); ok {
		return d
	}
	return installer.Home
}

// latestTag GETs the /releases/latest URL without following the redirect and
// reads the tag off the Location header.
func latestTag(ctx context.Context) (string, error) {
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	loc := resp.Header.Get("Location")
	const marker = "/releases/tag/"
	i := strings.Index(loc, marker)
	if i < 0 {
		return "", fmt.Errorf("no release tag in redirect from %s (status %s)", latestReleaseURL, resp.Status)
	}
	return loc[i+len(marker):], nil
}

// normalize reduces a git describe version to its base tag: any dash in the
// output introduces the -N-g<hash>[-dirty] suffix, e.g. "v0.1.1-2-gdfdcacf"
// becomes "v0.1.1". "dev" stays "dev" and is never comparable.
func normalize(v string) string {
	if i := strings.Index(v, "-"); i >= 0 {
		v = v[:i]
	}
	return v
}

// newer reports whether latest is a higher semver tag than current. Anything
// unparseable ("dev", malformed tags) is treated as not newer — better to skip
// an update than to reinstall on a misunderstanding.
func newer(current, latest string) bool {
	c, okC := parseSemver(current)
	l, okL := parseSemver(latest)
	if !okC || !okL {
		return false
	}
	for i := range c {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// parseSemver parses a vX.Y.Z tag into its numeric components.
func parseSemver(tag string) ([3]int, bool) {
	var v [3]int
	parts := strings.Split(strings.TrimPrefix(tag, "v"), ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, false
		}
		v[i] = n
	}
	return v, true
}
