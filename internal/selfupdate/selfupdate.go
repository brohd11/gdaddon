// Package selfupdate wires gdaddon's self-update command and TUI flow to the
// shared mechanism in github.com/brohd11/goutil/selfupdate: check GitHub for a
// newer release and, when one exists, install it by running the repo's own
// install.sh — the same script the README tells users to curl. What stays here
// is the gdaddon-specific glue: the repo slug, the installer.Dest mapping, and
// the default destination.
package selfupdate

import (
	"context"
	"path/filepath"

	"gdaddon/internal/installer"

	goutil "github.com/brohd11/goutil/selfupdate"
)

// Repo is the GitHub repository the releases and install.sh are fetched from.
const Repo = "brohd11/gdaddon"

// Info is the outcome of Check: the running version, the latest release tag,
// and whether the release is newer.
type Info = goutil.Info

// Check reports whether the latest gdaddon release is newer than current. A
// "dev" build is never comparable, hence never offered an update.
func Check(ctx context.Context, current string) (Info, error) {
	return goutil.Check(ctx, Repo, current)
}

// Apply installs info.LatestTag into dest's directory and returns the
// installed binary's path. Overwriting the running binary is safe: install.sh
// stages in a temp dir and mv -f's into place.
func Apply(ctx context.Context, info Info, dest installer.Dest, report func(string, ...any)) (string, error) {
	dir, err := dest.Dir()
	if err != nil {
		return "", err
	}
	if err := goutil.Apply(ctx, Repo, info, dir, report); err != nil {
		return "", err
	}
	return filepath.Join(dir, installer.ExeName()), nil
}

// DefaultDest is the install target self-update uses without an explicit choice: the
// managed location the running binary already occupies, falling back to the
// gdaddon-home location the Godot plugin launches when the running binary isn't a
// managed install.
func DefaultDest() installer.Dest {
	if d, ok := installer.CurrentDest(); ok {
		return d
	}
	return installer.Home
}
