// Package addon inspects and installs Godot addons from a YAML manifest, independent of
// the CLI and TUI; progress goes through a Reporter.
package addon

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kind classifies how a manifest entry relates to git, on one mutually-exclusive
// axis (mirroring the internal gitKind probe).
type Kind string

const (
	KindPackage   Kind = ""          // default: an unzipped package, gdaddon-managed, no .git of its own
	KindClone     Kind = "clone"     // a live git working copy gdaddon manages (cloned with .git kept)
	KindSubmodule Kind = "submodule" // a live git working copy the parent repo manages; gdaddon never installs it
)

// KindOptions is the label order for a kind toggle; index 0 is KindPackage.
// KindIndex/ParseKind convert.
var KindOptions = []string{"package", "clone", "submodule"}

// KindIndex returns k's position in KindOptions (KindClone→1, KindSubmodule→2,
// KindPackage/other→0).
func KindIndex(k Kind) int {
	switch k {
	case KindClone:
		return 1
	case KindSubmodule:
		return 2
	default:
		return 0
	}
}

// ParseKind maps a KindOptions label back to its Kind ("clone"/"submodule" to that
// Kind, anything else — including "package" — to KindPackage).
func ParseKind(label string) Kind {
	switch label {
	case "clone":
		return KindClone
	case "submodule":
		return KindSubmodule
	default:
		return KindPackage
	}
}

// Addon is one manifest entry. Name is its key and identity (a canonical repo id for new
// entries, see EntryKey); Label, not Name, is what gets displayed. Tag is the release tag it
// was installed from (empty for branch installs) and is what dependency specs match,
// since Version is the author's plugin.cfg version.
type Addon struct {
	Name string `yaml:"-"`
	// Display is the addon's declared name (from plugin.cfg on install, or Edit Manifest). It
	// is only a label, never a lookup key, so it may contain anything. Empty falls back to
	// Slug.
	Display string `yaml:"name"`
	URL     string `yaml:"url"`
	Path    string `yaml:"path"`
	Version string `yaml:"version"`
	Tag     string `yaml:"tag"`
	// Commit is the HEAD sha a branch package was pinned to (the url is that commit's
	// archive); a pinned entry reads as installed and never offers updates.
	Commit string `yaml:"commit"`
	// Kind marks a live git checkout. A clone is cloned with .git kept and never overwritten;
	// a submodule is never installed (its parent repo manages it). Tag holds the branch.
	Kind Kind `yaml:"kind"`
	// Lock pins the entry: no update reports, and install reinstalls the pinned version.
	Lock bool `yaml:"lock"`
	// SuppressDeps lists (by source.RepoID) declared dependencies the user chose to ignore, so
	// they never count as missing or get added.
	SuppressDeps []string `yaml:"suppress_deps"`
	// Dependency marks an entry auto-added as another plugin's dependency, so OrphanDeps can
	// flag it once nothing requires it. "Keep" clears it; export to global drops it.
	Dependency bool `yaml:"is_dependency"`
}

// Label is the entry's display name: its declared name, else its Slug.
func (a Addon) Label() string {
	if a.Display != "" {
		return a.Display
	}
	return a.Slug()
}

// Slug is the last path segment of the key, safe for paths and folder matches (a legacy
// key is its own slug).
func (a Addon) Slug() string {
	return path.Base(a.Name)
}

// IsLocked reports whether the entry is pinned (no update alerts, install/update
// reinstalls the pinned version rather than offering newer releases).
func (a Addon) IsLocked() bool { return a.Lock }

// IsClone reports whether the entry is a gdaddon-managed live git working copy.
func (a Addon) IsClone() bool { return a.Kind == KindClone }

// IsSubmodule reports whether the entry is a parent-repo-managed submodule that
// gdaddon must never install or update.
func (a Addon) IsSubmodule() bool { return a.Kind == KindSubmodule }

// IsGitWorkdir reports a live git checkout (clone or submodule), which is never
// overwritten and has a branch rather than a version.
func (a Addon) IsGitWorkdir() bool { return a.Kind == KindClone || a.Kind == KindSubmodule }

// State describes an addon's local install relative to the manifest.
type State int

const (
	StateInvalid       State = iota // missing url or path
	StateMissing                    // not installed locally
	StateInstalled                  // installed and version matches (or no version pinned + present unversioned)
	StateMismatch                   // installed but local version != pinned version
	StateUnversioned                // installed, present, manifest pins no version
	StateBranchChanged              // git checkout present, on a different branch than the manifest records
)

