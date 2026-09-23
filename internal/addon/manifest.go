package addon

import (
	"fmt"
	"os"
	"strings"
)

// UpdateEntry rewrites one entry's url, path, version and tag lines in place (inserting
// missing ones), leaving every other line byte-for-byte intact. Empty values leave their
// line untouched. It assumes the flat shape: keys at column 0, fields indented beneath.
func UpdateEntry(manifestPath, name, url, path, version, tag string) error {
	return writeEntryFields(manifestPath, name, url, path, version, tag, false)
}

// EditEntry is UpdateEntry with set-or-clear semantics: an empty value removes the line,
// as the Edit Manifest form needs. Kind is set with SetKind.
func EditEntry(manifestPath, name, url, path, version, tag string) error {
	return writeEntryFields(manifestPath, name, url, path, version, tag, true)
}

// writeEntryFields is the shared body of UpdateEntry and EditEntry; removeEmpty decides
// whether an empty value removes its line or leaves it.
func writeEntryFields(manifestPath, name, url, path, version, tag string, removeEmpty bool) error {
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
	var drop []int // indices of field lines to remove (empty values, removeEmpty only)
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
		switch {
		case val == "" && removeEmpty:
			drop = append(drop, i)
		case val != "":
			lines[i] = render(ind, key, val)
			// An empty value with removeEmpty == false leaves the existing line untouched.
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

// AddEntryFull appends an entry from a full Addon (deduplicated by repo, creating the file
// if needed): url and path, then a name line when it has one, version and tag lines when
// set, and a kind line for non-package kinds. Every "add a complete entry" path uses it,
// so they share one shape.
func AddEntryFull(manifestPath string, a Addon) error {
	if err := AddEntry(manifestPath, a.Name, a.URL, a.Path); err != nil {
		return err
	}
	if a.Display != "" {
		if err := SetDisplayName(manifestPath, a.Name, a.Display); err != nil {
			return err
		}
	}
	if a.Version != "" || a.Tag != "" {
		if err := UpdateEntry(manifestPath, a.Name, "", "", a.Version, a.Tag); err != nil {
			return err
		}
	}
	if a.Kind != KindPackage {
		if err := SetKind(manifestPath, a.Name, a.Kind); err != nil {
			return err
		}
	}
	if a.Lock {
		if err := SetLock(manifestPath, a.Name, true); err != nil {
			return err
		}
	}
	if a.Dependency {
		return SetIsDependency(manifestPath, a.Name, true)
	}
	return nil
}

// setScalarField inserts, updates (keep true, field is the rendered "key: value") or
// removes (keep false) one scalar line on entry name, in place. Backs SetKind, SetLock
// and SetCommit.
func setScalarField(manifestPath, name, key, field string, keep bool) error {
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
	fieldIdx := -1
	for i := keyIdx + 1; i < end; i++ {
		ind, k, ok := splitField(lines[i])
		if !ok {
			continue
		}
		indent = ind
		if k == key {
			fieldIdx = i
		}
	}

	switch {
	case !keep:
		if fieldIdx != -1 {
			lines = append(lines[:fieldIdx], lines[fieldIdx+1:]...)
		}
	case fieldIdx != -1:
		lines[fieldIdx] = indent + field
	default:
		tail := append([]string{indent + field}, lines[keyIdx+1:]...)
		lines = append(lines[:keyIdx+1], tail...)
	}

	return os.WriteFile(manifestPath, []byte(strings.Join(lines, "\n")), 0o644)
}

// SetKind sets `kind:` for clones and submodules and removes it for packages. Separate from
// UpdateEntry, whose "empty means untouched" rule would clash with "empty means package".
func SetKind(manifestPath, name string, kind Kind) error {
	return setScalarField(manifestPath, name, "kind", "kind: "+string(kind), kind != KindPackage)
}

// SetLock writes `lock: true`, or removes the line (absent means unlocked).
func SetLock(manifestPath, name string, lock bool) error {
	return setScalarField(manifestPath, name, "lock", "lock: true", lock)
}

// SetCommit writes `commit: "<sha>"` or removes it for "". Kept out of tag, which deps
// compare as semver.
func SetCommit(manifestPath, name, commit string) error {
	return setScalarField(manifestPath, name, "commit", `commit: "`+commit+`"`, commit != "")
}

// SetDisplayName writes `name: "<display>"` or removes it for "" (falling back to the
// slug). Always quoted: it is the author's free text, the only field that could otherwise
// break the YAML.
func SetDisplayName(manifestPath, name, display string) error {
	return setScalarField(manifestPath, name, "name", "name: "+quoteYAML(display), display != "")
}

// quoteYAML renders s as a YAML double-quoted scalar, escaping the two characters that
// would otherwise end it early, so arbitrary text round-trips back through Parse.
func quoteYAML(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// SetIsDependency writes `is_dependency: true`, or removes the line (absent means
// user-chosen).
func SetIsDependency(manifestPath, name string, isDep bool) error {
	return setScalarField(manifestPath, name, "is_dependency", "is_dependency: true", isDep)
}

// SetSuppressDeps writes `suppress_deps: ["a/b", "c/d"]` (source.RepoID values), or removes
// the line for an empty list.
func SetSuppressDeps(manifestPath, name string, ids []string) error {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = `"` + id + `"`
	}
	field := "suppress_deps: [" + strings.Join(quoted, ", ") + "]"
	return setScalarField(manifestPath, name, "suppress_deps", field, len(ids) > 0)
}

// RemoveEntry deletes an entry's key line and block in place, for the project manifest or
// the global list. Missing entries are an error.
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

// findEntryBlock returns the column-0 key line of entry name and the exclusive end of its
// indented block; ok is false when absent. Every in-place writer uses it.
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
