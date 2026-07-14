package project

import (
	"testing"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"
)

func row(name string, state addon.State) rowData {
	return rowData{s: addon.Status{Addon: addon.Addon{Name: name}, State: state}}
}

func names(rows []rowData) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.s.Addon.Name
	}
	return out
}

func TestSortRowsNames(t *testing.T) {
	base := func() []rowData {
		return []rowData{
			row("Zebra", addon.StateInstalled),
			row("apple", addon.StateMissing),
			row("Mango", addon.StateInstalled),
			row("beta", addon.StateMismatch),
		}
	}

	alpha := base()
	sortRows(alpha, appctx.SortAlpha)
	if want := []string{"apple", "beta", "Mango", "Zebra"}; !eq(names(alpha), want) {
		t.Errorf("alpha (case-insensitive) = %v, want %v", names(alpha), want)
	}

	rev := base()
	sortRows(rev, appctx.SortReverse)
	if want := []string{"Zebra", "Mango", "beta", "apple"}; !eq(names(rev), want) {
		t.Errorf("reverse = %v, want %v", names(rev), want)
	}

	stat := base()
	sortRows(stat, appctx.SortStatus)
	// missing(0) < mismatch(1) < installed(7); name tie-break within installed.
	if want := []string{"apple", "beta", "Mango", "Zebra"}; !eq(names(stat), want) {
		t.Errorf("status = %v, want %v", names(stat), want)
	}
}

// TestSortStatusWarnings proves warnings factor into the status sort: a warning lifts
// an otherwise-installed row above a clean installed one, in update→deps→dirty order,
// while genuine install-state issues (missing) still outrank all warnings.
func TestSortStatusWarnings(t *testing.T) {
	withWarn := func(name string, state addon.State, update, deps, dirty bool) rowData {
		r := row(name, state)
		r.update, r.deps, r.dirty = update, deps, dirty
		return r
	}
	rows := []rowData{
		withWarn("clean", addon.StateInstalled, false, false, false),
		withWarn("hasUpdate", addon.StateInstalled, true, false, false),
		withWarn("hasDirty", addon.StateInstalled, false, false, true),
		withWarn("hasDeps", addon.StateInstalled, false, true, false),
		withWarn("gone", addon.StateMissing, false, false, false),
	}
	sortRows(rows, appctx.SortStatus)
	want := []string{"gone", "hasUpdate", "hasDeps", "hasDirty", "clean"}
	if !eq(names(rows), want) {
		t.Errorf("status w/ warnings = %v, want %v", names(rows), want)
	}
}

// TestSortStatusGitSync proves upstream divergence factors into the status sort the way
// the workflow wants it: a checkout with something to pull outranks even an available
// release update, while unpushed local commits are informational and sink below a dirty
// tree (nothing is broken — you just haven't pushed).
func TestSortStatusGitSync(t *testing.T) {
	withSync := func(name string, ahead, behind int) rowData {
		r := row(name, addon.StateUnversioned)
		r.sync = addon.GitSync{Ahead: ahead, Behind: behind, Tracking: true}
		return r
	}
	hasUpdate := row("hasUpdate", addon.StateInstalled)
	hasUpdate.update = true
	dirty := row("dirty", addon.StateInstalled)
	dirty.dirty = true

	rows := []rowData{
		withSync("inSync", 0, 0),
		withSync("ahead", 2, 0),
		dirty,
		hasUpdate,
		withSync("behind", 0, 3),
	}
	sortRows(rows, appctx.SortStatus)
	want := []string{"behind", "hasUpdate", "dirty", "ahead", "inSync"}
	if !eq(names(rows), want) {
		t.Errorf("status w/ git sync = %v, want %v", names(rows), want)
	}
}

// TestAttentionRankLocked proves a locked entry (never UpdateAvailable) is not lifted
// by an update — it stays in the installed tier.
func TestAttentionRankLocked(t *testing.T) {
	locked := row("locked", addon.StateInstalled) // update stays false, as CheckUpdate reports UpdateLocked
	if got := attentionRank(locked); got != rankInstalled {
		t.Errorf("locked/installed rank = %d, want %d (installed)", got, rankInstalled)
	}
}

func TestNextSort(t *testing.T) {
	modes := projectSortModes
	if got := appctx.NextSort(appctx.SortAlpha, modes); got != appctx.SortReverse {
		t.Errorf("alpha -> %v, want reverse", got)
	}
	if got := appctx.NextSort(appctx.SortStatus, modes); got != appctx.SortAlpha {
		t.Errorf("status -> %v (should wrap), want alpha", got)
	}
	if got := appctx.NextSort(appctx.SortStatus, globalTwo()); got != globalTwo()[0] {
		t.Errorf("mode not in set -> %v, want first", got)
	}
}

func globalTwo() []appctx.SortMode {
	return []appctx.SortMode{appctx.SortAlpha, appctx.SortReverse}
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
