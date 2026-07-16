// Package addon holds the UI-agnostic logic for inspecting and installing
// Godot addons from a YAML manifest. It has no knowledge of cobra or the TUI;
// progress is surfaced through a Reporter so each front-end can render it
// however it likes.
package addon

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Reporter is a sink for human-readable progress lines. The CLI prints them to
// stdout; the TUI funnels them into bubbletea messages.
type Reporter func(format string, args ...any)

// Kind classifies how a manifest entry relates to git, on one mutually-exclusive
// axis (mirroring the internal gitKind probe).
type Kind string

const (
	KindPackage   Kind = ""          // default: an unzipped package, gdaddon-managed, no .git of its own
	KindClone     Kind = "clone"     // a live git working copy gdaddon manages (cloned with .git kept)
	KindSubmodule Kind = "submodule" // a live git working copy the parent repo manages; gdaddon never installs it
)

// KindOptions is the canonical label order for a kind toggle; index 0 is
// KindPackage ("package", the empty Kind rendered as a word). KindIndex/ParseKind
// convert between a Kind and its label.
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

// Addon is a single manifest entry. Name is the manifest key. Tag records the
// release tag the entry was installed from (empty for branch-HEAD installs, which
// have no tag); it's what dependency specs match against, since Version holds the
// author-controlled plugin.cfg version which can diverge from the tag.
type Addon struct {
	Name    string `yaml:"-"`
	URL     string `yaml:"url"`
	Path    string `yaml:"path"`
	Version string `yaml:"version"`
	Tag     string `yaml:"tag"`
	// Commit records the HEAD sha a branch package was pinned to (packages only; the
	// url is that commit's archive). It gives an otherwise-floating branch snapshot a
	// durable identity: a pinned entry reads as installed and never nags for updates.
	Commit string `yaml:"commit"`
	// Kind marks a live git checkout (clone or submodule) rather than an unzipped
	// package. For a clone, Install clones the repo with its .git kept and never
	// overwrites an existing checkout; for a submodule, gdaddon never installs at all
	// (the parent repo manages it) — both exist only for the utility actions. Tag
	// holds the checked-out branch.
	Kind Kind `yaml:"kind"`
	// Lock pins the entry: when true, gdaddon stops reporting available updates for it
	// and install/update reinstalls the pinned version rather than offering newer
	// releases. The user toggles it per-entry; it carries through set import/export.
	Lock bool `yaml:"lock"`
	// SuppressDeps lists (by source.RepoID) the dependencies this addon declares that
	// the user has chosen to ignore — an optional dep (e.g. a C++-rewritten perf
	// module) that should never contribute to the missing-deps warning nor be added by
	// "Add all missing". Stored as an inline flow list on the declaring addon's entry.
	SuppressDeps []string `yaml:"suppress_deps"`
	// Dependency records that this entry was auto-added because another plugin declares
	// it as a dependency — provenance, not the user's own choice. It lets OrphanDeps flag
	// the entry as an "unused dependency" once nothing installed still requires it. Set
	// when a dep is added via the Dependencies flow; cleared by the "Keep" action once the
	// user adopts it. Carries through set import/export but is dropped on export to global
	// (an explicit promotion). The user toggles it off, never on.
	Dependency bool `yaml:"is_dependency"`
}

// IsLocked reports whether the entry is pinned (no update alerts, install/update
// reinstalls the pinned version rather than offering newer releases).
func (a Addon) IsLocked() bool { return a.Lock }

// IsClone reports whether the entry is a gdaddon-managed live git working copy.
func (a Addon) IsClone() bool { return a.Kind == KindClone }

// IsSubmodule reports whether the entry is a parent-repo-managed submodule that
// gdaddon must never install or update.
func (a Addon) IsSubmodule() bool { return a.Kind == KindSubmodule }

// IsGitWorkdir reports whether the entry is a live git checkout (clone or
// submodule) — a present folder gdaddon never overwrites, carrying a branch and a
// dirty-state check rather than a pinned version.
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
	// LiveBranch is the branch currently checked out for a git workdir (clone/submodule),
	// read at inspect time; "" for non-git entries or an unreadable/detached checkout. When
	// it differs from the recorded Addon.Tag the State is StateBranchChanged.
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
		statuses = append(statuses, statusFor(a, baseDir))
	}
	return statuses, nil
}

func statusFor(a Addon, baseDir string) Status {
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

	// A live git checkout (clone or submodule) is never overwritten once present, so it
	// carries no version match. Instead compare its live branch against the branch the
	// manifest records (Addon.Tag): a known, differing branch is drift worth surfacing
	// (StateBranchChanged); otherwise it stays unversioned (which InstallAll skips). A
	// detached HEAD or unreadable repo yields "" and reads as unversioned, no false drift.
	if a.IsGitWorkdir() {
		s.LiveBranch = gitCheckedOutBranch(fullPath)
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

// getLocalPluginVersion reports the version recorded in an installed addon's
// plugin.cfg/version.cfg under addonPath, or "" if absent. Used after an
// install/update to pin the real installed version.
func getLocalPluginVersion(addonPath string) string {
	return readPluginCfgKey(addonPath, "version")
}

// ProjectName reads config/name from a Godot project.godot at root. exists
// reports whether project.godot is present; name may be "" if present but
// unnamed. A plain line scan is used since project.godot's full syntax (arrays,
// resource refs) trips strict INI parsers.
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
