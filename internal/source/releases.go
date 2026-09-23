package source

import (
	"sort"

	"github.com/brohd11/gdaddon/internal/version"
)

// IsPrerelease honors both provider metadata and semantic tag suffixes.
func (r Release) IsPrerelease() bool {
	return r.Prerelease || version.IsPrerelease(r.Tag)
}

// SortReleases orders semantic tags descending, then uncomparable tags in original order
// (stable for equal versions). It only reorders.
func SortReleases(releases []Release) {
	sort.SliceStable(releases, func(i, j int) bool {
		a, b := releases[i].Tag, releases[j].Tag
		if order, ok := version.Compare(a, b); ok {
			return order > 0
		}
		return version.IsValid(a) && !version.IsValid(b)
	})
}
