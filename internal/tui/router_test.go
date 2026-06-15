package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newTestRouter builds a router around the browse root with no real project on
// disk (statuses nil → just the pinned Actions row).
func newTestRouter() router {
	sh := newShared("/tmp/gdaddon-test/addon_manifest.yml", "/tmp/gdaddon-test")
	return newRouter(sh, newBrowseScreen(nil))
}

func sized(tm tea.Model) tea.Model {
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return tm
}

// pump delivers msg, then runs the returned command and feeds its (single,
// non-batch) result back — enough to drive the navigation commands (push/pop)
// that bubbletea would otherwise round-trip through its event loop.
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

// TestRouterEnterAndBack walks browse → Actions (enter on the pinned menu row) →
// back (esc), exercising push/pop through the router.
func TestRouterEnterAndBack(t *testing.T) {
	tm := sized(newTestRouter())
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tm.(router).top().(*actionsScreen); !ok {
		t.Fatalf("after enter want *actionsScreen, got %T", tm.(router).top())
	}
	_ = tm.View()
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEsc})
	if _, ok := tm.(router).top().(*browseScreen); !ok {
		t.Fatalf("after esc want *browseScreen, got %T", tm.(router).top())
	}
}

// TestRouterStackPushPop checks the stack semantics: push grows it, pop shrinks
// it, and popping at the root (single screen) is ignored.
func TestRouterStackPushPop(t *testing.T) {
	tm := sized(newTestRouter())

	tm, _ = tm.Update(pushMsg{s: newActionsScreen()})
	if got := len(tm.(router).stack); got != 2 {
		t.Fatalf("after push want 2, got %d", got)
	}

	tm, _ = tm.Update(popMsg{})
	if got := len(tm.(router).stack); got != 1 {
		t.Fatalf("after pop want 1, got %d", got)
	}

	tm, _ = tm.Update(popMsg{})
	if got := len(tm.(router).stack); got != 1 {
		t.Fatalf("root pop should be ignored, want 1, got %d", got)
	}
}

// TestOutputFocusAndClear seeds a log line, then checks tab focuses the output
// pane and c clears it (returning focus to the list) — the router's global keys.
func TestOutputFocusAndClear(t *testing.T) {
	tm := sized(newTestRouter())
	sh := tm.(router).sh
	sh.appendLog("hello")
	tm = sized(tm) // re-lay-out with the log present

	tm = pump(tm, tea.KeyMsg{Type: tea.KeyTab})
	if sh.focus != focusOutput {
		t.Fatalf("tab should focus the output pane, got %v", sh.focus)
	}

	tm = pump(tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if len(sh.logs) != 0 {
		t.Fatalf("c should clear the logs, got %d", len(sh.logs))
	}
	if sh.focus != focusList {
		t.Fatal("clearing should return focus to the list")
	}
}

// TestNewPluginFormToConfirm checks the form validates the URL (empty stays put)
// and a filled URL pushes the confirm screen.
func TestNewPluginFormToConfirm(t *testing.T) {
	tm := sized(newTestRouter())
	tm, _ = tm.Update(pushMsg{s: newNewPluginForm()})
	form, ok := tm.(router).top().(*newPluginForm)
	if !ok {
		t.Fatalf("want *newPluginForm, got %T", tm.(router).top())
	}

	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tm.(router).top().(*newPluginForm); !ok {
		t.Fatalf("empty URL should keep the form, got %T", tm.(router).top())
	}

	form.inputs[fldURL].SetValue("https://github.com/owner/repo")
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tm.(router).top().(*confirmScreen); !ok {
		t.Fatalf("filled URL should push confirm, got %T", tm.(router).top())
	}
	if !strings.Contains(tm.View(), "owner/repo") {
		t.Fatal("confirm view should show the entered url")
	}
}
