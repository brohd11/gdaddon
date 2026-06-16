package actions

import (
	"fmt"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// add targets for the New Plugin target toggle.
const (
	targetProject = iota
	targetGlobal
)

// rows of the New Plugin form (url/name/path text fields + the target toggle).
// URL is first because it's the only mandatory field.
const (
	fldURL = iota
	fldName
	fldPath
	fldTarget
	fldCount
)

var newPluginConfirmHelp = []key.Binding{
	key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "target")),
	key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "add")),
	key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
}

// ---------- new plugin confirm ----------

func newNewPluginConfirm(name, url, path string, addTarget int) *components.ConfirmScreen {
	target := addTarget // local copy the toggle mutates
	return &components.ConfirmScreen{
		Crumb:  core.RenderTitleBar("New Plugin"),
		Render: func(sh *core.Shared) string { return sh.Box(newPluginConfirmBody(sh, name, url, path, target)) },
		OnKey: func(sh *core.Shared, k string) tea.Cmd {
			switch k {
			case "left", "right", "h", "l":
				target = otherTarget(target)
			}
			return nil
		},
		OnYes: func(sh *core.Shared) tea.Cmd { return commitNewPlugin(sh, name, url, path, target) },
		Help:  newPluginConfirmHelp,
	}
}

func newPluginConfirmBody(sh *core.Shared, name, url, path string, addTarget int) string {
	urlBlock := core.IndentLines(core.HardWrap(url, sh.ConfirmWidth()-4), "    ")
	if path == "" {
		path = "(derived on install)"
	}
	return fmt.Sprintf(
		"Add plugin\n\n  name:     %s\n  url:\n%s\n  path:     %s\n\n  add to:   %s",
		name, urlBlock, path, targetToggle(addTarget))
}

// commitNewPlugin writes the pending entry to the project manifest or the global
// list, then unwinds to the root (rebuilding the Browse list for a project add).
func commitNewPlugin(sh *core.Shared, name, url, path string, addTarget int) tea.Cmd {
	if addTarget == targetGlobal {
		globalPath, err := addon.GlobalListPath()
		if err == nil {
			err = addon.AddEntry(globalPath, name, url, path)
		}
		if err != nil {
			sh.StatusMsg = "error: " + err.Error()
			return core.ResetToRoot()
		}
		// Show the Global tab rebuilt with the new entry (parallel to a project add
		// switching to Browse).
		return core.GlobalRefresh(fmt.Sprintf("added %s to global list", name))
	}

	if err := addon.AddEntry(sh.ManifestPath, name, url, path); err != nil {
		sh.StatusMsg = "error: " + err.Error()
		return core.ResetToRoot()
	}
	return tea.Batch(core.ResetToRoot(), reloadCmd(sh, "added "+name))
}

// targetToggle renders the Project ◄ ► Global switch with the active side
// highlighted.
func targetToggle(addTarget int) string {
	active := lipgloss.NewStyle().Foreground(core.FocusedColor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(core.MutedColor)
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
