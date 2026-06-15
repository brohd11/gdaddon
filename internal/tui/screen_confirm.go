package tui

import (
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/archive"
	"gdaddon/internal/source"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// confirmScreen is the shared y/n confirm/summary box, reused by the install
// confirm, the archive confirm, and the New Plugin confirm. It snapshots its
// breadcrumb and renders its body via a closure, so the screens below keep no
// pending state. onYes runs on confirm; onToggle (when set) handles ←/→.
type confirmScreen struct {
	crumb    string
	render   func(*shared) string
	onYes    func(*shared) tea.Cmd
	onToggle func() // nil unless the screen has a Project/Global toggle
	help     []key.Binding
}

func (s *confirmScreen) Init(*shared) tea.Cmd { return nil }

func (s *confirmScreen) Update(sh *shared, msg tea.Msg) (screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch key.String() {
	case "left", "right", "h", "l":
		if s.onToggle != nil {
			s.onToggle()
		}
		return s, nil
	case "y", "Y", "enter":
		return s, s.onYes(sh)
	case "n", "N", "esc":
		return s, pop()
	}
	return s, nil
}

func (s *confirmScreen) View(sh *shared) string {
	return lipgloss.JoinVertical(lipgloss.Left, s.crumb, s.render(sh))
}

func (s *confirmScreen) HelpView(sh *shared) string { return sh.bindingHelp(s.help) }
func (s *confirmScreen) SetSize(*shared, int, int)  {}

var confirmHelp = []key.Binding{
	key.NewBinding(key.WithKeys("y", "enter"), key.WithHelp("y/enter", "confirm")),
	key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n/esc", "cancel")),
}

var newPluginConfirmHelp = []key.Binding{
	key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "target")),
	key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "add")),
	key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
}

// ---------- install confirm ----------

func newInstallConfirm(selected addon.Addon, local string, pick versionItem) *confirmScreen {
	return &confirmScreen{
		crumb:  renderTitleBar(headerTitle(selected.Name, local, pickSection(pick))),
		render: func(sh *shared) string { return sh.box(confirmInstallBody(sh, selected, pick)) },
		onYes:  func(sh *shared) tea.Cmd { return replace(newInstallTask(selected, local, pick)) },
		help:   confirmHelp,
	}
}

func confirmInstallBody(sh *shared, selected addon.Addon, pick versionItem) string {
	// Hard-wrap the (space-less) URL to fit inside the box.
	urlBlock := indentLines(hardWrap(pick.asset.URL, sh.confirmWidth()-4), "    ")
	return fmt.Sprintf(
		"Install %s\n\n  version:  %s\n  asset:    %s\n  path:     %s\n  url:\n%s",
		selected.Name, pick.tag, pick.asset.Name, selected.Path, urlBlock)
}

// ---------- archive confirm ----------

// archiveKeyHandler returns a picker onKey that archives the selected item on
// 'a' — the shared version/asset/branch archive action. The key is always
// consumed (handled=true); buildArchiveConfirm decides whether there's anything
// to push.
func archiveKeyHandler(selected addon.Addon, local string) func(*shared, string, list.Item) (tea.Cmd, bool) {
	return func(sh *shared, k string, it list.Item) (tea.Cmd, bool) {
		if k != "a" {
			return nil, false
		}
		cs, status, ok := buildArchiveConfirm(selected, local, it)
		if status != "" {
			sh.statusMsg = status
		}
		if !ok {
			return nil, true
		}
		return push(cs), true
	}
}

// buildArchiveConfirm derives the package(s) to archive for the selected
// version-list item and returns a confirm screen. It returns ok=false (with an
// optional status line) when there is nothing to archive: HEAD, an error, or an
// already-archived selection.
func buildArchiveConfirm(selected addon.Addon, local string, sel list.Item) (*confirmScreen, string, bool) {
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

	cs := &confirmScreen{
		crumb:  renderTitleBar(headerTitle(selected.Name, local, "Archive "+tag)),
		render: func(sh *shared) string { return sh.box(archiveConfirmBody(selected, tag, remote)) },
		onYes:  func(sh *shared) tea.Cmd { return replace(newArchiveTask(selected, tag, repoID, remote)) },
		help:   confirmHelp,
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

// ---------- new plugin confirm ----------

func newNewPluginConfirm(name, url, path string, addTarget int) *confirmScreen {
	target := addTarget // local copy the toggle mutates
	return &confirmScreen{
		crumb:    renderTitleBar("New Plugin"),
		render:   func(sh *shared) string { return sh.box(newPluginConfirmBody(sh, name, url, path, target)) },
		onToggle: func() { target = otherTarget(target) },
		onYes:    func(sh *shared) tea.Cmd { return commitNewPlugin(sh, name, url, path, target) },
		help:     newPluginConfirmHelp,
	}
}

func newPluginConfirmBody(sh *shared, name, url, path string, addTarget int) string {
	urlBlock := indentLines(hardWrap(url, sh.confirmWidth()-4), "    ")
	if path == "" {
		path = "(derived on install)"
	}
	return fmt.Sprintf(
		"Add plugin\n\n  name:     %s\n  url:\n%s\n  path:     %s\n\n  add to:   %s",
		name, urlBlock, path, targetToggle(addTarget))
}

// commitNewPlugin writes the pending entry to the project manifest or the global
// list, then unwinds to browse (rebuilding the list for a project add).
func commitNewPlugin(sh *shared, name, url, path string, addTarget int) tea.Cmd {
	if addTarget == targetGlobal {
		globalPath, err := addon.GlobalListPath()
		if err == nil {
			err = addon.AddEntry(globalPath, name, url, path)
		}
		if err != nil {
			sh.statusMsg = "error: " + err.Error()
		} else {
			sh.statusMsg = fmt.Sprintf("added %s to global list", name)
		}
		return resetToRoot()
	}

	if err := addon.AddEntry(sh.manifestPath, name, url, path); err != nil {
		sh.statusMsg = "error: " + err.Error()
		return resetToRoot()
	}
	return tea.Batch(resetToRoot(), reloadCmd(sh, "added "+name))
}

// targetToggle renders the Project ◄ ► Global switch with the active side
// highlighted.
func targetToggle(addTarget int) string {
	active := lipgloss.NewStyle().Foreground(focusedColor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(mutedColor)
	project, global := dim.Render("Project"), dim.Render("Global")
	if addTarget == targetProject {
		project = active.Render("Project")
	} else {
		global = active.Render("Global")
	}
	return fmt.Sprintf("%s  ◄ ►  %s", project, global)
}

func otherTarget(t int) int {
	if t == targetProject {
		return targetGlobal
	}
	return targetProject
}
