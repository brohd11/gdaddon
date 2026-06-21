package addon

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/ini.v1"
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

// readPluginCfgKey reads one key from dir's plugin.cfg/version.cfg [plugin] section,
// returning the unquoted, space-trimmed value or "" when the file/section/key is
// absent or unreadable. The silent "" fallback is the behavior every caller relies on
// (an unversioned or dir-less addon is normal, not an error). Callers that need to
// distinguish a read error (e.g. Dependencies) load the config themselves.
func readPluginCfgKey(dir, key string) string {
	cfgPath := pluginCfgPath(dir)
	if cfgPath == "" {
		return ""
	}
	cfg, err := ini.Load(cfgPath)
	if err != nil {
		return ""
	}
	raw := cfg.Section("plugin").Key(key).String()
	return strings.Trim(strings.TrimSpace(raw), `'"`)
}
