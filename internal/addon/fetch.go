package addon

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/brohd11/gdaddon/internal/restrule"

	"github.com/brohd11/gitstack/repo"
)

// fetchToStaging downloads (.zip) or clones (.git) the addon into a temp directory and
// returns its content root, unwrapping a zip's wrapper folder. pkgName is that folder's
// name when it is the author's package folder (for deriving a path-less install), else "".
// cleanup removes everything; ctx cancels.
func fetchToStaging(ctx context.Context, url, addonName string, report Reporter) (stagingRoot, pkgName string, cleanup func(), err error) {
	switch {
	case strings.HasSuffix(url, ".zip"):
		return fetchZip(ctx, url, addonName, report)
	case strings.HasSuffix(url, ".git"):
		root, clean, err := fetchGit(ctx, url, addonName, report)
		return root, "", clean, err
	default:
		return "", "", func() {}, fmt.Errorf("URL must end in '.zip' or '.git'. Found: %s", url)
	}
}

func fetchZip(ctx context.Context, url, addonName string, report Reporter) (string, string, func(), error) {
	zipPath, zipCleanup, err := obtainZip(ctx, url, addonName, report)
	if err != nil {
		return "", "", func() {}, err
	}
	defer zipCleanup()

	extractDir, err := os.MkdirTemp("", "godot-addon-extract-*")
	if err != nil {
		return "", "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(extractDir) }

	report("[%s] Extracting...", addonName)
	if err := unzip(zipPath, extractDir); err != nil {
		cleanup()
		return "", "", func() {}, err
	}

	// Unwrap a single top-level folder that wraps the package (isWrapperDir). When it is the
	// author's package folder, not a synthetic archive wrapper, report its name as pkgName.
	root, pkgName := extractDir, ""
	if entries, err := os.ReadDir(extractDir); err == nil && len(entries) == 1 && entries[0].IsDir() {
		if dir := filepath.Join(extractDir, entries[0].Name()); isWrapperDir(dir, url) {
			root = dir
			if !isSourceArchiveURL(url) {
				pkgName = entries[0].Name()
			}
		}
	}
	return root, pkgName, cleanup, nil
}

// isWrapperDir reports whether a zip's single top-level folder is a wrapper (strip it)
// rather than a real layout level like addon_lib/ (keep it). They look identical, so it
// relies on the url (a host-generated archive) or a version-stamped folder name; unknown
// folders are kept.
func isWrapperDir(dir, url string) bool {
	switch {
	case isSourceArchiveURL(url):
		return true // host-generated repo-tag/: synthetic and version-stamped
	case hasPluginCfg(dir):
		return true // the folder *is* the addon (script_tabs/plugin.cfg)
	case len(pluginDirs(dir)) == 0:
		return true // no config anywhere: an asset pack, and the folder is the pack
	default:
		return versionStamped.MatchString(filepath.Base(dir))
	}
}

// versionStamped matches a release asset's version-stamped wrapper folder
// (MyPlugin-1.2.3, foo_v2.0) and not a plain package folder (addon_lib).
var versionStamped = regexp.MustCompile(`(^|[-_.])v?\d+(\.\d+)+`)

// isSourceArchiveURL reports a host-generated source or branch archive (GitHub
// archive/refs/..., Codeberg archive/...), whose wrapper name is synthetic. Remote urls
// only, so a local archived copy's path does not match.
func isSourceArchiveURL(url string) bool {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return false
	}
	return strings.Contains(url, "/archive/")
}

// obtainZip returns the zip to extract: a local archive in place (cleanup does nothing), or
// a download to a temp file.
func obtainZip(ctx context.Context, url, addonName string, report Reporter) (zipPath string, cleanup func(), err error) {
	if info, err := os.Stat(url); err == nil && !info.IsDir() {
		report("[%s] Using local archive %s...", addonName, url)
		return url, func() {}, nil
	}

	report("[%s] Downloading ZIP from %s...", addonName, url)
	resp, err := restrule.Get(ctx, url)
	if err != nil {
		return "", func() {}, err
	}
	defer resp.Body.Close()

	tmpFile, err := os.CreateTemp("", "godot-addon-*.zip")
	if err != nil {
		return "", func() {}, err
	}
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", func() {}, err
	}
	tmpFile.Close()
	return tmpFile.Name(), func() { os.Remove(tmpFile.Name()) }, nil
}

func fetchGit(ctx context.Context, url, addonName string, report Reporter) (string, func(), error) {
	report("[%s] Cloning repository...", addonName)

	tempDir, err := os.MkdirTemp("", "godot-addon-clone-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(tempDir) }

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", url, tempDir)
	cmd.Env = repo.GitEnv() // never prompt for credentials behind the TUI
	if out, err := cmd.CombinedOutput(); err != nil {
		report("  -> Failed to clone %s:\n%s", addonName, string(out))
		cleanup()
		return "", func() {}, err
	}
	os.RemoveAll(filepath.Join(tempDir, ".git"))
	return tempDir, cleanup, nil
}

// gitCloneBranch clones url's branch into dest as a full working copy (.git kept). An empty
// branch uses the remote default; the caller then records the branch it got
// (CurrentBranch), or the entry reads as drifted. ctx cancels.
func gitCloneBranch(ctx context.Context, url, branch, dest, addonName string, report Reporter) error {
	args := []string{"clone"}
	if branch == "" {
		report("[%s] Cloning %s (default branch)...", addonName, url)
	} else {
		report("[%s] Cloning %s (branch %s)...", addonName, url, branch)
		args = append(args, "--branch", branch)
	}
	args = append(args, url, dest)

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = repo.GitEnv() // never prompt for credentials behind the TUI
	if out, err := cmd.CombinedOutput(); err != nil {
		report("  -> Failed to clone %s:\n%s", addonName, string(out))
		os.RemoveAll(dest)
		return err
	}
	return nil
}
