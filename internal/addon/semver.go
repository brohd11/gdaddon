package addon

import "github.com/brohd11/gdaddon/internal/version"

// SatisfiedByTag reports whether an entry on installedTag meets d's tag. verified is false
// when either tag is not a comparable version, meaning "can't verify".
func (d Dependency) SatisfiedByTag(installedTag string) (satisfied, verified bool) {
	ge, ok := semverGE(installedTag, d.Tag)
	if !ok {
		return false, false
	}
	return ge, true
}

// semverGE retains prerelease precedence and ignores only build metadata.
func semverGE(a, b string) (ge, ok bool) {
	order, ok := version.Compare(a, b)
	return ok && order >= 0, ok
}
