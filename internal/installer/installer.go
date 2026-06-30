// Package installer implements `gdaddon install`: copying the running binary to a
// chosen destination (system / user / gdaddon-home) and wiring up PATH. The logic
// is split so the cross-platform parts (destination enum, self-locate, byte copy)
// live here and the per-OS parts (concrete dirs, PATH handling, elevation) live in
// path_unix.go / path_windows.go. There is no TUI here — cmd/install.go selects a
// Dest (via a bubbletea menu) and then calls Install, which keeps the privileged /
// PATH side effects out of the terminal-owning UI.
package installer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Dest is a selectable install location. The concrete directory each maps to is
// platform-specific (see destDir in path_unix.go / path_windows.go).
type Dest int

const (
	System Dest = iota // on PATH by default, needs elevation
	User               // no elevation, may need PATH setup
	Home               // ~/.gdaddon/bin, not on PATH (the Godot-plugin target)
)

func (d Dest) String() string {
	switch d {
	case System:
		return "system"
	case User:
		return "user"
	case Home:
		return "home"
	}
	return "unknown"
}

// ParseDest maps the --dest flag value to a Dest.
func ParseDest(s string) (Dest, error) {
	switch s {
	case "system":
		return System, nil
	case "user":
		return User, nil
	case "home":
		return Home, nil
	}
	return 0, fmt.Errorf("invalid destination %q (want system|user|home)", s)
}

// Option is a menu row: a Dest plus its display label and one-line description.
type Option struct {
	Dest  Dest
	Label string
	Desc  string
}

// Result reports where the binary landed and any follow-up the user should see
// (e.g. PATH guidance), printed by the caller after the TUI has exited.
type Result struct {
	Path string
	Note string
}

// Dests lists the selectable destinations in menu order, with platform-specific
// labels/descriptions.
func Dests() []Option {
	out := make([]Option, 0, 3)
	for _, d := range []Dest{System, User, Home} {
		out = append(out, Option{Dest: d, Label: optionLabel(d), Desc: optionDesc(d)})
	}
	return out
}

// Self resolves the path of the running binary, following a symlink so the copy
// reads the real file (e.g. when launched through ~/.local/bin/gdaddon).
func Self() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if rp, err := filepath.EvalSymlinks(p); err == nil {
		p = rp
	}
	return p, nil
}

// Install copies the running binary to dest and applies any PATH wiring.
func Install(dest Dest) (Result, error) {
	src, err := Self()
	if err != nil {
		return Result{}, err
	}
	return InstallFrom(dest, src)
}

// InstallFrom copies an explicit source binary to dest and applies any PATH wiring.
// Install is InstallFrom with src = the running binary; self-update passes a freshly
// downloaded binary instead.
func InstallFrom(dest Dest, src string) (Result, error) {
	return doInstall(dest, src)
}

// ExeName is the gdaddon binary's filename on this platform ("gdaddon", or
// "gdaddon.exe" on Windows). Exported so callers (e.g. self-update locating the
// binary inside a downloaded release zip) don't duplicate the per-OS name.
func ExeName() string { return exeName() }

// CurrentDest reports which managed destination (System/User/Home) the running
// binary occupies, comparing by inode so a symlinked launcher still resolves to its
// target. ok is false when the running binary is outside all managed locations (e.g.
// a dev build under build/).
func CurrentDest() (Dest, bool) {
	self, err := Self()
	if err != nil {
		return 0, false
	}
	selfInfo, err := os.Stat(self)
	if err != nil {
		return 0, false
	}
	for _, d := range []Dest{System, User, Home} {
		p, err := binPath(d)
		if err != nil {
			continue
		}
		if fi, err := os.Stat(p); err == nil && os.SameFile(fi, selfInfo) {
			return d, true
		}
	}
	return 0, false
}

// binPath is the full path the gdaddon binary would occupy for dest.
func binPath(d Dest) (string, error) {
	dir, err := destDir(d)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, exeName()), nil
}

// Uninstall removes the gdaddon binary from every known destination where it is
// present (binaries only — it leaves PATH entries and other ~/.gdaddon files
// untouched). The currently-running binary is left in place and reported in
// skipped. System removal may prompt for sudo on unix.
func Uninstall() (removed, skipped []string, err error) {
	self, err := Self()
	if err != nil {
		return nil, nil, err
	}
	var paths []string
	for _, d := range []Dest{System, User, Home} {
		p, err := binPath(d)
		if err != nil {
			continue // unresolvable dest (e.g. missing home) — nothing to remove there
		}
		paths = append(paths, p)
	}
	return uninstallFrom(paths, self)
}

// uninstallFrom removes each existing path that isn't the running binary. Split
// out so it can be tested with temp paths (never the real /usr/local/bin).
func uninstallFrom(paths []string, self string) (removed, skipped []string, err error) {
	selfInfo, _ := os.Stat(self)
	for _, p := range paths {
		fi, statErr := os.Stat(p)
		if statErr != nil {
			continue // not installed here
		}
		if selfInfo != nil && os.SameFile(fi, selfInfo) {
			skipped = append(skipped, p)
			continue
		}
		if err := removeAt(p); err != nil {
			return removed, skipped, fmt.Errorf("remove %s: %w", p, err)
		}
		removed = append(removed, p)
	}
	return removed, skipped, nil
}

// copyExe streams src to dstPath as a 0755 executable, creating the parent dir. A
// no-op when src == dstPath (re-installing from the same location).
func copyExe(src, dstPath string) error {
	if src == dstPath {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dstPath, 0o755)
}
