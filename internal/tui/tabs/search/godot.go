package search

import (
	"os"
	"path/filepath"
	"regexp"
)

// defaultGodotVersion is the version filter when project.godot has none.
const defaultGodotVersion = "4.4"

// featuresVersion matches the leading "major.minor" token in a Godot 4
// project.godot config/features line, e.g. config/features=PackedStringArray("4.3", …).
var featuresVersion = regexp.MustCompile(`config/features=PackedStringArray\("(\d+\.\d+)`)

// detectGodotVersion reads <root>/project.godot and returns its "major.minor"
// engine version, falling back to defaultGodotVersion when it can't be found.
func detectGodotVersion(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "project.godot"))
	if err != nil {
		return defaultGodotVersion
	}
	if m := featuresVersion.FindSubmatch(data); m != nil {
		return string(m[1])
	}
	return defaultGodotVersion
}
