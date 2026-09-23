package addon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/brohd11/gdaddon/internal/store"
)

// InstallOutcome is one addon installed in a batch run to a single folder, for per-addon
// follow-up (the TUI's location form). PriorPath is the manifest path before install.
type InstallOutcome struct {
	Name string
	// Display is the entry's label after install: its recorded name, else the one the package
	// declared.
	Display   string
	URL       string
	PriorPath string
	Path      string
	Version   string
}

// Label is the outcome's human-facing name, on the same rule as Addon.Label: the addon's
// own name when it has one, else its key's slug.
func (o InstallOutcome) Label() string {
	return Addon{Name: o.Name, Display: o.Display}.Label()
}

// displayOf is an outcome's label: the entry's name, else the freshly declared one (the
// in-memory entry predates AdoptName's write).
func displayOf(a Addon, res InstallResult) string {
	if a.Display != "" {
		return a.Display
	}
	return sanitizeDisplay(res.Name)
}

// InstallAll applies the manifest's policy: skip installed and unversioned-present entries,
// update mismatches, and pin each result's path and version. It returns an outcome per
// single-folder install.
func InstallAll(ctx context.Context, manifestPath string, statuses []Status, baseDir string, report Reporter) ([]InstallOutcome, error) {
	var outcomes []InstallOutcome
	for _, s := range statuses {
		a := s.Addon
		switch s.State {
		case StateInvalid:
			report("Skipping %s: missing 'url'", a.Label())
			continue
		case StateInstalled:
			report("[%s] v%s is already installed. Skipping...", a.Label(), s.LocalVersion)
			continue
		case StateUnversioned:
			report("[%s] already exists at %s (no version specified). Skipping...", a.Label(), a.Path)
			continue
		case StateBranchChanged:
			// A git checkout on a different branch is never touched by a batch install; report it
			// ("Update branch record" re-records the branch).
			report("[%s] branch changed (recorded %s, on %s). Skipping...", a.Label(), a.Tag, s.LiveBranch)
			continue
		case StateMismatch:
			old := s.LocalVersion
			if old == "" {
				old = "Unknown/None"
			}
			report("[%s] Version mismatch! Local is %s, YAML wants %s. Updating...", a.Label(), old, a.Version)
		}

		res, err := Install(ctx, a, baseDir, report)
		if err != nil {
			report("[%s] Error: %v", a.Label(), err)
			continue
		}
		if res.Path != "" {
			_, outcome, err := recordInstall(manifestPath, baseDir, a, res, func(err error) {
				report("[%s] Could not record the declared name: %v", a.Label(), err)
			})
			if err != nil {
				report("[%s] Error pinning manifest: %v", a.Label(), err)
				continue
			}
			outcomes = append(outcomes, outcome)
		}
	}
	return outcomes, nil
}

// InstallResult is what an install produced: the project-relative Path, and Version and
// Name read from the installed config. All empty when the package installs several
// folders; Name empty when none is declared.
type InstallResult struct {
	Path    string
	Version string
	Name    string
}

// installedAt reads what landed at dest for entry path destRel; every install path ends
// here, so results are read the same way.
func installedAt(destRel, dest string) InstallResult {
	return InstallResult{
		Path:    destRel,
		Version: getLocalPluginVersion(dest),
		Name:    getLocalPluginName(dest),
	}
}

// AdoptName records a package's declared name on its manifest entry and matching global
// entry, only where neither has a name yet. It never renames keys or overwrites names, so
// every install path can call it; it is the one place an install sets an entry's label.
func AdoptName(manifestPath string, a Addon, res InstallResult) error {
	display := sanitizeDisplay(res.Name)
	if display == "" {
		return nil
	}
	setGlobalDisplayName(a.URL, display)
	if a.Display != "" {
		return nil
	}
	return SetDisplayName(manifestPath, a.Name, display)
}

// sanitizeDisplay trims a declared name, returning "" for names with control characters.
func sanitizeDisplay(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsFunc(name, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return ""
	}
	return name
}

