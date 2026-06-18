package project

import (
	"fmt"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"

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
func addonItem(s addon.Status) components.Item {
	var pick func(*core.Shared) core.Action
	if s.Installable() {
		pick = func(sh *core.Shared) core.Action { return core.Push(newSubmenuScreen(s, sh)) }
	}
	return components.Item{Name: s.Addon.Name, Desc: addonDesc(s), Pick: pick}
}

// projectListItems builds the browse list contents: one row per addon.
func projectListItems(sh *core.Shared) []list.Item {
	statuses := inspect(sh)
	items := make([]list.Item, 0, len(statuses))
	for _, s := range statuses {
		items = append(items, addonItem(s))
	}
	return items
}

// ---------- install payload ----------

// versionItem is a leaf choice (a branch or a release asset) carried through
// confirm/install. It is a payload built from a packages.Selection at the install
// endpoint boundary (see installEndpoint).
type versionItem struct {
	tag      string
	asset    source.Asset
	branch   bool
	archived bool // asset comes from the local archive (local-file URL)
}

// pickSection describes the chosen asset for the confirm breadcrumb, e.g.
// "Assets v1.0.0 - addon.zip" or "Branches - main".
func pickSection(pick versionItem) string {
	if pick.branch {
		return "Branches - " + pick.tag
	}
	return fmt.Sprintf("Assets %s - %s", pick.tag, pick.asset.Name)
}
