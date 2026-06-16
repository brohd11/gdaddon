package core

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// stubScreen is a minimal Screen for exercising the router's stack/chrome plumbing
// without pulling in any domain screen. It opts into the output pane.
type stubScreen struct{}

func (stubScreen) Init(*Shared) tea.Cmd                      { return nil }
func (stubScreen) Update(*Shared, tea.Msg) (Screen, tea.Cmd) { return stubScreen{}, nil }
func (stubScreen) View(*Shared) string                       { return "stub" }
func (stubScreen) HelpView(*Shared) string                   { return "" }
func (stubScreen) SetSize(*Shared, int, int)                 {}
func (stubScreen) Filtering() bool                           { return false }
func (stubScreen) WantsOutput() bool                         { return true }

func newCoreTestRouter() Router {
	sh := NewShared("/tmp/gdaddon-test/addon_manifest.yml", "/tmp/gdaddon-test")
	return NewRouter(sh, []TabEntry{{Title: "Stub", Root: stubScreen{}}})
}

func sized(tm tea.Model) tea.Model {
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return tm
}

// pump delivers msg, then runs the returned command and feeds its (single,
// non-batch) result back — enough to drive the navigation commands.
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

// TestRouterStackPushPop checks the stack semantics: push grows it, pop shrinks it,
// and popping at the root (single screen) is ignored.
func TestRouterStackPushPop(t *testing.T) {
	tm := sized(newCoreTestRouter())

	tm, _ = tm.Update(pushMsg{s: stubScreen{}})
	if got := len(tm.(Router).stack); got != 2 {
		t.Fatalf("after push want 2, got %d", got)
	}

	tm, _ = tm.Update(popMsg{})
	if got := len(tm.(Router).stack); got != 1 {
		t.Fatalf("after pop want 1, got %d", got)
	}

	tm, _ = tm.Update(popMsg{})
	if got := len(tm.(Router).stack); got != 1 {
		t.Fatalf("root pop should be ignored, want 1, got %d", got)
	}
}

// TestOutputFocusAndClear seeds a log line, then checks tab focuses the output pane
// and c clears it (returning focus to the list) — the router's global keys.
func TestOutputFocusAndClear(t *testing.T) {
	tm := sized(newCoreTestRouter())
	sh := tm.(Router).sh
	sh.AppendLog("hello")
	tm = sized(tm) // re-lay-out with the log present

	tm = pump(tm, tea.KeyMsg{Type: tea.KeyTab})
	if sh.focus != focusOutput {
		t.Fatalf("tab should focus the output pane, got %v", sh.focus)
	}

	tm = pump(tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if len(sh.Logs) != 0 {
		t.Fatalf("c should clear the logs, got %d", len(sh.Logs))
	}
	if sh.focus != focusList {
		t.Fatal("clearing should return focus to the list")
	}
}
