package addon

import (
	"strconv"
	"strings"
)

// SatisfiedByTag reports whether an installed entry on installedTag meets this
// dependency's required tag. verified is false when either tag isn't a comparable
// dotted-numeric version (a date stamp, a branch-HEAD entry with no tag, …), so the
// caller can surface it as "can't verify" rather than a definite miss.
func (d Dependency) SatisfiedByTag(installedTag string) (satisfied, verified bool) {
	ge, ok := semverGE(installedTag, d.Tag)
	if !ok {
		return false, false
	}
	return ge, true
}

// SemverGE is the exported wrapper over semverGE, for callers outside the package
// (e.g. self-update comparing the running binary's version against the latest tag).
func SemverGE(a, b string) (ge, ok bool) { return semverGE(a, b) }

// semverGE reports whether version a is >= version b, treating both as dotted
// numeric versions. A leading "v" and any pre-release/build suffix (after "-"/"+")
// are ignored. ok is false when either side has no comparable numeric components.
func semverGE(a, b string) (ge, ok bool) {
	na, oka := numericParts(a)
	nb, okb := numericParts(b)
	if !oka || !okb {
		return false, false
	}
	for i := 0; i < len(na) || i < len(nb); i++ {
		var x, y int
		if i < len(na) {
			x = na[i]
		}
		if i < len(nb) {
			y = nb[i]
		}
		if x != y {
			return x > y, true
		}
	}
	return true, true
}

func numericParts(v string) ([]int, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	// Strip a semver pre-release/build suffix, but only when the part before it
	// looks like a dotted version — so a date stamp like "2024-01-02" stays
	// non-numeric (uncomparable) rather than truncating to its first field.
	if i := strings.IndexAny(v, "-+"); i >= 0 && strings.Contains(v[:i], ".") {
		v = v[:i]
	}
	if v == "" {
		return nil, false
	}
	parts := strings.Split(v, ".")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, false
		}
		nums = append(nums, n)
	}
	return nums, true
}
