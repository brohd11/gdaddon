package cmd

import (
	"github.com/brohd11/goutil/selfupdate"
)

// `gdaddon update` updates gdaddon itself; `update-addons` updates a project's addons.
func init() {
	rootCmd.AddCommand(selfupdate.NewUpdateCommand("brohd11/gdaddon", "gdaddon", version))
}
