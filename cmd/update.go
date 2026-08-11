package cmd

import (
	"github.com/brohd11/goutil/selfupdate"
)

// `gdaddon update` is the shared self-update command every brohd11 app registers: it
// compares this binary's version against the latest release and installs it in place
// (over wherever the binary lives), or reports with --check. Nothing gdaddon-specific
// is involved, so there is no local wrapper — the mechanism, the flags and the output
// are the same here as in gote, gossh, golaunch and repoview.
//
// This updates gdaddon itself. `gdaddon update-addons` is the one that updates the
// Godot addons installed in a project.
func init() {
	rootCmd.AddCommand(selfupdate.NewUpdateCommand("brohd11/gdaddon", "gdaddon", version))
}
