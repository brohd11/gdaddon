package addon

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// fetchToStaging downloads (.zip) or clones (.git) the addon into a temporary
// staging directory and returns its content root: for zips, GitHub's single
// "repo-tag/" wrapper folder is unwrapped so the layout matches a git checkout.
// pkgName is the unwrapped folder's name when it is the author's package folder
// (used to derive the install dir for a path-less entry; see resolveInstall),
// or "" for git clones and synthetic source-archive wrappers. The returned
// cleanup removes all temp artifacts. ctx cancels the in-flight download/clone.
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

	// Unwrap a single top-level folder (GitHub archives wrap content in repo-tag/).
	// When that folder is the author's own package folder (not a synthetic
	// source-archive wrapper) its name is the addon's install dir for a path-less
	// entry, so surface it as pkgName.
	root, pkgName := extractDir, ""
	if entries, err := os.ReadDir(extractDir); err == nil && len(entries) == 1 && entries[0].IsDir() {
		root = filepath.Join(extractDir, entries[0].Name())
		if !isSourceArchiveURL(url) {
			pkgName = entries[0].Name()
		}
	}
	return root, pkgName, cleanup, nil
}

// isSourceArchiveURL reports whether url is a host-generated source/branch archive
// (e.g. GitHub ".../archive/refs/tags/<tag>.zip" / "…/refs/heads/<branch>.zip",
// Codeberg ".../archive/<tag>.zip"), whose top-level "repo-tag/" folder is synthetic
// and version-stamped — not a name to install under (a path-less entry falls back to
// the manifest name instead). It requires a remote http(s) url so it doesn't misfire
// on a local archived copy's file path, which lives under the ".../archive/" dir.
func isSourceArchiveURL(url string) bool {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return false
	}
	return strings.Contains(url, "/archive/")
}

// obtainZip returns a path to the zip to extract: a local archive file is used in
// place (cleanup is a no-op so the user's archive isn't deleted); a remote url is
// downloaded to a temp file (cleanup removes it).
func obtainZip(ctx context.Context, url, addonName string, report Reporter) (zipPath string, cleanup func(), err error) {
	if info, err := os.Stat(url); err == nil && !info.IsDir() {
		report("[%s] Using local archive %s...", addonName, url)
		return url, func() {}, nil
	}

	report("[%s] Downloading ZIP from %s...", addonName, url)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", func() {}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := http.DefaultClient.Do(req)
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
	if out, err := cmd.CombinedOutput(); err != nil {
		report("  -> Failed to clone %s:\n%s", addonName, string(out))
		cleanup()
		return "", func() {}, err
	}
	os.RemoveAll(filepath.Join(tempDir, ".git"))
	return tempDir, cleanup, nil
}

// gitCloneBranch clones url's <branch> directly into dest as a live working copy:
// full history, .git kept (unlike fetchGit, which shallow-clones to staging and
// strips .git). The parent dir is created first. ctx cancels the in-flight clone.
func gitCloneBranch(ctx context.Context, url, branch, dest, addonName string, report Reporter) error {
	report("[%s] Cloning %s (branch %s)...", addonName, url, branch)

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "git", "clone", "--branch", branch, url, dest)
	if out, err := cmd.CombinedOutput(); err != nil {
		report("  -> Failed to clone %s:\n%s", addonName, string(out))
		os.RemoveAll(dest)
		return err
	}
	return nil
}

// placement is one source folder in the staging tree and the project-root-relative
// path it should be installed to.
type placement struct {
	src     string
	destRel string
}

// pluginDirs returns every directory under root that holds an addon config file
// (see hasPluginCfg), pruned so a match nested inside another match is dropped (a
// sub-addon is managed by its parent addon, not installed on its own).
func pluginDirs(root string) []string {
	var dirs []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if hasPluginCfg(path) {
			dirs = append(dirs, path)
		}
		return nil
	})

	var pruned []string
	for _, d := range dirs {
		nested := false
		for _, other := range dirs {
			if other != d && isUnder(d, other) {
				nested = true
				break
			}
		}
		if !nested {
			pruned = append(pruned, d)
		}
	}
	return pruned
}

// isUnder reports whether path is a descendant of base.
func isUnder(path, base string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..")
}

