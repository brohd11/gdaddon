package project

import (
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// ---------- browse rows ----------

// addonDesc renders an addon row's status line from its inspected state.
func addonDesc(s addon.Status) string {
	switch s.State {
	case addon.StateInvalid:
		return "✗ invalid — missing url or path"
	case addon.StateMissing:
		if s.Addon.Version != "" {
			return "• not installed — target v" + s.Addon.Version
		}
		return "• not installed"
	case addon.StateInstalled:
		return fmt.Sprintf("✓ installed v%s", s.LocalVersion)
	case addon.StateUnversioned:
		return "✓ installed (no version pinned)"
	case addon.StateMismatch:
		local := s.LocalVersion
		if local == "" {
			local = "unknown"
		}
		return fmt.Sprintf("⚠ manifest pins %s, installed %s", s.Addon.Version, local)
	}
	return ""
}

// addonItem builds one browse row. A non-installable addon gets a nil Pick (an
// inert row), which replaces the old Installable() gate in the screen's Update.
// upd is the cached update-check result and missingDeps whether the addon has
// unsatisfied dependencies; rowMarker decorates the name from both.
func addonItem(s addon.Status, upd addon.UpdateInfo, missingDeps bool) components.Item {
	var pick func(*core.Shared) core.Action
	if s.Installable() {
		pick = func(sh *core.Shared) core.Action { return core.Push(newSubmenuScreen(s, sh)) }
	}
	return components.Item{Name: s.Addon.Name + rowMarker(upd, missingDeps), Desc: addonDesc(s), Pick: pick}
}

// rowMarker builds the combined name suffix from the update and dependency checks,
// e.g. "  ⚠ [update]", "  ⚠ [missing deps]", or "  ⚠ [update / missing deps]".
// Empty when the addon is current and its deps are satisfied.
func rowMarker(upd addon.UpdateInfo, missingDeps bool) string {
	var parts []string
	if upd.State == addon.UpdateAvailable {
		parts = append(parts, "update")
	}
	if missingDeps {
		parts = append(parts, "missing deps")
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
		items = append(items, addonItem(s, c.UpdateChecks[s.Addon.Name], len(c.DepChecks[s.Addon.Name]) > 0))
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
	branch        bool
	archived      bool // asset comes from the local archive (local-file URL)
}

// pickSection describes the chosen asset for the confirm breadcrumb, e.g.
// "Assets v1.0.0 - addon.zip" or "Branches - main".
func pickSection(pick versionItem) string {
	if pick.branch {
		return "Branches - " + pick.tag
	}
	return fmt.Sprintf("Assets %s - %s", pick.tag, pick.asset.Name)
}
