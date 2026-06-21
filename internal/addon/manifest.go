package addon

import (
	"fmt"
	"os"
	"strings"
)

// UpdateEntry rewrites a single manifest entry's url, path, version, and tag in
// place. It edits only those lines (inserting them if absent), leaving every other
// line — blank lines, comments, indentation, quoting — byte-for-byte intact. An
// empty value for any field leaves its existing line untouched (e.g. after
// install/update we pin the resolved path + version + tag but keep the user's
// original source url; adding a dependency pins url + tag with no version yet).
// It assumes the flat manifest shape: top-level entry keys at column 0 with
// indented url/path/version/tag fields beneath them.
func UpdateEntry(manifestPath, name, url, path, version, tag string) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")

	keyIdx, end, ok := findEntryBlock(lines, name)
	if !ok {
		return fmt.Errorf("addon %q not found in %s", name, manifestPath)
	}

	indent := "    "
	urlDone, pathDone, versionDone, tagDone := false, false, false, false
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
			if version != "" {
				lines[i] = ind + `version: "` + version + `"`
			}
			versionDone = true
		case "tag":
			if tag != "" {
				lines[i] = ind + `tag: "` + tag + `"`
			}
			tagDone = true
		}
	}

	var inserts []string
	if !urlDone && url != "" {
		inserts = append(inserts, indent+"url: "+url)
	}
	if !pathDone && path != "" {
		inserts = append(inserts, indent+"path: "+path)
	}
	if !versionDone && version != "" {
		inserts = append(inserts, indent+`version: "`+version+`"`)
	}
	if !tagDone && tag != "" {
		inserts = append(inserts, indent+`tag: "`+tag+`"`)
	}
	if len(inserts) > 0 {
		tail := append(inserts, lines[keyIdx+1:]...)
		lines = append(lines[:keyIdx+1], tail...)
	}

	return os.WriteFile(manifestPath, []byte(strings.Join(lines, "\n")), 0o644)
}

// EditEntry rewrites a single manifest entry's url, path, version, and tag in place
// with set-or-clear semantics: a non-empty value sets/inserts the line (like
// UpdateEntry — url/path unquoted, version/tag quoted), while an empty value REMOVES
// that field's line if present. This is the opposite of UpdateEntry's "empty leaves
// the line untouched" rule, and is what the Edit Manifest form needs (a blanked
// field means the user wants the field gone). Every other line — blank lines,
// comments, the clone line, indentation — is left byte-for-byte intact. clone is a
// bool and stays out of here; use SetCloneFlag for it.
func EditEntry(manifestPath, name, url, path, version, tag string) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")

	keyIdx, end, ok := findEntryBlock(lines, name)
	if !ok {
		return fmt.Errorf("addon %q not found in %s", name, manifestPath)
	}

	// Desired rendered line for a field (empty value ⇒ no line).
	render := func(ind, key, val string) string {
		if val == "" {
			return ""
		}
		switch key {
		case "version", "tag":
			return ind + key + `: "` + val + `"`
		default:
			return ind + key + ": " + val
		}
	}

	indent := "    "
	urlDone, pathDone, versionDone, tagDone := false, false, false, false
	var drop []int // indices of field lines to remove (cleared values), descending-safe via later sort
	for i := keyIdx + 1; i < end; i++ {
		ind, key, ok := splitField(lines[i])
		if !ok {
			continue
		}
		indent = ind
		val := ""
		seen := true
		switch key {
		case "url":
			val, urlDone = url, true
		case "path":
			val, pathDone = path, true
		case "version":
			val, versionDone = version, true
		case "tag":
			val, tagDone = tag, true
		default:
			seen = false
		}
		if !seen {
			continue
		}
		if val == "" {
			drop = append(drop, i)
		} else {
			lines[i] = render(ind, key, val)
		}
	}

	// Remove cleared field lines (descending so earlier indices stay valid).
	for j := len(drop) - 1; j >= 0; j-- {
		i := drop[j]
		lines = append(lines[:i], lines[i+1:]...)
	}

	// Insert any non-empty fields that weren't already present, after the key line.
	var inserts []string
	if !urlDone {
		if s := render(indent, "url", url); s != "" {
			inserts = append(inserts, s)
		}
	}
	if !pathDone {
		if s := render(indent, "path", path); s != "" {
			inserts = append(inserts, s)
		}
	}
	if !versionDone {
		if s := render(indent, "version", version); s != "" {
			inserts = append(inserts, s)
		}
	}
	if !tagDone {
		if s := render(indent, "tag", tag); s != "" {
			inserts = append(inserts, s)
		}
	}
	if len(inserts) > 0 {
		tail := append(inserts, lines[keyIdx+1:]...)
		lines = append(lines[:keyIdx+1], tail...)
	}

	return os.WriteFile(manifestPath, []byte(strings.Join(lines, "\n")), 0o644)
}

