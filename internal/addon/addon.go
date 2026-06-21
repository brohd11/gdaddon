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
	// Clone marks a branch-HEAD entry installed as a live git working copy (a
	// "sub-repo" for development) rather than an unzipped package: Install clones
	// the repo with its .git kept and never overwrites an existing checkout. Tag
	// holds the branch that was cloned.
	Clone bool `yaml:"clone"`
}

// State describes an addon's local install relative to the manifest.
type State int

const (
	StateInvalid     State = iota // missing url or path
	StateMissing                  // not installed locally
	StateInstalled                // installed and version matches (or no version pinned + present unversioned)
	StateMismatch                 // installed but local version != pinned version
	StateUnversioned              // installed, present, manifest pins no version
)

// Status pairs an addon with its computed local state.
type Status struct {
	Addon        Addon
	State        State
	LocalVersion string
	FullPath     string
}

// Installable reports whether installing this addon makes sense for an explicit
// user action (the TUI). Invalid entries cannot be installed.
func (s Status) Installable() bool { return s.State != StateInvalid }

// Present reports whether the addon is installed on disk (any present state),
// regardless of version match.
func (s Status) Present() bool {
	return s.State == StateInstalled || s.State == StateMismatch || s.State == StateUnversioned
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

	// A clone entry is a live git working copy managed by the user; once present
	// it's never overwritten, so report it as unversioned (which InstallAll skips)
	// regardless of any version match.
	if a.Clone {
		s.State = StateUnversioned
		return s
	}

	switch {
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
