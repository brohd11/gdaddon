package project

import (
	"fmt"
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

// addonItem builds one browse row. A non-installable addon gets a nil Pick (an
// inert row), which replaces the old Installable() gate in the screen's Update.
// upd is the cached update-check result and missingDeps whether the addon has
// unsatisfied dependencies; rowMarker decorates the name from both.
func addonItem(s addon.Status, upd addon.UpdateInfo, missingDeps, dirty bool) components.Item {
	var pick func(*core.Shared) core.Action
	if s.Installable() {
		pick = func(sh *core.Shared) core.Action { return core.Push(newSubmenuScreen(s, sh)) }
	}
	// A present git checkout (clone/submodule) gets a "t" shortcut to open a terminal
	// at its install path (the framework dispatches Item.Keys for the highlighted row,
	// see RootUpdate).
	var keys func(*core.Shared, string) (core.Action, bool)
	if s.Addon.IsGitWorkdir() && s.Present() {
		keys = func(sh *core.Shared, k string) (core.Action, bool) {
			if k == "t" {
				return sysopen.Terminal(s.FullPath), true
			}
			return core.Action{}, false
		}
	}
	name := s.Addon.Name + rowMarker(upd, missingDeps, dirty, s.State == addon.StateBranchChanged)
	return components.Item{Name: name, Desc: addonDesc(s), Pick: pick, Keys: keys}
}

// rowMarker builds the combined name suffix from the update, branch-drift,
// dependency, and git-dirty checks, e.g. "  ⚠ [update]", "  ⚠ [branch changed]", or
// "  ⚠ [missing deps / uncommitted changes]". Empty when the addon is current, on its
// recorded branch, its deps are satisfied, and (for a clone) its working tree is clean.
func rowMarker(upd addon.UpdateInfo, missingDeps, dirty, branchChanged bool) string {
	var parts []string
	if upd.State == addon.UpdateAvailable {
		parts = append(parts, "update")
	}
	if branchChanged {
		parts = append(parts, "branch changed")
	}
	if missingDeps {
		parts = append(parts, "missing deps")
	}
	if dirty {
		parts = append(parts, "uncommitted changes")
	}
	if len(parts) == 0 {
		return ""
	}
	return "  ⚠ [" + strings.Join(parts, " / ") + "]"
}

// projectListItems builds the browse list contents: one row per addon, decorated
// with the cached update-check and dependency-check markers.
func projectListItems(sh *core.Shared) []list.Item {
	statuses := inspect(sh)
	c := appctx.Of(sh)
	items := make([]list.Item, 0, len(statuses))
	for _, s := range statuses {
		items = append(items, addonItem(s, c.UpdateChecks[s.Addon.Name], len(c.DepChecks[s.Addon.Name]) > 0, c.GitDirty[s.Addon.Name]))
	}
	return items
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
