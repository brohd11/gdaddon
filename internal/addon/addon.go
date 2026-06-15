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

	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

// Reporter is a sink for human-readable progress lines. The CLI prints them to
// stdout; the TUI funnels them into bubbletea messages.
type Reporter func(format string, args ...any)

// Addon is a single manifest entry. Name is the manifest key.
type Addon struct {
	Name    string `yaml:"-"`
	URL     string `yaml:"url"`
	Path    string `yaml:"path"`
	Version string `yaml:"version"`
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

// InstallAll applies the manifest's skip/update policy: already-installed and
// unversioned-present entries are skipped, mismatches are updated. After a
// successful install it pins the resolved path + version back into the manifest
// (entries start url-only; this records where they landed).
func InstallAll(manifestPath string, statuses []Status, baseDir string, report Reporter) error {
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

		res, err := Install(a, baseDir, report)
		if err != nil {
			report("[%s] Error: %v", a.Name, err)
			continue
		}
		if res.Path != "" {
			_ = UpdateEntry(manifestPath, a.Name, "", res.Path, res.Version)
		}
	}
	return nil
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
func Install(a Addon, baseDir string, report Reporter) (InstallResult, error) {
	if a.URL == "" {
		return InstallResult{}, fmt.Errorf("missing 'url'")
	}

	stagingRoot, cleanup, err := fetchToStaging(a.URL, a.Name, report)
	if err != nil {
		return InstallResult{}, err
	}
	defer cleanup()

	placements := resolveInstall(stagingRoot, a.Name, a.Path)
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

func getLocalPluginVersion(addonPath string) string {
	cfgPath := filepath.Join(addonPath, "plugin.cfg")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return ""
	}

	cfg, err := ini.Load(cfgPath)
	if err != nil {
		return ""
	}

	section := cfg.Section("plugin")
	if section == nil {
		return ""
	}

	key := section.Key("version")
	if key == nil {
		return ""
	}

	return strings.Trim(key.String(), `'"`)
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
