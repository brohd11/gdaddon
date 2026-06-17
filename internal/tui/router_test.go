package tui

import (
	"strings"
	"testing"

	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/flows/newplugin"
	"gdaddon/internal/tui/tabs/actions"
	"gdaddon/internal/tui/tabs/project"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// newTestRouter builds a router with the Browse + Actions tabs and no real project
// on disk (statuses nil → an empty browse list).
func newTestRouter() core.Router {
	sh := core.NewShared(appctx.New("/tmp/gdaddon-test/addon_manifest.yml", "/tmp/gdaddon-test"))
	sh.Header = appctx.Header
	return core.NewRouter(sh, []core.TabEntry{
		{Title: "Browse", Root: project.NewProjectScreen(nil)},
		{Title: "Actions", Root: actions.NewActionsScreen()},
	})
}

func sized(tm tea.Model) tea.Model {
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return tm
}

// pump delivers msg, then runs the returned command and feeds its (single,
// non-batch) result back — enough to drive the navigation commands (push/pop).
func pump(tm tea.Model, msg tea.Msg) tea.Model {
	tm, cmd := tm.Update(msg)
	for i := 0; i < 8 && cmd != nil; i++ {
		out := cmd()
		if out == nil {
			break
		}
		if _, isBatch := out.(tea.BatchMsg); isBatch {
			break
		}
		tm, cmd = tm.Update(out)
	}
	return tm
}

// TestRouterRenders confirms the router renders the framed view (header + body +
// help) without panicking and includes the persistent header.
func TestRouterRenders(t *testing.T) {
	tm := sized(newTestRouter())
	out := tm.View()
	if out == "" {
		t.Fatal("empty view")
	}
	if !strings.Contains(out, "Project:") {
		t.Fatalf("header missing from view:\n%s", out)
	}
}

// TestTabSwitch walks Browse → Actions (]) → Browse ([), exercising top-level tab
// switching through the router's global keys.
func TestTabSwitch(t *testing.T) {
	tm := sized(newTestRouter())
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if _, ok := tm.(core.Router).Top().(*actions.ActionsScreen); !ok {
		t.Fatalf("after ] want *actions.ActionsScreen, got %T", tm.(core.Router).Top())
	}
	_ = tm.View()
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	if _, ok := tm.(core.Router).Top().(*project.ProjectScreen); !ok {
		t.Fatalf("after [ want *project.ProjectScreen, got %T", tm.(core.Router).Top())
	}
}

// TestTabSwitchGatedAtDepth confirms [ / ] only switch tabs at the root: after
// drilling into a sub-screen, the tab key is ignored.
func TestTabSwitchGatedAtDepth(t *testing.T) {
	tm := sized(newTestRouter())
	tm, _ = tm.Update(core.Push(newplugin.NewNewPluginForm())()) // depth 2 on the Browse tab
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if _, ok := tm.(core.Router).Top().(*components.FormScreen); !ok {
		t.Fatalf("] at depth 2 should be ignored, got %T", tm.(core.Router).Top())
	}
}
