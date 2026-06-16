package project

import (
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
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
	var pick func(*core.Shared) tea.Cmd
	if s.Installable() {
		pick = func(sh *core.Shared) tea.Cmd { return core.Push(newSubmenuScreen(s)) }
	}
	return components.Item{Name: s.Addon.Name, Desc: addonDesc(s), Pick: pick}
}

// addonListItems builds the browse list contents: one row per addon.
func addonListItems(statuses []addon.Status) []list.Item {
	items := make([]list.Item, 0, len(statuses))
	for _, s := range statuses {
		items = append(items, addonItem(s))
	}
	return items
}

// ---------- install payload ----------

// versionItem is a leaf choice (a branch or a release asset) carried through
// confirm/install. It is a payload, not a list row — the version/asset/branch
// pickers are built from components.Item (see versions.go).
type versionItem struct {
	tag        string
	asset      source.Asset
	prerelease bool
	branch     bool
	archived   bool // asset comes from the local archive (local-file URL)
}

// isArchived reports whether an asset is a local archive entry (local-file URL)
// rather than a remote download.
func isArchived(a source.Asset) bool { return !strings.HasPrefix(a.URL, "http") }

// pickSection describes the chosen asset for the confirm breadcrumb, e.g.
// "Assets v1.0.0 - addon.zip" or "Branches - main".
func pickSection(pick versionItem) string {
	if pick.branch {
		return "Branches - " + pick.tag
	}
	return fmt.Sprintf("Assets %s - %s", pick.tag, pick.asset.Name)
}
