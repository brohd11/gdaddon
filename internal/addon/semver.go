package addon

import "github.com/brohd11/gdaddon/internal/version"

// SatisfiedByTag reports whether an installed entry on installedTag meets this
// dependency's required tag. verified is false when either tag isn't a comparable
// semantic version (a date stamp, a branch-HEAD entry with no tag, …), so the
// caller can surface it as "can't verify" rather than a definite miss.
func (d Dependency) SatisfiedByTag(installedTag string) (satisfied, verified bool) {
	ge, ok := semverGE(installedTag, d.Tag)
	if !ok {
		return false, false
	}
	return ge, true
}

// SemverGE reports whether a is at least b in semantic precedence and whether
// both versions are comparable.
func SemverGE(a, b string) (ge, ok bool) { return semverGE(a, b) }

// semverGE retains prerelease precedence and ignores only build metadata.
func semverGE(a, b string) (ge, ok bool) {
	order, ok := version.Compare(a, b)
	return ok && order >= 0, ok
}
