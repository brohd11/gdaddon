package project

import (
	"fmt"
	"sort"
	"strings"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/source"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gdaddon/internal/tui/sysopen"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repoui"

	"charm.land/bubbles/v2/list"
)

// ---------- browse rows ----------

// addonDesc renders an addon row's status line from its inspected state.
func addonDesc(s addon.Status) string {
	// Describe git checkouts by branch and presence; a submodule is parent-managed.
	if s.Addon.IsSubmodule() {
		branch := s.Addon.Tag
		if branch == "" {
			branch = "HEAD"
		}
		if s.State == addon.StateBranchChanged {
			return "⛓ submodule · recorded " + branch + " → on " + s.LiveBranch
		}
		if s.Present() {
			return "⛓ submodule · " + branch
		}
		return "⛓ submodule (missing) · branch " + branch
	}
	if s.Addon.IsClone() {
		branch := s.Addon.Tag
		if branch == "" {
			branch = "HEAD"
		}
		if s.State == addon.StateBranchChanged {
			return "⎇ cloned (dev) · recorded " + branch + " → on " + s.LiveBranch
		}
		if s.Present() {
			return "⎇ cloned (dev) · " + branch
		}
		return "⎇ not cloned · branch " + branch
	}
	desc := ""
	switch s.State {
	case addon.StateInvalid:
		desc = "✗ invalid — missing url or path"
	case addon.StateMissing:
		switch {
		case s.Addon.Version != "":
			desc = "• not installed — target " + s.Addon.Version
		case s.Addon.Tag != "":
			desc = "• not installed — target " + s.Addon.Tag
		default:
			desc = "• not installed"
		}
	case addon.StateInstalled:
		desc = fmt.Sprintf("✓ installed v%s", s.LocalVersion)
	case addon.StateUnversioned:
		desc = "✓ installed (no version pinned)"
	case addon.StateMismatch:
		local := s.LocalVersion
		if local == "" {
			local = "unknown"
		}
		desc = fmt.Sprintf("⚠ manifest pins %s, installed %s", s.Addon.Version, local)
	}
	// Lock is only ever set on package entries (the toggle is gated to non-git-workdir),
	// so the note rides after the install status here, e.g. "✓ installed v1.0.0 · 🔒 locked".
	if s.Addon.Lock {
		if desc != "" {
			desc += " · 🔒 locked"
		} else {
			desc = "🔒 locked"
		}
	}
	return desc
}

// addonItem builds one row with its warning marker. Non-installable addons are inert rows
// (nil Pick).
func addonItem(r rowData) components.Item {
	s := r.s
	var pick func(*core.Shared) core.Action
	if s.Installable() {
		pick = func(sh *core.Shared) core.Action { return core.Push(newSubmenuScreen(s, sh)) }
	}
	// A present row's keys: "t" and "T" open a terminal at the install path; "v" (Git menu)
	// and "d" (diff list) only for git checkouts, otherwise falling through.
	var keys func(*core.Shared, string) (core.Action, bool)
	if s.Present() {
		keys = func(sh *core.Shared, k string) (core.Action, bool) {
			switch {
			case core.MatchKey(k, appctx.AppKeys.Terminal):
				return sysopen.TerminalInline(s.FullPath), true
			case core.MatchKey(k, appctx.AppKeys.TerminalWindow):
				return sysopen.Terminal(s.FullPath), true
			case core.MatchKey(k, appctx.AppKeys.OpenDir):
				return sysopen.Path(s.FullPath, false), true
			case core.MatchKey(k, appctx.AppKeys.Git) && s.Addon.IsGitWorkdir():
				rp := repoFromStatus(s)
				return core.Push(repoui.RepoMenu(sh, rp, rp.Name)), true
			case core.MatchKey(k, appctx.AppKeys.Diff) && s.Addon.IsGitWorkdir():
				rp := repoFromStatus(s)
				return repoui.DiffAction(sh, rp, rp.Name), true
			}
			return core.Action{}, false
		}
	}
	return components.Item{Name: s.Addon.Label() + rowMarker(r), Desc: addonDesc(s), Pick: pick, Keys: keys}
}

// depsNeedAttention reports any unsuppressed dependency not installed and satisfying,
// which drives the "missing deps" marker.
func depsNeedAttention(statuses []addon.DepStatus) bool {
	for _, ds := range statuses {
		if !ds.Suppressed && ds.State != addon.DepInstalled {
			return true
		}
	}
	return false
}

// rowMarker combines the row's warnings (update, branch drift, divergence, deps, dirty)
// into one suffix, e.g. "  ⚠ [behind origin 3]"; empty when all is well. Counts are as
// fresh as the last fetch.
func rowMarker(r rowData) string {
	var parts []string
	if r.update {
		parts = append(parts, "update")
	}
	if r.s.State == addon.StateBranchChanged {
		parts = append(parts, "branch changed")
	}
	if r.sync.Behind > 0 {
		parts = append(parts, fmt.Sprintf("behind origin %d", r.sync.Behind))
	}
	if r.sync.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("ahead %d", r.sync.Ahead))
	}
	if r.deps {
		parts = append(parts, "missing deps")
	}
	if r.orphan {
		parts = append(parts, "unused dependency")
	}
	if r.dirty {
		parts = append(parts, "uncommitted changes")
	}
	if len(parts) == 0 {
		return ""
	}
	return "  ⚠ [" + strings.Join(parts, " / ") + "]"
}

