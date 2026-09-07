package addon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/brohd11/gdaddon/internal/store"
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
		case StateBranchChanged:
			// A git checkout on a different branch than the manifest records: a present git
			// workdir is never touched by a batch install. Report the drift and skip;
			// reconcile (re-record the tag) is the explicit per-addon "Update branch record".
			report("[%s] branch changed (recorded %s, on %s). Skipping...", a.Name, a.Tag, s.LiveBranch)
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
			if err := UpdateEntry(manifestPath, a.Name, "", res.Path, res.Version, ""); err != nil {
				report("[%s] Error pinning manifest: %v", a.Name, err)
				continue
			}
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

	// A submodule's checkout is owned by the parent repo; gdaddon registers it for the
	// utility actions but must never install or overwrite it.
	if a.IsSubmodule() {
		return InstallResult{}, fmt.Errorf("%q is a submodule, managed by the parent repo; not installable", a.Name)
	}

	if a.IsClone() {
		return cloneInstall(ctx, a, baseDir, report)
	}

	// Asset Store entries pin a canonical store URL, not a git/zip url; resolve the
	// store-hosted download and install it (see storeInstall).
	if store.IsStoreURL(a.URL) {
		return storeInstall(ctx, a, baseDir, report)
	}

	stagingRoot, pkgName, cleanup, err := fetchToStaging(ctx, a.URL, a.Name, report)
	if err != nil {
		return InstallResult{}, err
	}
	defer cleanup()

	return installStaged(stagingRoot, pkgName, a, baseDir, report)
}

// installStaged places staged content under baseDir, derived from the package
// layout (resolveInstall, unless the entry pins an explicit path). A single-folder
// install overwrites its destination and is pinned to a path/version. A package that
// unpacks to several plugin folders (a bundle, e.g. cogito shipping other addons) is
// handled carefully: only the primary folder (the one matching the entry) is
// overwritten and pinned; every other folder is written only when absent, so a
// bundled copy never clobbers a plugin the user manages separately. Shared by the
// generic and store install branches.
func installStaged(stagingRoot, pkgName string, a Addon, baseDir string, report Reporter) (InstallResult, error) {
	placements := resolveInstall(stagingRoot, a.Name, a.Path, pkgName)

	if len(placements) == 1 {
		if err := writePlacement(placements[0], baseDir, report); err != nil {
			return InstallResult{}, err
		}
		dest := filepath.Join(baseDir, placements[0].destRel)
		stampVersion(dest, intendedVersion(a), canonicalRepoURL(a.URL))
		return InstallResult{Path: placements[0].destRel, Version: getLocalPluginVersion(dest)}, nil
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
			res = InstallResult{Path: p.destRel, Version: getLocalPluginVersion(dest)}
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

// writePlacement replaces the folder at p.destRel (under baseDir) with p.src. It is the
// choke point every install path passes through, so it is where the project-root
// containment check lives — the destination is removed before it is written, and part
// of it can come from the downloaded package's own config (see resolveUnder).
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

// primaryPlacement returns the index of the placement that is the entry's own addon
// (so a bundle's other folders can be treated as extras), matched by the install
// folder's basename against the entry's name or its pinned path. Returns -1 when no
// placement matches — a genuinely ambiguous multi-addon package, where nothing is
// pinned and every folder is written only if absent.
func primaryPlacement(placements []placement, a Addon) int {
	want := map[string]bool{}
	if a.Name != "" {
		want[a.Name] = true
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
		if isGitCheckout(dest) {
			// Nothing to clone — but the checkout can still be in the wrong place: an
			// entry pinning no path only learns the addon's declared one from the
			// checkout itself, which an install made before that was consulted never
			// applied. Settling here relocates it once instead of stranding it forever.
			if a.Path == "" {
				destRel, dest = settleDeclaredPath(destRel, dest, baseDir, a.Name, false, report)
			}
			report("[%s] Already cloned at %s. Skipping (manage updates with git).", a.Name, destRel)
			return InstallResult{Path: destRel, Version: getLocalPluginVersion(dest)}, nil
		}
		// A non-git folder here (e.g. a prior package install being converted to a
		// clone): replace it so the clone can take its place. Package folders are
		// freely overwritten elsewhere and hold no uncommitted git work.
		report("[%s] Replacing non-git folder at %s with a fresh clone.", a.Name, destRel)
		if err := os.RemoveAll(dest); err != nil {
			return InstallResult{}, fmt.Errorf("could not remove existing folder %s: %w", destRel, err)
		}
	}

	if err := gitCloneBranch(ctx, a.URL, a.Tag, dest, a.Name, report); err != nil {
		return InstallResult{}, err
	}
	report("  -> Successfully cloned to %s", destRel)

	// A clone can only declare its own install path once it is on disk, so a derived
	// dest is provisional — settle it against the config the clone brought with it.
	// Only when the entry pins no path of its own, which keeps the precedence
	// installStaged applies to a package (manifest path > the addon's declared
	// dir=/path= key > addons/<name>); a clone just resolves the middle term after the
	// clone rather than before it.
	if a.Path == "" {
		destRel, dest = settleDeclaredPath(destRel, dest, baseDir, a.Name, true, report)
	}
	return InstallResult{Path: destRel, Version: getLocalPluginVersion(dest)}, nil
}

// settleDeclaredPath moves a cloned checkout from its derived location to the install
// path the clone declares in its own plugin.cfg/version.cfg (see installDir),
// returning where it ended up (project-relative and absolute). It mirrors
// cloneInstall's handling of an occupied destination: a checkout already at the
// declared path is a live working copy that is never overwritten, a non-git folder
// there is replaced. The checkout is already installed and usable at fromRel, so a
// move that cannot be made is reported and the derived location kept rather than
// failing an install that otherwise succeeded.
//
// fresh says whether the checkout at fromRel was just cloned by this install. Only
// then may it be discarded to resolve a clash with a checkout already at the declared
// path — one that was already there is the user's working copy, uncommitted work and
// all, and is left exactly where it is.
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

// Relocate moves an installed addon directory from fromRel to toRel (both
// project-root-relative), creating the destination's parent. It fails if the
// destination already exists — the caller decides what to do about a clash rather
// than silently overwriting. Used by the post-install "confirm location" form to
// honor a corrected path without re-downloading; a plain os.Rename moves a normal
// install or a clone (.git and all) alike.
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

// Uninstall deletes an addon's installed files under baseDir, symmetric with
// Install. The location is the entry's recorded path; it is lenient — an empty
// path (nothing recorded) or an already-absent directory is a no-op — so a
// "remove + delete files" action is safe even when the addon isn't installed. The
// manifest entry is removed separately via RemoveEntry.
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