// String renders a State as a short lowercase label for non-interactive output.
func (s State) String() string {
	switch s {
	case StateMissing:
		return "missing"
	case StateInstalled:
		return "installed"
	case StateMismatch:
		return "mismatch"
	case StateUnversioned:
		return "unversioned"
	case StateBranchChanged:
		return "branch_changed"
	default:
		return "invalid"
	}
}

// Status pairs an addon with its computed local state.
type Status struct {
	Addon        Addon
	State        State
	LocalVersion string
	FullPath     string
	// LiveBranch is a git workdir's checked-out branch at inspect time ("" otherwise); differing
	// from Addon.Tag makes the state StateBranchChanged.
	LiveBranch string
}

// Installable reports whether installing this addon makes sense for an explicit
// user action (the TUI). Invalid entries cannot be installed.
func (s Status) Installable() bool { return s.State != StateInvalid }

// Present reports whether the addon is installed on disk (any present state),
// regardless of version match.
func (s Status) Present() bool {
	return s.State == StateInstalled || s.State == StateMismatch || s.State == StateUnversioned || s.State == StateBranchChanged
}

// Parse reads and unmarshals the manifest into a name-sorted slice.
func Parse(manifestPath string) ([]Addon, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", manifestPath, err)
	}

	var raw map[string]Addon
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("could not parse YAML: %w", err)
	}

	names := make([]string, 0, len(raw))
	for name := range raw {
		names = append(names, name)
	}
	sort.Strings(names)

	addons := make([]Addon, 0, len(raw))
	for _, name := range names {
		a := raw[name]
		a.Name = name
		addons = append(addons, a)
	}
	return addons, nil
}

// Inspect parses the manifest and computes each addon's local state against
// baseDir. It performs no installs and does not modify the filesystem.
func Inspect(manifestPath, baseDir string) ([]Status, error) {
	addons, err := Parse(manifestPath)
	if err != nil {
		return nil, err
	}

	statuses := make([]Status, 0, len(addons))
	for _, a := range addons {
		statuses = append(statuses, InspectOne(a, baseDir))
	}
	return statuses, nil
}

// InspectOne computes one cached manifest entry's current local state without
// parsing the manifest or inspecting any other addon.
func InspectOne(a Addon, baseDir string) Status {
	if a.URL == "" {
		return Status{Addon: a, State: StateInvalid}
	}
	// A url-only entry's install location is unknown until it's installed (the
	// path is derived from the package contents then), so treat it as missing.
	if a.Path == "" {
		return Status{Addon: a, State: StateMissing}
	}

	fullPath, err := filepath.Abs(filepath.Join(baseDir, a.Path))
	if err != nil {
		return Status{Addon: a, State: StateInvalid}
	}

	s := Status{Addon: a, FullPath: fullPath}

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		s.State = StateMissing
		return s
	}

	local := getLocalPluginVersion(fullPath)
	s.LocalVersion = local

	// A present git checkout is never overwritten, so compare branches instead of versions: a
	// known, different branch is StateBranchChanged; otherwise unversioned. A detached or
	// unreadable repo reads as unversioned.
	if a.IsGitWorkdir() {
		s.LiveBranch = CurrentBranch(fullPath)
		if s.LiveBranch != "" && s.LiveBranch != a.Tag {
			s.State = StateBranchChanged
		} else {
			s.State = StateUnversioned
		}
		return s
	}

	switch {
	case a.Commit != "":
		// A commit-pinned package: its exact snapshot can't be re-verified from a
		// .git-less folder, so trust the recorded pin — present means installed.
		s.State = StateInstalled
	case a.Version == "":
		s.State = StateUnversioned
	case local == a.Version:
		s.State = StateInstalled
	default:
		s.State = StateMismatch
	}
	return s
}

// getLocalPluginVersion returns the version in addonPath's plugin.cfg/version.cfg, or "".
func getLocalPluginVersion(addonPath string) string {
	return readPluginCfgKey(addonPath, "version")
}

// getLocalPluginName returns the name addonPath's plugin.cfg/version.cfg declares, or "".
// ScanInstalled and AdoptName both use it.
func getLocalPluginName(addonPath string) string {
	return readPluginCfgKey(addonPath, "name")
}

// ProjectName reads config/name from project.godot at root; exists reports whether the
// file is there. A line scan, since project.godot trips strict INI parsers.
func ProjectName(root string) (name string, exists bool) {
	data, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "config/name="); ok {
			return strings.Trim(strings.TrimSpace(rest), `"'`), true
		}
	}
	return "", true
}