// Install fetches one addon and installs it under baseDir, at the entry's path or one
// derived from the package layout (resolveInstall), replacing existing folders.
func Install(ctx context.Context, a Addon, baseDir string, report Reporter) (InstallResult, error) {
	if a.URL == "" {
		return InstallResult{}, fmt.Errorf("missing 'url'")
	}

	// A submodule's checkout is owned by the parent repo; gdaddon registers it for the
	// utility actions but must never install or overwrite it.
	if a.IsSubmodule() {
		return InstallResult{}, fmt.Errorf("%q is a submodule, managed by the parent repo; not installable", a.Label())
	}

	if a.IsClone() {
		return cloneInstall(ctx, a, baseDir, report)
	}

	// Asset Store entries pin a canonical store URL, not a git/zip url; resolve the
	// store-hosted download and install it (see storeInstall).
	if store.IsStoreURL(a.URL) {
		return storeInstall(ctx, a, baseDir, report)
	}

	stagingRoot, pkgName, cleanup, err := fetchToStaging(ctx, a.URL, a.Label(), report)
	if err != nil {
		return InstallResult{}, err
	}
	defer cleanup()

	return installStaged(stagingRoot, pkgName, a, baseDir, report)
}

// installStaged places staged content under baseDir. A single folder is overwritten and
// pinned. For a bundle of several plugin folders, only the entry's own folder is
// overwritten and pinned; the others are written only if absent, so a bundled copy never
// replaces a separately managed plugin.
func installStaged(stagingRoot, pkgName string, a Addon, baseDir string, report Reporter) (InstallResult, error) {
	placements := resolveInstall(stagingRoot, a.Slug(), a.Path, pkgName)

	if len(placements) == 1 {
		if err := writePlacement(placements[0], baseDir, report); err != nil {
			return InstallResult{}, err
		}
		dest := filepath.Join(baseDir, placements[0].destRel)
		stampVersion(dest, intendedVersion(a), canonicalRepoURL(a.URL))
		return installedAt(placements[0].destRel, dest), nil
	}

	primary := primaryPlacement(placements, a)
	var res InstallResult
	for i, p := range placements {
		if i == primary {
			if err := writePlacement(p, baseDir, report); err != nil {
				return InstallResult{}, err
			}
			dest := filepath.Join(baseDir, p.destRel)
			stampVersion(dest, intendedVersion(a), canonicalRepoURL(a.URL))
			res = installedAt(p.destRel, dest)
			continue
		}
		// Bundled extra: never overwrite an existing folder (it may be a plugin the
		// user installs/manages on its own); only write it when absent.
		if _, err := os.Stat(filepath.Join(baseDir, p.destRel)); err == nil {
			report("  -> skipped %s (already present; manage separately)", p.destRel)
			continue
		}
		if err := writePlacement(p, baseDir, report); err != nil {
			return InstallResult{}, err
		}
	}
	return res, nil
}

// writePlacement replaces the folder at p.destRel with p.src. Every install passes
// through here, so it enforces project-root containment (see resolveUnder).
func writePlacement(p placement, baseDir string, report Reporter) error {
	dest, err := resolveUnder(baseDir, p.destRel)
	if err != nil {
		return err
	}
	os.RemoveAll(dest)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := copyDir(p.src, dest); err != nil {
		return err
	}
	report("  -> Successfully installed to %s", p.destRel)
	return nil
}

// primaryPlacement returns the placement that is the entry's own addon (folder name
// matching its name or path), or -1 when ambiguous (nothing pinned, all written only if
// absent).
func primaryPlacement(placements []placement, a Addon) int {
	want := map[string]bool{}
	// The entry's slug, not its key: an identity-keyed entry
	// (github.com/owner/repo) still matches the folder named repo.
	if s := a.Slug(); s != "" {
		want[s] = true
	}
	if a.Path != "" {
		want[filepath.Base(a.Path)] = true
	}
	for i, p := range placements {
		if want[filepath.Base(p.destRel)] {
			return i
		}
	}
	return -1
}

