package tui

import (
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
)

// ---------- list items ----------

// menuItem is the entry pinned to the top of the browse list; selecting it opens
// the Actions submenu (install all, add plugin, future config).
type menuItem struct{}

func (menuItem) Title() string       { return "☰ Actions" }
func (menuItem) FilterValue() string { return "actions menu" }
func (menuItem) Description() string { return "install all · global plugins" }

// actionKind identifies a row in the Actions submenu.
type actionKind int

const (
	actInstallAll actionKind = iota
	actNewPlugin
	actImportPlugin
)

// actionItem is one row in the Actions submenu.
type actionItem struct {
	title string
	desc  string
	kind  actionKind
}

func (a actionItem) Title() string       { return a.title }
func (a actionItem) FilterValue() string { return a.title }
func (a actionItem) Description() string { return a.desc }

// actionItems builds the Actions submenu rows.
func actionItems() []list.Item {
	return []list.Item{
		actionItem{title: "↧ Install / update all", desc: "download everything per the manifest", kind: actInstallAll},
		actionItem{title: "+ New Plugin", desc: "add a plugin to the project or your global list", kind: actNewPlugin},
		actionItem{title: "⬇ Import Plugin", desc: "add a plugin from your global list", kind: actImportPlugin},
	}
}

// importItem is one row in the Import Plugin picker (an entry from the global
// list); selecting it copies the entry into the project manifest.
type importItem struct {
	name string
	url  string
	path string
}

func (i importItem) Title() string       { return i.name }
func (i importItem) FilterValue() string { return i.name }
func (i importItem) Description() string { return i.url }

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

// addonListItems builds the browse list contents: the pinned Actions menu first,
// then one row per addon (so addon index i lives at list index i+1 — see
// applyStatuses).
func addonListItems(statuses []addon.Status) []list.Item {
	items := make([]list.Item, 0, len(statuses)+1)
	items = append(items, menuItem{})
	for _, s := range statuses {
		items = append(items, item{status: s})
	}
	return items
}

// newSelectList builds a list styled like the others (no status bar, help drawn
// separately, esc/enter hints) for the versions and submenu screens. It's sized
// to zero; the owning screen's SetSize gives it real dimensions.
func newSelectList(items []list.Item, title string, extra ...key.Binding) list.Model {
	l := list.New(items, newDelegate(), 0, 0)
	l.Title = title
	styleList(&l)
	keys := func() []key.Binding {
		return append([]key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		}, extra...)
	}
	l.AdditionalShortHelpKeys = keys
	l.AdditionalFullHelpKeys = keys
	return l
}

// archiveKey is the version-screen hint for the archive action.
var archiveKey = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "archive"))

// newDelegate is the shared list delegate with brightened description text.
func newDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(mutedColor)
	d.Styles.DimmedDesc = d.Styles.DimmedDesc.Foreground(mutedColor)
	return d
}

// styleList applies the shared list config: hide the built-in status bar and
// help (help is drawn manually at the bottom), and brighten the help colors.
func styleList(l *list.Model) {
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Help.Styles.ShortKey = l.Help.Styles.ShortKey.Foreground(mutedColor)
	l.Help.Styles.ShortDesc = l.Help.Styles.ShortDesc.Foreground(mutedColor)
	l.Help.Styles.ShortSeparator = l.Help.Styles.ShortSeparator.Foreground(mutedColor)
	l.Help.Styles.FullKey = l.Help.Styles.FullKey.Foreground(mutedColor)
	l.Help.Styles.FullDesc = l.Help.Styles.FullDesc.Foreground(mutedColor)
	l.Help.Styles.FullSeparator = l.Help.Styles.FullSeparator.Foreground(mutedColor)
}
