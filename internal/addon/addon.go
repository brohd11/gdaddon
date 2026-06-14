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
	if a.URL == "" || a.Path == "" {
		return Status{Addon: a, State: StateInvalid}
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
// unversioned-present entries are skipped, mismatches are updated. This mirrors
// the original non-interactive `addon_install` behavior.
func InstallAll(statuses []Status, baseDir string, report Reporter) error {
	for _, s := range statuses {
		a := s.Addon
		switch s.State {
		case StateInvalid:
			report("Skipping %s: missing 'url' or 'path'", a.Name)
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

		if err := Install(a, baseDir, report); err != nil {
			report("[%s] Error: %v", a.Name, err)
		}
	}
	return nil
}

// Install fetches a single addon and installs it to baseDir/a.Path, replacing
// any existing install at that path. It dispatches on the URL suffix.
func Install(a Addon, baseDir string, report Reporter) error {
	if a.URL == "" || a.Path == "" {
		return fmt.Errorf("missing 'url' or 'path'")
	}

	fullPath, err := filepath.Abs(filepath.Join(baseDir, a.Path))
	if err != nil {
		return fmt.Errorf("could not resolve path: %w", err)
	}

	if _, err := os.Stat(fullPath); err == nil {
		os.RemoveAll(fullPath)
	}

	switch {
	case strings.HasSuffix(a.URL, ".zip"):
		return downloadAndExtractZip(a.URL, fullPath, a.Name, report)
	case strings.HasSuffix(a.URL, ".git"):
		return cloneGitRepo(a.URL, fullPath, a.Name, a.Path, report)
	default:
		return fmt.Errorf("URL must end in '.zip' or '.git'. Found: %s", a.URL)
	}
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
