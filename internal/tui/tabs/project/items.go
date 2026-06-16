package project

import (
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
)

// ---------- list items ----------

type item struct{ status addon.Status }

func (i item) Title() string       { return i.status.Addon.Name }
func (i item) FilterValue() string { return i.status.Addon.Name }

func (i item) Description() string {
	s := i.status
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

// headItem is the top-of-list entry that opens the branch (refs/heads) submenu.
type headItem struct{}

func (headItem) Title() string       { return "HEAD" }
func (headItem) FilterValue() string { return "HEAD" }
func (headItem) Description() string { return "track a branch (refs/heads)" }

// releaseItem is one release in the top-level versions list. Selecting it either
// drops straight into confirm (single asset) or opens the asset submenu.
type releaseItem struct{ rel source.Release }

func (r releaseItem) Title() string       { return r.rel.Tag }
func (r releaseItem) FilterValue() string { return r.rel.Tag }

func (r releaseItem) Description() string {
	d := fmt.Sprintf("%d asset(s)", len(r.rel.Assets))
	if r.rel.Prerelease {
		d += " · prerelease"
	}
	return d
}

// versionItem is a leaf choice (a branch or a release asset) shown in a submenu
// and carried in m.pick through confirm/install.
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

func (v versionItem) Title() string {
	if v.branch {
		return "branch: " + v.tag
	}
	return v.asset.Name
}

func (v versionItem) Description() string {
	if v.branch {
		return "latest commit · " + v.asset.Name
	}
	d := v.tag
	if v.prerelease {
		d += " · prerelease"
	}
	return d
}

func (v versionItem) FilterValue() string { return v.tag + " " + v.asset.Name }

// pickSection describes the chosen asset for the confirm breadcrumb, e.g.
// "Assets v1.0.0 - addon.zip" or "Branches - main".
func pickSection(pick versionItem) string {
	if pick.branch {
		return "Branches - " + pick.tag
	}
	return fmt.Sprintf("Assets %s - %s", pick.tag, pick.asset.Name)
}

// versionTopItems builds the top-level versions list: HEAD first, then one entry
// per release (newest first).
func versionTopItems(l *source.Listing) []list.Item {
	items := []list.Item{headItem{}}
	for _, r := range l.Releases {
		items = append(items, releaseItem{rel: r})
	}
	return items
}

// assetItems builds the per-release asset submenu.
func assetItems(r source.Release) []list.Item {
	items := make([]list.Item, 0, len(r.Assets))
	for _, a := range r.Assets {
		items = append(items, versionItem{tag: r.Tag, asset: a, prerelease: r.Prerelease, archived: isArchived(a)})
	}
	return items
}

// branchItems builds the HEAD/branch submenu.
func branchItems(branches []source.Asset) []list.Item {
	items := make([]list.Item, 0, len(branches))
	for _, b := range branches {
		items = append(items, versionItem{tag: b.Name, asset: b, branch: true, archived: isArchived(b)})
	}
	return items
}

// addonListItems builds the browse list contents: one row per addon (Actions now
// lives in its own top-level tab, reached with [ / ]).
func addonListItems(statuses []addon.Status) []list.Item {
	items := make([]list.Item, 0, len(statuses))
	for _, s := range statuses {
		items = append(items, item{status: s})
	}
	return items
}

// archiveKey is the version-screen hint for the archive action.
var archiveKey = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "archive"))