// AddEntryWithVersion appends an entry like AddEntry (deduped by repo identity,
// creating the file if absent), then pins version and/or tag lines onto it when
// non-empty. It composes the two existing writers so a versioned/tagged add (a set
// "Add Version", importing a set entry, or adding a tagged dependency) doesn't need
// a second manifest shape. Empty version and tag behave exactly like AddEntry.
func AddEntryWithVersion(manifestPath, name, url, path, version, tag string) error {
	if err := AddEntry(manifestPath, name, url, path); err != nil {
		return err
	}
	if version == "" && tag == "" {
		return nil
	}
	return UpdateEntry(manifestPath, name, "", "", version, tag)
}

// SetCloneFlag sets (or clears) the boolean `clone:` line on an entry, in place,
// using the same flat-shape block scan as UpdateEntry. When clone is true it
// inserts/updates `clone: true`; when false it removes any existing clone line.
// Kept separate from UpdateEntry so its string-field "empty means leave untouched"
// convention isn't muddied by a bool.
func SetCloneFlag(manifestPath, name string, clone bool) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")

	keyIdx, end, ok := findEntryBlock(lines, name)
	if !ok {
		return fmt.Errorf("addon %q not found in %s", name, manifestPath)
	}

	indent := "    "
	cloneIdx := -1
	for i := keyIdx + 1; i < end; i++ {
		ind, key, ok := splitField(lines[i])
		if !ok {
			continue
		}
		indent = ind
		if key == "clone" {
			cloneIdx = i
		}
	}

	switch {
	case !clone:
		if cloneIdx != -1 {
			lines = append(lines[:cloneIdx], lines[cloneIdx+1:]...)
		}
	case cloneIdx != -1:
		lines[cloneIdx] = indent + "clone: true"
	default:
		tail := append([]string{indent + "clone: true"}, lines[keyIdx+1:]...)
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

	keyIdx, end, ok := findEntryBlock(lines, name)
	if !ok {
		return fmt.Errorf("addon %q not found in %s", name, manifestPath)
	}

	lines = append(lines[:keyIdx], lines[end:]...)
	return os.WriteFile(manifestPath, []byte(strings.Join(lines, "\n")), 0o644)
}

// findEntryBlock locates the entry named name in the flat manifest shape: it returns
// the index of the column-0 key line and the exclusive end of the indented block
// beneath it (the next column-0 content line, or len(lines)). ok is false when name
// isn't present as a column-0 entry key. Shared by every in-place entry writer.
func findEntryBlock(lines []string, name string) (keyIdx, end int, ok bool) {
	keyIdx = -1
	for i, ln := range lines {
		if isEntryKey(ln, name) {
			keyIdx = i
			break
		}
	}
	if keyIdx == -1 {
		return 0, 0, false
	}

	end = len(lines)
	for i := keyIdx + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		if !startsWithSpace(lines[i]) {
			end = i
			break
		}
	}
	return keyIdx, end, true
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
