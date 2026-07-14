package project

import (
	"fmt"
	"sort"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/sysopen"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// ---------- browse rows ----------

// addonDesc renders an addon row's status line from its inspected state.
func addonDesc(s addon.Status) string {
	// Git checkouts (clone/submodule) are working copies, not version-pinned
	// packages: describe them by their tracked branch and whether the checkout is
	// present. A submodule is parent-managed, so it can't be "cloned" by gdaddon.
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

// addonItem builds one browse row from its rowData — the inspected status plus the cached
// warning flags rowMarker decorates the name with. A non-installable addon gets a nil Pick
// (an inert row), which replaces the old Installable() gate in the screen's Update.
func addonItem(r rowData) components.Item {
	s := r.s
	var pick func(*core.Shared) core.Action
	if s.Installable() {
		pick = func(sh *core.Shared) core.Action { return core.Push(newSubmenuScreen(s, sh)) }
	}
	// Any addon present on disk (package or git checkout) gets a "t" shortcut to open
	// a terminal at its install path (the framework dispatches Item.Keys for the
	// highlighted row, see RootUpdate).
	var keys func(*core.Shared, string) (core.Action, bool)
	if s.Present() {
		keys = func(sh *core.Shared, k string) (core.Action, bool) {
			if core.MatchKey(k, appctx.AppKeys.Terminal) {
				return sysopen.Terminal(s.FullPath), true
			}
			return core.Action{}, false
		}
	}
	return components.Item{Name: s.Addon.Name + rowMarker(r), Desc: addonDesc(s), Pick: pick, Keys: keys}
}

// depsNeedAttention reports whether any declared dependency still needs the user's
// action: not suppressed and not yet installed-and-satisfying (missing from the
// manifest, in the manifest but not on disk, or installed but outdated). It's what
// drives the "missing deps" row marker, so the warning persists until every
// non-suppressed dep is actually installed.
func depsNeedAttention(statuses []addon.DepStatus) bool {
	for _, ds := range statuses {
		if !ds.Suppressed && ds.State != addon.DepInstalled {
			return true
		}
	}
	return false
}

// rowMarker builds the combined name suffix from every warning the row carries — update,
// branch drift, upstream divergence, dependencies, git-dirty — e.g. "  ⚠ [update]",
// "  ⚠ [behind origin 3]", or "  ⚠ [ahead 2 / uncommitted changes]". Empty when the addon
// is current, on its recorded branch, in sync with its upstream, its deps are satisfied,
// and (for a checkout) its working tree is clean. The ahead/behind counts are as fresh as
// the last fetch (see appctx.Ctx.GitSync).
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
	if r.dirty {
		parts = append(parts, "uncommitted changes")
	}
	if len(parts) == 0 {
		return ""
	}
	return "  ⚠ [" + strings.Join(parts, " / ") + "]"
}

// projectSortModes is the Project tab's sort cycle: name A→Z, name Z→A, then
// grouped by install state. The "i" key advances through it (see ProjectScreen.Update).
var projectSortModes = []appctx.SortMode{appctx.SortAlpha, appctx.SortReverse, appctx.SortStatus}

// rowData pairs an inspected addon with its cached warning flags (the same signals
// rowMarker draws), so a row can be both sorted — the status mode factors warnings,
// not just install state — and built from one value.
type rowData struct {
	s      addon.Status
	update bool          // a newer release exists (UpdateAvailable; excludes locked/current/unknown)
	deps   bool          // has unsatisfied dependencies
	dirty  bool          // git checkout has uncommitted changes
	sync   addon.GitSync // git checkout's divergence from its upstream, as of the last fetch
}

// projectListItems builds the browse list contents: one row per addon, decorated
// with the cached update-check and dependency-check markers, ordered per mode.
func projectListItems(sh *core.Shared, mode appctx.SortMode) []list.Item {
	c := appctx.Of(sh)
	statuses := inspect(sh)
	rows := make([]rowData, len(statuses))
	for i, s := range statuses {
		rows[i] = rowData{
			s:      s,
			update: c.UpdateChecks[s.Addon.Name].State == addon.UpdateAvailable,
			deps:   depsNeedAttention(c.DepStatuses[s.Addon.Name]),
			dirty:  c.GitDirty[s.Addon.Name],
			sync:   c.GitSync[s.Addon.Name],
		}
	}
	sortRows(rows, mode)
	items := make([]list.Item, len(rows))
	for i, r := range rows {
		items[i] = addonItem(r)
	}
	return items
}

// sortRows reorders rows in place for the chosen mode: A→Z / Z→A by name
// (case-insensitive), or by attentionRank (install state + warnings) with a name
// tie-break. Sorting this domain-aware slice — not the finished rows — keeps the
// status mode keyed on real state/warnings rather than the marker-suffixed Title.
func sortRows(rows []rowData, mode appctx.SortMode) {
	name := func(i int) string { return strings.ToLower(rows[i].s.Addon.Name) }
	switch mode {
	case appctx.SortReverse:
		sort.SliceStable(rows, func(i, j int) bool { return name(i) > name(j) })
	case appctx.SortStatus:
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

// Attention tiers for SortStatus, most-urgent (lowest) first: install-state issues,
// then the three warnings in the order the user cares about (update → deps → dirty),
// then settled/installed, with invalid at the bottom. Reorder these to retune the
// status sort — they're the single source of the ordering.
const (
	rankMissing     = iota // not installed
	rankMismatch           // installed version != pinned
	rankBranch             // git checkout on a different branch than recorded
	rankBehind             // git checkout behind its upstream — there's something to pull
	rankUpdate             // a newer release is available
	rankDeps               // unsatisfied dependencies
	rankDirty              // uncommitted changes in a git checkout
	rankAhead              // unpushed local commits — informational, nothing is broken
	rankUnversioned        // installed, no version pinned
	rankInstalled          // installed and clean
	rankInvalid            // broken entry (missing url/path)
)

// attentionRank scores a row for the status sort: a base rank from install state,
// which any warning can only raise in urgency (take the minimum). So an installed
// addon with an available update ranks at the "update" tier, not "installed".
// Invalid entries carry no install path (hence no warnings) and sort to the bottom.
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
	if r.sync.Ahead > 0 && rankAhead < base {
		base = rankAhead
	}
	return base
}

// ---------- install payload ----------

// versionItem is a leaf choice (a branch or a release asset) carried through
// confirm/install. It is a payload built from a packages.Selection at the install
// endpoint boundary (see installEndpoint).
type versionItem struct {
	tag           string
	asset         source.Asset
	archivedAsset source.Asset // local archived copy of this version, if any (enables the install source toggle); zero = none
	repoID        string       // canonical host/owner/repo, used to build the .git url for a clone install
	branch        bool
	archived      bool // asset comes from the local archive (local-file URL)
	clone         bool // install the branch as a live git working copy (keeps .git) instead of an unzipped package
}

// pickSection describes the chosen asset for the confirm screen's title, e.g.
// "Assets v1.0.0 - addon.zip" or "Branches - main".
func pickSection(pick versionItem) string {
	if pick.branch {
		return "Branches - " + pick.tag
	}
	return fmt.Sprintf("Assets %s - %s", pick.tag, pick.asset.Name)
}
