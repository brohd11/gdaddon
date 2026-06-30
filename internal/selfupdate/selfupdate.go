// Package selfupdate checks whether a newer gdaddon release than the running binary
// exists and installs it over the current one. It mirrors the per-addon update check
// (internal/addon CheckUpdate) but targets gdaddon's own repo: it fetches the repo's
// release listing via internal/source, compares the running version against the
// latest tag with the same semver rules addons use (addon.SemverGE), and — when an
// update is available — downloads the platform release zip and places the new binary
// with internal/installer's copy/PATH/elevation logic.
package selfupdate

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/installer"
	"gdaddon/internal/source"
)

// RepoURL is gdaddon's own repository — the source of self-update releases. A plain
// host/owner/repo URL, which source.AvailableVersions parses like any addon URL.
const RepoURL = "https://github.com/brohd11/gdaddon"

// Info is the result of a self-update check: the running version, the latest release
// tag, and — when an update for this platform is available — the asset to download.
type Info struct {
	Current   string
	LatestTag string
	Available bool
	AssetURL  string
	AssetName string
}

// Check fetches gdaddon's release listing and compares the running version against
// the latest release tag. Available is true only when current is a comparable semver
// strictly older than the latest tag and a release asset exists for this platform.
// A non-comparable version (a plain "dev" build), an already-current binary, a
// missing platform asset, or no releases all yield Available=false with no error, so
// no false update is ever surfaced. A fetch failure returns the error.
func Check(ctx context.Context, current string) (Info, error) {
	info := Info{Current: current}
	listing, err := source.AvailableVersions(ctx, RepoURL)
	if err != nil {
		return info, err
	}
	if listing == nil {
		return info, nil
	}
	latest, ok := latestRelease(listing.Releases)
	if !ok {
		return info, nil
	}
	info.LatestTag = latest.Tag

	ge, comparable := addon.SemverGE(current, latest.Tag)
	if !comparable || ge {
		// "dev" build (uncomparable) or already on/ahead of the latest tag.
		return info, nil
	}

	asset, ok := platformAsset(latest)
	if !ok {
		return info, nil
	}
	info.Available = true
	info.AssetURL = asset.URL
	info.AssetName = asset.Name
	return info, nil
}

// Apply downloads info's platform asset and installs the new binary to dest,
// reporting progress lines. It returns the installed path. Not available / no asset
// is an error (the caller should only Apply a result Check reported available).
func Apply(ctx context.Context, info Info, dest installer.Dest, report func(string, ...any)) (string, error) {
	if !info.Available || info.AssetURL == "" {
		return "", fmt.Errorf("no gdaddon update available")
	}
	report("Downloading %s...", info.AssetName)
	binPath, cleanup, err := downloadBinary(ctx, info.AssetURL)
	if err != nil {
		return "", err
	}
	defer cleanup()

	report("Installing %s to %s...", info.LatestTag, dest)
	res, err := installer.InstallFrom(dest, binPath)
	if err != nil {
		return "", err
	}
	if res.Note != "" {
		report("%s", res.Note)
	}
	return res.Path, nil
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

// platformAsset picks the release asset matching this OS/arch by name token
// ("darwin-arm64" / "linux-amd64" / "windows-amd64", as produced by the Makefile's
// package-* targets), ignoring the host-generated source archive. ok is false when
// no uploaded asset matches the running platform.
func platformAsset(rel source.Release) (source.Asset, bool) {
	token := runtime.GOOS + "-" + runtime.GOARCH
	for _, a := range rel.Assets {
		if a.Generated {
			continue
		}
		if strings.Contains(strings.ToLower(a.Name), token) {
			return a, true
		}
	}
	return source.Asset{}, false
}

// latestRelease picks the newest non-prerelease (releases come newest-first),
// falling back to the newest release when every one is a prerelease. Mirrors the
// unexported addon.latestRelease.
func latestRelease(releases []source.Release) (source.Release, bool) {
	if len(releases) == 0 {
		return source.Release{}, false
	}
	for _, r := range releases {
		if !r.Prerelease {
			return r, true
		}
	}
	return releases[0], true
}

// downloadBinary fetches the release zip and extracts just the gdaddon binary entry
// to a temp file, returning its path and a cleanup that removes the temp artifacts.
// ctx cancels the in-flight download.
func downloadBinary(ctx context.Context, url string) (binPath string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "gdaddon-selfupdate-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup = func() { os.RemoveAll(tmpDir) }

	zipPath := filepath.Join(tmpDir, "release.zip")
	if err := downloadFile(ctx, url, zipPath); err != nil {
		cleanup()
		return "", func() {}, err
	}

	binPath, err = extractBinary(zipPath, tmpDir)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	return binPath, cleanup, nil
}

// downloadFile streams url to dst, erroring on a non-2xx response.
func downloadFile(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// extractBinary writes the zip's gdaddon binary entry (matched by base name) into
// destDir as an executable and returns its path.
func extractBinary(zipPath, destDir string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	want := installer.ExeName()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || path.Base(f.Name) != want {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		dst := filepath.Join(destDir, want)
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			rc.Close()
			return "", err
		}
		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			return "", err
		}
		rc.Close()
		if err := out.Close(); err != nil {
			return "", err
		}
		return dst, nil
	}
	return "", fmt.Errorf("no %s binary found in release archive", want)
}
