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

// ---------- archive confirm ----------

// archiveKeyHandler returns a picker onKey that archives the selected item on
// 'a' — the shared version/asset/branch archive action. The key is always
// consumed (handled=true); buildArchiveConfirm decides whether there's anything
// to push.
func archiveKeyHandler(selected addon.Addon, local string) func(*core.Shared, string, list.Item) (tea.Cmd, bool) {
	return func(sh *core.Shared, k string, it list.Item) (tea.Cmd, bool) {
		if k != "a" {
			return nil, false
		}
		cs, status, ok := buildArchiveConfirm(selected, local, it)
		if status != "" {
			sh.StatusMsg = status
		}
		if !ok {
			return nil, true
		}
		return core.Push(cs), true
	}
}

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