// resolveInstall decides where the staged content should land, relative to the
// project root. The manifest path is authoritative only for a *submodule-style*
// package — one whose plugin.cfg sits at the staging root, so the whole tree is the
// addon ("path is king"). Otherwise the package is a container of one or more plugin
// folders and destinations are *derived*: a pinned path still applies to a single
// folder (so a relocation is honored), but it can no longer collapse a multi-folder
// bundle into one folder (which previously dumped the entire tree into the pinned
// path). Precedence stays definedPath > a dir= override (destFor) > addons/<name>.
//
// Derivation order:
//  1. submodule-style (plugin.cfg at the staging root) — the whole tree is the addon.
//  2. an addons/ folder anywhere in the tree — the canonical Godot layout: mirror its
//     immediate child folders into the project's addons/. This handles packages with
//     loose files beside the plugin folder(s) and packages with no plugin.cfg at all
//     (icon packs, asset libraries), which the config search alone would mis-derive.
//  3. otherwise locate plugin folders by their plugin.cfg/version.cfg and derive.
func resolveInstall(stagingRoot, name, definedPath, pkgName string) []placement {
	// rootName is the install dir basename when the package is installed whole (no
	// config, or its config is at the staging root): the author's package folder
	// name when known, else the manifest name.
	rootName := name
	if pkgName != "" {
		rootName = pkgName
	}

	// Submodule-style: the package root itself is the addon. Install the whole tree
	// to the pinned path, else addons/<name> (honoring a dir= override). Kept first:
	// a root config unambiguously means the repo *is* the addon, so any addons/
	// subfolder it bundles must ride along inside it rather than be extracted alone.
	if hasPluginCfg(stagingRoot) {
		return []placement{{src: stagingRoot, destRel: pathOr(definedPath, destFor(stagingRoot, DefaultPath(rootName)))}}
	}

	// An addons/ folder in the tree is the canonical layout: its immediate child
	// folders are the plugins. Derive from those, ignoring sibling junk (docs/,
	// .github/, README) and the GitHub wrapper name.
	if addonsDir := findAddonsDir(stagingRoot); addonsDir != "" {
		if ps := placementsForDirs(childDirs(addonsDir), definedPath); len(ps) > 0 {
			return ps
		}
	}

	dirs := pluginDirs(stagingRoot)
	if len(dirs) == 0 {
		// No config anywhere: install the whole tree (pinned path, else addons/<name>).
		return []placement{{src: stagingRoot, destRel: pathOr(definedPath, DefaultPath(rootName))}}
	}
	return placementsForDirs(dirs, definedPath)
}

// placementsForDirs maps a set of plugin folders to their install destinations with
// the derive precedence definedPath > a dir= override (destFor) > addons/<folder>.
// A single folder honors a pinned/relocated definedPath; a bundle of folders each
// derives its own addons/<folder> (the pinned path can't collapse them — installStaged
// overwrites only the entry's own folder and leaves bundled siblings, see
// primaryPlacement). Returns nil for an empty set.
func placementsForDirs(dirs []string, definedPath string) []placement {
	switch len(dirs) {
	case 0:
		return nil
	case 1:
		return []placement{{src: dirs[0], destRel: pathOr(definedPath, destFor(dirs[0], DefaultPath(filepath.Base(dirs[0]))))}}
	default:
		out := make([]placement, 0, len(dirs))
		for _, d := range dirs {
			out = append(out, placement{src: d, destRel: destFor(d, DefaultPath(filepath.Base(d)))})
		}
		return out
	}
}

// findAddonsDir returns the shallowest directory named "addons" anywhere under root,
// or "" when there is none. Shallowest wins so a submodule that bundles its own
// addons/ (e.g. addons/foo/addons/bar) resolves to the top-level addons/, not the
// nested one — and a found addons/ is not descended into for that reason.
func findAddonsDir(root string) string {
	best, bestDepth := "", -1
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if filepath.Base(path) != "addons" {
			return nil
		}
		depth := pathDepth(root, path)
		if best == "" || depth < bestDepth {
			best, bestDepth = path, depth
		}
		return filepath.SkipDir
	})
	return best
}

// pathDepth is the number of path segments from base to path (0 when equal).
func pathDepth(base, path string) int {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(os.PathSeparator)) + 1
}

// childDirs returns the immediate subdirectories of dir (absolute paths), or nil.
func childDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(dir, e.Name()))
		}
	}
	return dirs
}

// pathOr returns p when set, else the fallback — used to express the "explicit
// manifest path wins, otherwise derive" precedence in resolveInstall.
func pathOr(p, fallback string) string {
	if p != "" {
		return p
	}
	return fallback
}

// destFor returns a staged config dir's install destination: the addon's own
// `dir=` override declared in its config (see installDir) when present, else the
// derived fallback. An explicit manifest path still wins upstream in resolveInstall.
func destFor(dir, fallback string) string {
	if d := installDir(dir); d != "" {
		return d
	}
	return fallback
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if strings.HasPrefix(f.Name, "__MACOSX/") ||
			base == ".DS_Store" ||
			base == "Thumbs.db" ||
			base == "desktop.ini" ||
			base == "ehthumbs.db" {
			continue
		}
		fpath := filepath.Join(dest, filepath.Clean("/"+f.Name))
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", fpath)
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}
		out, err := os.Create(fpath)
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
