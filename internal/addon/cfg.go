package addon

import (
	"os"
	"path/filepath"
)

// pluginCfgNames are the config filenames that mark an installable addon/library
// and carry its version: plugin.cfg (Godot editor plugins) and version.cfg
// (libraries that are versioned but intentionally don't appear in the plugin
// menu). Both use the same INI shape — a [plugin] section with version="…".
// Centralized here so the recognized set can be changed in one place.
var pluginCfgNames = []string{"plugin.cfg", "version.cfg"}

// pluginCfgPath returns the path of the recognized config file in dir (checked in
// pluginCfgNames order), or "" if none is present.
func pluginCfgPath(dir string) string {
	for _, name := range pluginCfgNames {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// hasPluginCfg reports whether dir is an addon/library folder.
func hasPluginCfg(dir string) bool { return pluginCfgPath(dir) != "" }
