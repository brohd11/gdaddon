package git

import (
	"strings"
	"testing"

	"gdaddon/internal/addon"
)

// The confirm/batch rendering is tested in the shared repoui package; what's gdaddon's own
// here is which manifest checkouts each scope selects.

func TestScopeMatches(t *testing.T) {
	clone := addon.Addon{Kind: addon.KindClone}
	sub := addon.Addon{Kind: addon.KindSubmodule}
	pkg := addon.Addon{Kind: addon.KindPackage}

	cases := []struct {
		sc              scope
		clone, sub, pkg bool
	}{
		{scopeClones, true, false, false},
		{scopeSubmodules, false, true, false},
		{scopeAll, true, true, false}, // "all" still means all *git* checkouts; a package is never one
	}
	for _, c := range cases {
		if got := c.sc.matches(clone); got != c.clone {
			t.Errorf("%s.matches(clone) = %v, want %v", c.sc.label(), got, c.clone)
		}
		if got := c.sc.matches(sub); got != c.sub {
			t.Errorf("%s.matches(submodule) = %v, want %v", c.sc.label(), got, c.sub)
		}
		if got := c.sc.matches(pkg); got != c.pkg {
			t.Errorf("%s.matches(package) = %v, want %v", c.sc.label(), got, c.pkg)
		}
	}
}

func TestScopeLabels(t *testing.T) {
	got := strings.Join([]string{scopeClones.label(), scopeSubmodules.label(), scopeAll.label()}, " ")
	if got != "clones submodules all" {
		t.Errorf("scope labels = %q, want %q", got, "clones submodules all")
	}
}