// cloneInstall clones the repo (full history, branch a.Tag, .git kept) to the entry's path
// as a working copy. An existing checkout is never overwritten.
func cloneInstall(ctx context.Context, a Addon, baseDir string, report Reporter) (InstallResult, error) {
	destRel := a.Path
	if destRel == "" {
		destRel = DefaultPath(a.Slug())
	}
	dest, err := filepath.Abs(filepath.Join(baseDir, destRel))
	if err != nil {
		return InstallResult{}, fmt.Errorf("could not resolve path: %w", err)
	}

	if _, err := os.Stat(dest); err == nil {
		if isGitCheckout(dest) {
			// No clone needed, but a path-less entry may still sit at the wrong place: settle it
			// against the checkout's declared path once.
			if a.Path == "" {
				destRel, dest = settleDeclaredPath(destRel, dest, baseDir, a.Label(), false, report)
			}
			report("[%s] Already cloned at %s. Skipping (manage updates with git).", a.Label(), destRel)
			return installedAt(destRel, dest), nil
		}
		// Replace a non-git folder (a former package install) with the clone.
		report("[%s] Replacing non-git folder at %s with a fresh clone.", a.Label(), destRel)
		if err := os.RemoveAll(dest); err != nil {
			return InstallResult{}, fmt.Errorf("could not remove existing folder %s: %w", destRel, err)
		}
	}

	if err := gitCloneBranch(ctx, a.URL, a.Tag, dest, a.Label(), report); err != nil {
		return InstallResult{}, err
	}
	report("  -> Successfully cloned to %s", destRel)

	// A clone's declared install path is only readable after cloning, so settle a derived
	// destination afterwards, keeping the precedence: manifest path, declared dir/path,
	// addons/<name>.
	if a.Path == "" {
		destRel, dest = settleDeclaredPath(destRel, dest, baseDir, a.Label(), true, report)
	}
	return installedAt(destRel, dest), nil
}

// settleDeclaredPath moves a fresh clone to the path its config declares, returning where
// it ended up. An existing checkout at the target is never overwritten; a non-git folder
// there is replaced. Failures are reported and the current location kept. fresh reports
// whether the checkout was just cloned: only a fresh clone may be discarded on a clash.
func settleDeclaredPath(fromRel, from, baseDir, name string, fresh bool, report Reporter) (string, string) {
	declared := installDir(from)
	if declared == "" {
		return fromRel, from
	}
	to, err := resolveUnder(baseDir, declared)
	if err != nil {
		report("  -> Keeping %s at %s: %v", name, fromRel, err)
		return fromRel, from
	}
	// Compared resolved, not as written: the declared value is the author's spelling of
	// the path the derivation already produced as often as not.
	if to == from {
		return fromRel, from
	}

	if _, err := os.Stat(to); err == nil {
		if isGitCheckout(to) {
			if !fresh {
				report("[%s] Declared path %s already holds a checkout; keeping %s.", name, declared, fromRel)
				return fromRel, from
			}
			report("[%s] Already cloned at %s; discarding the copy at %s.", name, declared, fromRel)
			os.RemoveAll(from)
			return declared, to
		}
		report("[%s] Replacing non-git folder at %s with the clone.", name, declared)
		if err := os.RemoveAll(to); err != nil {
			report("  -> Keeping %s at %s: %v", name, fromRel, err)
			return fromRel, from
		}
	}

	if err := Relocate(baseDir, fromRel, declared); err != nil {
		report("  -> Keeping %s at %s: %v", name, fromRel, err)
		return fromRel, from
	}
	report("  -> Moved to %s (declared by the addon)", declared)
	return declared, to
}

// Relocate moves an installed addon from fromRel to toRel (project-relative), creating the
// parent and failing if the target exists. Used by the post-install location form.
func Relocate(root, fromRel, toRel string) error {
	from, err := resolveUnder(root, fromRel)
	if err != nil {
		return err
	}
	to, err := resolveUnder(root, toRel)
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

// Uninstall deletes an addon's files at its recorded path; an empty path or missing
// directory is a no-op. RemoveEntry removes the manifest entry.
func Uninstall(a Addon, baseDir string) error {
	if a.Path == "" {
		return nil
	}
	fullPath, err := resolveUnder(baseDir, a.Path)
	if err != nil {
		return err
	}
	return os.RemoveAll(fullPath)
}
