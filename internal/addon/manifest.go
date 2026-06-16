package addon

import (
	"fmt"
	"os"
	"strings"
)

// LocalVersion reports the version recorded in an installed addon's plugin.cfg,
// or "" if absent. Used after an update to pin the real installed version.
func LocalVersion(fullPath string) string {
	return getLocalPluginVersion(fullPath)
}

// UpdateEntry rewrites a single manifest entry's url, path, and version in place.
// It edits only those lines (inserting them if absent), leaving every other line
// — blank lines, comments, indentation, quoting — byte-for-byte intact. An empty
// url or path leaves that existing line untouched (e.g. after install/update we
// pin the resolved path + version but keep the user's original source url).
// It assumes the flat manifest shape: top-level entry keys at column 0 with
// indented url/path/version fields beneath them.
func UpdateEntry(manifestPath, name, url, path, version string) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")

	keyIdx := -1
	for i, ln := range lines {
		if isEntryKey(ln, name) {
			keyIdx = i
			break
		}
	}
	if keyIdx == -1 {
		return fmt.Errorf("addon %q not found in %s", name, manifestPath)
	}

	// The entry block runs until the next column-0 (non-indented) content line.
	end := len(lines)
	for i := keyIdx + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		if !startsWithSpace(lines[i]) {
			end = i
			break
		}
	}

	indent := "    "
	urlDone, pathDone, versionDone := false, false, false
	for i := keyIdx + 1; i < end; i++ {
		ind, key, ok := splitField(lines[i])
		if !ok {
			continue
		}
		indent = ind
		switch key {
		case "url":
			if url != "" {
				lines[i] = ind + "url: " + url
			}
			urlDone = true
		case "path":
			if path != "" {
				lines[i] = ind + "path: " + path
			}
			pathDone = true
		case "version":
			lines[i] = ind + `version: "` + version + `"`
			versionDone = true
		}
	}

	var inserts []string
	if !urlDone && url != "" {
		inserts = append(inserts, indent+"url: "+url)
	}
	if !pathDone && path != "" {
		inserts = append(inserts, indent+"path: "+path)
	}
	if !versionDone {
		inserts = append(inserts, indent+`version: "`+version+`"`)
	}
	if len(inserts) > 0 {
		tail := append(inserts, lines[keyIdx+1:]...)
		lines = append(lines[:keyIdx+1], tail...)
	}

	return os.WriteFile(manifestPath, []byte(strings.Join(lines, "\n")), 0o644)
}

// RemoveEntry deletes a manifest entry — its key line and the indented block
// beneath it — in place, leaving every other entry byte-for-byte intact. It uses
// the same flat-shape block detection as UpdateEntry, so it works on the project
// manifest and the global list alike. Returns an error if the entry isn't found.
func RemoveEntry(manifestPath, name string) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")

	keyIdx := -1
	for i, ln := range lines {
		if isEntryKey(ln, name) {
			keyIdx = i
			break
		}
	}
	if keyIdx == -1 {
		return fmt.Errorf("addon %q not found in %s", name, manifestPath)
	}

	// The entry block runs until the next column-0 (non-indented) content line.
	end := len(lines)
	for i := keyIdx + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		if !startsWithSpace(lines[i]) {
			end = i
			break
		}
	}

	lines = append(lines[:keyIdx], lines[end:]...)
	return os.WriteFile(manifestPath, []byte(strings.Join(lines, "\n")), 0o644)
}

// isEntryKey reports whether ln is the column-0 mapping key `<name>:`.
func isEntryKey(ln, name string) bool {
	if startsWithSpace(ln) {
		return false
	}
	rest, ok := strings.CutPrefix(ln, name+":")
	if !ok {
		return false
	}
	// Guard against name being a prefix of another key, e.g. `Foo:` vs `FooBar:`.
	return rest == "" || strings.TrimSpace(rest) == "" || strings.HasPrefix(rest, " ") || strings.HasPrefix(rest, "\t")
}

func startsWithSpace(ln string) bool {
	return strings.HasPrefix(ln, " ") || strings.HasPrefix(ln, "\t")
}

// splitField parses an indented `key: ...` line, returning its indent and key.
func splitField(ln string) (indent, key string, ok bool) {
	trimmed := strings.TrimLeft(ln, " \t")
	indent = ln[:len(ln)-len(trimmed)]
	if indent == "" {
		return "", "", false
	}
	colon := strings.IndexByte(trimmed, ':')
	if colon < 1 {
		return "", "", false
	}
	return indent, trimmed[:colon], true
}
