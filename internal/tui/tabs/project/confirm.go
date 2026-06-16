package project

import (
	"fmt"
	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/archive"
	"gdaddon/internal/source"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The confirm box mechanism lives in components.ConfirmScreen; the builders below
// supply its crumb/render/onYes closures for each feature (install / archive / new
// plugin). confirmHelp and newPluginConfirmHelp are the per-builder key hints.

var confirmHelp = []key.Binding{
	key.NewBinding(key.WithKeys("y", "enter"), key.WithHelp("y/enter", "confirm")),
	key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n/esc", "cancel")),
}

// ---------- install confirm ----------

func newInstallConfirm(selected addon.Addon, local string, pick versionItem) *components.ConfirmScreen {
	return &components.ConfirmScreen{
		Crumb:  core.RenderTitleBar(core.HeaderTitle(selected.Name, local, pickSection(pick))),
		Render: func(sh *core.Shared) string { return sh.Box(confirmInstallBody(sh, selected, pick)) },
		OnYes:  func(sh *core.Shared) tea.Cmd { return core.Replace(newInstallTask(selected, local, pick)) },
		Help:   confirmHelp,
	}
}

func confirmInstallBody(sh *core.Shared, selected addon.Addon, pick versionItem) string {
	// Hard-wrap the (space-less) URL to fit inside the box.
	urlBlock := core.IndentLines(core.HardWrap(pick.asset.URL, sh.ConfirmWidth()-4), "    ")
	return fmt.Sprintf(
		"Install %s\n\n  version:  %s\n  asset:    %s\n  path:     %s\n  url:\n%s",
		selected.Name, pick.tag, pick.asset.Name, selected.Path, urlBlock)
}

// ---------- remove confirm ----------

// remove modes (also the vertical option order).
const (
	removeProject      = iota // remove the manifest entry only
	removeProjectLocal        // also delete the installed files
)

var removeConfirmHelp = []key.Binding{
	key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "option")),
	key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "remove")),
	key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
}

// newRemoveConfirm builds the project Remove confirm: a vertical selector between
// removing just the manifest entry or that plus the installed files. ↑/↓ move the
// selection (via the confirm's OnKey), enter commits the chosen mode.
func newRemoveConfirm(st addon.Status) *components.ConfirmScreen {
	mode := removeProject // local copy the selector mutates; default = non-destructive
	return &components.ConfirmScreen{
		Crumb:  core.RenderTitleBar(core.HeaderTitle(st.Addon.Name, st.LocalVersion, "Remove")),
		Render: func(sh *core.Shared) string { return sh.Box(removeConfirmBody(sh, st, mode)) },
		OnKey: func(sh *core.Shared, k string) tea.Cmd {
			switch k {
			case "up", "k":
				if mode > removeProject {
					mode--
				}
			case "down", "j":
				if mode < removeProjectLocal {
					mode++
				}
			}
			return nil
		},
		OnYes: func(sh *core.Shared) tea.Cmd { return commitRemove(sh, st, mode) },
		Help:  removeConfirmHelp,
	}
}

func removeConfirmBody(sh *core.Shared, st addon.Status, mode int) string {
	path := st.Addon.Path
	if path == "" {
		path = "(none)"
	}
	return fmt.Sprintf("Remove %s\n\n  path:  %s\n\n%s", st.Addon.Name, path, removeOptions(mode))
}

// removeOptions renders the two removal modes stacked vertically, the active one
// marked and highlighted (vertical analog of the New Plugin target toggle).
func removeOptions(mode int) string {
	active := lipgloss.NewStyle().Foreground(core.FocusedColor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(core.MutedColor)
	opts := []struct{ label, desc string }{
		{"Project", "remove from the project manifest only"},
		{"Project + local files", "also delete the installed files"},
	}
	lines := make([]string, len(opts))
	for i, o := range opts {
		text := o.label + " — " + o.desc
		if i == mode {
			lines[i] = "  ▸ " + active.Render(text)
		} else {
			lines[i] = "    " + dim.Render(text)
		}
	}
	return strings.Join(lines, "\n")
}

// ---------- archive confirm ----------

// buildArchiveConfirm derives the package(s) to archive for the selected
// version-list item and returns a confirm screen. It returns ok=false (with an
// optional status line) when there is nothing to archive: HEAD, an error, or an
// already-archived selection.
func buildArchiveConfirm(selected addon.Addon, local string, sel list.Item) (*components.ConfirmScreen, string, bool) {
	repoID, err := source.RepoID(selected.URL)
	if err != nil {
		return nil, "cannot archive: " + err.Error(), false
	}

	var tag string
	var assets []source.Asset
	switch it := sel.(type) {
	case releaseItem:
		tag = it.rel.Tag
		assets = it.rel.Assets
	case versionItem:
		tag = it.tag
		assets = []source.Asset{it.asset}
	default:
		return nil, "", false // HEAD or anything without a concrete package
	}

	// Drop already-archived (local) assets; nothing to fetch for those.
	var remote []source.Asset
	for _, a := range assets {
		if !isArchived(a) {
			remote = append(remote, a)
		}
	}
	if len(remote) == 0 {
		return nil, tag + " already archived", false
	}

	cs := &components.ConfirmScreen{
		Crumb:  core.RenderTitleBar(core.HeaderTitle(selected.Name, local, "Archive "+tag)),
		Render: func(sh *core.Shared) string { return sh.Box(archiveConfirmBody(selected, tag, remote)) },
		OnYes:  func(sh *core.Shared) tea.Cmd { return core.Replace(newArchiveTask(selected, tag, repoID, remote)) },
		Help:   confirmHelp,
	}
	return cs, "", true
}

func archiveConfirmBody(selected addon.Addon, tag string, assets []source.Asset) string {
	root, _ := archive.Dir()
	lines := make([]string, len(assets))
	for i, a := range assets {
		lines[i] = "    • " + strings.TrimSuffix(a.Name, " - archived")
	}
	return fmt.Sprintf(
		"Archive %s\n\n  version:   %s\n  packages:\n%s\n\n  into:      %s",
		selected.Name, tag, strings.Join(lines, "\n"), root)
}
