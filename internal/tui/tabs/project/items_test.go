package project

import (
	"testing"

	"gdaddon/internal/addon"
)

// TestRowMarker covers the warning suffix a row's name carries, including the case that
// motivated the git-sync work: several warnings on one checkout compose into one bracket.
func TestRowMarker(t *testing.T) {
	sync := func(ahead, behind int) addon.GitSync {
		return addon.GitSync{Ahead: ahead, Behind: behind, Tracking: true}
	}
	cases := []struct {
		name string
		r    rowData
		want string
	}{
		{"clean", row("a", addon.StateInstalled), ""},
		{"in sync with upstream", withSyncRow(row("a", addon.StateUnversioned), sync(0, 0)), ""},
		{"behind", withSyncRow(row("a", addon.StateUnversioned), sync(0, 3)), "  ⚠ [behind origin 3]"},
		{"ahead", withSyncRow(row("a", addon.StateUnversioned), sync(2, 0)), "  ⚠ [ahead 2]"},
		{"diverged", withSyncRow(row("a", addon.StateUnversioned), sync(2, 3)), "  ⚠ [behind origin 3 / ahead 2]"},
		{"branch drift", row("a", addon.StateBranchChanged), "  ⚠ [branch changed]"},
	}
	for _, c := range cases {
		if got := rowMarker(c.r); got != c.want {
			t.Errorf("%s: rowMarker = %q, want %q", c.name, got, c.want)
		}
	}

	// The compound case: a checkout that is behind, has unpushed work, and hasn't committed
	// all of it — every signal the user needs before switching projects, in one marker.
	busy := withSyncRow(row("a", addon.StateUnversioned), sync(1, 2))
	busy.dirty = true
	busy.deps = true
	want := "  ⚠ [behind origin 2 / ahead 1 / missing deps / uncommitted changes]"
	if got := rowMarker(busy); got != want {
		t.Errorf("compound: rowMarker = %q, want %q", got, want)
	}
}

func withSyncRow(r rowData, s addon.GitSync) rowData {
	r.sync = s
	return r
}