// projectSortModes is the "i" key's cycle: A→Z, Z→A, by status, by status with
// uninstalled rows hidden.
var projectSortModes = []appctx.SortMode{appctx.SortAlpha, appctx.SortReverse, appctx.SortStatus, appctx.SortStatusInstalled}

// visibleRows hides uninstalled addons under SortStatusInstalled.
func visibleRows(rows []rowData, mode appctx.SortMode) []rowData {
	if mode != appctx.SortStatusInstalled {
		return rows
	}
	kept := rows[:0]
	for _, r := range rows {
		if r.s.Present() {
			kept = append(kept, r)
		}
	}
	return kept
}

// rowData pairs an inspected addon with its warning flags, for sorting and building rows.
type rowData struct {
	s      addon.Status
	update bool          // a newer release exists (UpdateAvailable; excludes locked/current/unknown)
	deps   bool          // has unsatisfied dependencies
	orphan bool          // an is_dependency entry no longer required by anything installed
	dirty  bool          // git checkout has uncommitted changes
	sync   addon.GitSync // git checkout's divergence from its upstream, as of the last fetch
}

// projectListItems builds a row per addon, with markers, in mode order. The project root
// has no row (it is in the header).
func projectListItems(sh *core.Shared, mode appctx.SortMode) []list.Item {
	c := appctx.Of(sh)
	statuses := inspect(sh)
	rows := make([]rowData, len(statuses))
	for i, s := range statuses {
		rows[i] = rowData{
			s:      s,
			update: c.UpdateChecks[s.Addon.Name].State == addon.UpdateAvailable,
			deps:   depsNeedAttention(c.DepStatuses[s.Addon.Name]),
			orphan: c.OrphanDeps[s.Addon.Name],
			dirty:  c.GitDirty[s.Addon.Name],
			sync:   c.GitSync[s.Addon.Name],
		}
	}
	rows = visibleRows(rows, mode)
	sortRows(rows, mode)
	items := make([]list.Item, len(rows))
	for i, r := range rows {
		items[i] = addonItem(r)
	}
	return items
}

// sortRows orders rows by name (case-insensitive, either way) or by attentionRank with a
// name tie-break, on real state rather than the marked-up titles.
func sortRows(rows []rowData, mode appctx.SortMode) {
	name := func(i int) string { return strings.ToLower(rows[i].s.Addon.Label()) }
	switch mode {
	case appctx.SortReverse:
		sort.SliceStable(rows, func(i, j int) bool { return name(i) > name(j) })
	case appctx.SortStatus, appctx.SortStatusInstalled:
		sort.SliceStable(rows, func(i, j int) bool {
			ri, rj := attentionRank(rows[i]), attentionRank(rows[j])
			if ri != rj {
				return ri < rj
			}
			return name(i) < name(j)
		})
	default: // SortAlpha
		sort.SliceStable(rows, func(i, j int) bool { return name(i) < name(j) })
	}
}

// Status sort tiers, most urgent first: install issues, then update, deps, dirty, then
// installed, invalid last.
const (
	rankMissing     = iota // not installed
	rankMismatch           // installed version != pinned
	rankBranch             // git checkout on a different branch than recorded
	rankBehind             // git checkout behind its upstream — there's something to pull
	rankUpdate             // a newer release is available
	rankDeps               // unsatisfied dependencies
	rankDirty              // uncommitted changes in a git checkout
	rankOrphan             // an is_dependency entry nothing installed needs — a cleanup hint
	rankAhead              // unpushed local commits — informational, nothing is broken
	rankUnversioned        // installed, no version pinned
	rankInstalled          // installed and clean
	rankInvalid            // broken entry (missing url/path)
)

// attentionRank is a row's tier: the base install tier, raised by any warning.
func attentionRank(r rowData) int {
	base := rankInstalled
	switch r.s.State {
	case addon.StateMissing:
		base = rankMissing
	case addon.StateMismatch:
		base = rankMismatch
	case addon.StateBranchChanged:
		base = rankBranch
	case addon.StateUnversioned:
		base = rankUnversioned
	case addon.StateInvalid:
		return rankInvalid
	}
	// Warnings raise urgency (lower the number) but never lower it.
	if r.sync.Behind > 0 && rankBehind < base {
		base = rankBehind
	}
	if r.update && rankUpdate < base {
		base = rankUpdate
	}
	if r.deps && rankDeps < base {
		base = rankDeps
	}
	if r.dirty && rankDirty < base {
		base = rankDirty
	}
	if r.orphan && rankOrphan < base {
		base = rankOrphan
	}
	if r.sync.Ahead > 0 && rankAhead < base {
		base = rankAhead
	}
	return base
}

// ---------- install payload ----------

// versionItem is a chosen branch or release asset carried through confirm and install.
type versionItem struct {
	tag           string
	asset         source.Asset
	archivedAsset source.Asset // local archived copy of this version, if any (enables the install source toggle); zero = none
	repoID        string       // canonical host/owner/repo, used to build the .git url for a clone install
	branch        bool
	archived      bool // asset comes from the local archive (local-file URL)
	clone         bool // install the branch as a live git working copy (keeps .git) instead of an unzipped package
}
