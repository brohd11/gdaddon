package addon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// InstallOutcome records one addon that was actually installed in a batch run, so a
// caller can follow up per addon (the TUI's post-install location form). PriorPath is
// the entry's manifest path before the install (empty for a path-less first install),
// Path/Version the resolved result, URL the entry's source url. Only single-folder
// installs (a pinnable Path) produce an outcome.
type InstallOutcome struct {
	Name      string
	URL       string
	PriorPath string
	Path      string
	Version   string
}

// InstallAll applies the manifest's skip/update policy: already-installed and
// unversioned-present entries are skipped, mismatches are updated. After a
// successful install it pins the resolved path + version back into the manifest
// (entries start url-only; this records where they landed). It returns one
// InstallOutcome per addon actually installed (to a single, pinnable folder).
func InstallAll(ctx context.Context, manifestPath string, statuses []Status, baseDir string, report Reporter) ([]InstallOutcome, error) {
	var outcomes []InstallOutcome
	for _, s := range statuses {
		a := s.Addon
		switch s.State {
		case StateInvalid:
			report("Skipping %s: missing 'url'", a.Name)
			continue
		case StateInstalled:
			report("[%s] v%s is already installed. Skipping...", a.Name, s.LocalVersion)
			continue
		case StateUnversioned:
			report("[%s] already exists at %s (no version specified). Skipping...", a.Name, a.Path)
			continue
		case StateMismatch:
			old := s.LocalVersion
			if old == "" {
				old = "Unknown/None"
			}
			report("[%s] Version mismatch! Local is %s, YAML wants %s. Updating...", a.Name, old, a.Version)
		}

		res, err := Install(ctx, a, baseDir, report)
		if err != nil {
			report("[%s] Error: %v", a.Name, err)
			continue
		}
		if res.Path != "" {
			_ = UpdateEntry(manifestPath, a.Name, "", res.Path, res.Version, "")
			outcomes = append(outcomes, InstallOutcome{
				Name: a.Name, URL: a.URL, PriorPath: a.Path, Path: res.Path, Version: res.Version,
			})
		}
	}
	return outcomes, nil
}

// InstallResult reports where a single addon landed so the manifest entry can be
// pinned. Path is the project-root-relative install path and Version is read from
// the installed plugin.cfg; both are empty when the install can't be tracked to a
// single folder (a package that ships several top-level addons).
type InstallResult struct {
	Path    string
	Version string
}

// Install fetches a single addon and installs it under baseDir. The destination
// is the entry's explicit path when set, otherwise it's derived from the
// package's plugin.cfg layout (see resolveInstall). Existing folders at each
// destination are replaced.
func Install(ctx context.Context, a Addon, baseDir string, report Reporter) (InstallResult, error) {
	if a.URL == "" {
		return InstallResult{}, fmt.Errorf("missing 'url'")
	}

	if a.Clone {
		return cloneInstall(ctx, a, baseDir, report)
	}

	stagingRoot, pkgName, cleanup, err := fetchToStaging(ctx, a.URL, a.Name, report)
	if err != nil {
		return InstallResult{}, err
	}
	defer cleanup()

	placements := resolveInstall(stagingRoot, a.Name, a.Path, pkgName)
	for _, p := range placements {
		dest, err := filepath.Abs(filepath.Join(baseDir, p.destRel))
		if err != nil {
			return InstallResult{}, fmt.Errorf("could not resolve path: %w", err)
		}
		os.RemoveAll(dest)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return InstallResult{}, err
		}
		if err := copyDir(p.src, dest); err != nil {
			return InstallResult{}, err
		}
		report("  -> Successfully installed to %s", p.destRel)
	}

	// Only a single-folder install can be pinned to a path/version.
	if len(placements) == 1 {
		dest := filepath.Join(baseDir, placements[0].destRel)
		return InstallResult{Path: placements[0].destRel, Version: getLocalPluginVersion(dest)}, nil
	}
	return InstallResult{}, nil
}

// cloneInstall installs a clone entry: it git-clones the repo (full history, the
// branch named in a.Tag, .git kept) to the entry's path so it's a live working
// copy. The whole repo is placed at the path, so it suits repos whose root is the
// addon itself. An already-present checkout is left untouched — never overwritten —
// so uncommitted development work is safe across re-runs.
func cloneInstall(ctx context.Context, a Addon, baseDir string, report Reporter) (InstallResult, error) {
	destRel := a.Path
	if destRel == "" {
		destRel = DefaultPath(a.Name)
	}
	dest, err := filepath.Abs(filepath.Join(baseDir, destRel))
	if err != nil {
		return InstallResult{}, fmt.Errorf("could not resolve path: %w", err)
	}

	if _, err := os.Stat(dest); err == nil {
		report("[%s] Already cloned at %s. Skipping (manage updates with git).", a.Name, destRel)
		return InstallResult{Path: destRel, Version: getLocalPluginVersion(dest)}, nil
	}

	if err := gitCloneBranch(ctx, a.URL, a.Tag, dest, a.Name, report); err != nil {
		return InstallResult{}, err
	}
	report("  -> Successfully cloned to %s", destRel)
	return InstallResult{Path: destRel, Version: getLocalPluginVersion(dest)}, nil
}

// Relocate moves an installed addon directory from fromRel to toRel (both
// project-root-relative), creating the destination's parent. It fails if the
// destination already exists — the caller decides what to do about a clash rather
// than silently overwriting. Used by the post-install "confirm location" form to
// honor a corrected path without re-downloading; a plain os.Rename moves a normal
// install or a clone (.git and all) alike.
func Relocate(root, fromRel, toRel string) error {
	from, err := filepath.Abs(filepath.Join(root, fromRel))
	if err != nil {
		return err
	}
	to, err := filepath.Abs(filepath.Join(root, toRel))
	if err != nil {
		return err
	}
	if from == to {
		return nil
	}
	if _, err := os.Stat(to); err == nil {
		return fmt.Errorf("destination already exists: %s", toRel)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	return os.Rename(from, to)
}

// Uninstall deletes an addon's installed files under baseDir, symmetric with
// Install. The location is the entry's recorded path; it is lenient — an empty
// path (nothing recorded) or an already-absent directory is a no-op — so a
// "remove + delete files" action is safe even when the addon isn't installed. The
// manifest entry is removed separately via RemoveEntry.
func Uninstall(a Addon, baseDir string) error {
	if a.Path == "" {
		return nil
	}
	fullPath, err := filepath.Abs(filepath.Join(baseDir, a.Path))
	if err != nil {
		return fmt.Errorf("could not resolve path: %w", err)
	}
	return os.RemoveAll(fullPath)
}
