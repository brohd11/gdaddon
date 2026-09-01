package tui

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gdaddon/internal/tui/flows/docs"
	"github.com/brohd11/gdaddon/internal/tui/flows/newplugin"
	"github.com/brohd11/gdaddon/internal/tui/tabs/actions"
	"github.com/brohd11/gdaddon/internal/tui/tabs/project"

	tea "charm.land/bubbletea/v2"
)

// newTestRouter builds a router with the Browse + Actions tabs and no real project
// on disk (the manifest path doesn't exist → an empty browse list).
func newTestRouter() core.Router {
	sh := core.NewShared(appctx.New("/tmp/gdaddon-test", "dev"))
	sh.Chrome = &core.Chrome{Header: core.NewHeaderPane(appctx.Header), Output: components.NewLogPane()}
	return core.NewRouter(sh, []core.TabEntry{
		{Title: "Browse", New: func(sh *core.Shared) core.Screen { return project.NewProjectScreen(sh) }},
		{Title: "Actions", New: func(sh *core.Shared) core.Screen { return actions.NewActionsScreen(sh) }},
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
	out := view(tm)
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
	tm = pump(tm, keyMsg("]"))
	if _, ok := tm.(core.Router).Top().(*actions.ActionsScreen); !ok {
		t.Fatalf("after ] want *actions.ActionsScreen, got %T", tm.(core.Router).Top())
	}
	_ = view(tm)
	tm = pump(tm, keyMsg("["))
	if _, ok := tm.(core.Router).Top().(*project.ProjectScreen); !ok {
		t.Fatalf("after [ want *project.ProjectScreen, got %T", tm.(core.Router).Top())
	}
}

// TestTabSwitchGatedAtDepth confirms [ / ] only switch tabs at the root: after
// drilling into a sub-screen, the tab key is ignored.
func TestTabSwitchGatedAtDepth(t *testing.T) {
	tm := sized(newTestRouter())
	tm, _ = tm.Update(core.Push(newplugin.NewNewPluginForm())) // depth 2 on the Browse tab
	tm = pump(tm, keyMsg("]"))
	if _, ok := tm.(core.Router).Top().(*components.FormScreen); !ok {
		t.Fatalf("] at depth 2 should be ignored, got %T", tm.(core.Router).Top())
	}
}

// TestFirstRunDocsFlow walks the onboarding path a first-run user takes: the welcome
// popup the startup hook shows, enter into the docs index, enter into a page, then esc
// back out. The popup Replaces itself with the index, so backing out of the index lands
// on the tab root rather than re-showing the popup.
func TestFirstRunDocsFlow(t *testing.T) {
	tm := sized(newTestRouter())
	root := tm.(core.Router).Top()

	tm = pump(tm, docs.WelcomeCmd()()) // what bubblestack.Config.Init dispatches
	if d, ok := tm.(core.Router).Top().(*components.DialogScreen); !ok || !d.IsOverlay() {
		t.Fatalf("want the welcome popup on top, got %T", tm.(core.Router).Top())
	}

	tm = pump(tm, keyMsg("enter"))
	if _, ok := tm.(core.Router).Top().(*components.PickerScreen); !ok {
		t.Fatalf("enter on the popup should open the docs index, got %T", tm.(core.Router).Top())
	}

	tm = pump(tm, keyMsg("enter"))
	if _, ok := tm.(core.Router).Top().(*components.DocScreen); !ok {
		t.Fatalf("enter on a docs row should open the page, got %T", tm.(core.Router).Top())
	}
	if out := view(tm); !strings.Contains(out, "Docs › Getting started") {
		t.Errorf("breadcrumb should name the open page:\n%s", out)
	}

	tm = pump(tm, keyMsg("esc"))
	if _, ok := tm.(core.Router).Top().(*components.PickerScreen); !ok {
		t.Fatalf("esc on a page should return to the index, got %T", tm.(core.Router).Top())
	}

	tm = pump(tm, keyMsg("esc"))
	if tm.(core.Router).Top() != root {
		t.Fatalf("esc on the index should return to the tab root, got %T", tm.(core.Router).Top())
	}
}

// view renders the model to the plain text the assertions match against. v2's View
// returns a tea.View — the frame's content plus the terminal modes it asks for — so this
// reaches through to the content, and strips it: lipgloss v2 renders styles verbatim
// where v1's TTY-less Ascii profile dropped them, so a substring like "Docs › Getting
// started" now has escape sequences between its words.
func view(tm tea.Model) string { return ansi.Strip(tm.View().Content) }
