// Package version compares addon versions without discarding prerelease identity.
package version

import (
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

var dateTag = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

// normalize accepts optional v/V prefixes and shortened numeric cores, while
// leaving validation and precedence to semver. The original tag is never changed.
func normalize(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "v") || strings.HasPrefix(v, "V") {
		v = v[1:]
	}
	if dateTag.MatchString(v) {
		return ""
	}
	core, suffix := v, ""
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		core, suffix = v[:i], v[i:]
	}
	for strings.Count(core, ".") < 2 {
		core += ".0"
	}
	return "v" + core + suffix
}

// Compare returns -1, 0, or 1 and whether both versions are comparable.
// Build metadata does not affect precedence.
func Compare(a, b string) (order int, ok bool) {
	a, b = normalize(a), normalize(b)
	if !semver.IsValid(a) || !semver.IsValid(b) {
		return 0, false
	}
	return semver.Compare(a, b), true
}

// IsValid reports whether v is a supported semantic version.
func IsValid(v string) bool { return semver.IsValid(normalize(v)) }

// IsPrerelease reports whether v is a valid semantic prerelease tag.
func IsPrerelease(v string) bool { return semver.Prerelease(normalize(v)) != "" }
