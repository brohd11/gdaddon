package core

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// stubScreen is a minimal Screen for exercising the router's stack/chrome plumbing
// without pulling in any domain screen.
type stubScreen struct{}

func (stubScreen) Init(*Shared) tea.Cmd                      { return nil }
func (stubScreen) Update(*Shared, tea.Msg) (Screen, tea.Cmd) { return stubScreen{}, nil }
func (stubScreen) View(*Shared) string                       { return "stub" }
func (stubScreen) HelpView(*Shared) string                   { return "" }
func (stubScreen) SetSize(*Shared, int, int)                 {}
func (stubScreen) Filtering() bool                           { return false }

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

	tm, _ = tm.Update(popMsg{n: 1})
	if got := len(tm.(Router).stack); got != 1 {
		t.Fatalf("after pop want 1, got %d", got)
	}

	tm, _ = tm.Update(popMsg{n: 1})
	if got := len(tm.(Router).stack); got != 1 {
		t.Fatalf("root pop should be ignored, want 1, got %d", got)
	}
}

// TestOutputFocusAndClear seeds a log line (which reveals the output box), then
// checks the ToggleOutput key focuses the output pane and the Clear key clears it
// (returning focus to the list) — the router's global keys. The keys are taken from
// the central keymap so the test tracks rebinds.
func TestOutputFocusAndClear(t *testing.T) {
	tm := sized(newCoreTestRouter())
	sh := tm.(Router).sh
	sh.AppendLog("hello")
	tm = sized(tm) // re-lay-out with the log present

	tm = pump(tm, keyMsg(Keys.ToggleOutput.Keys()[0]))
	if sh.focus != focusOutput {
		t.Fatalf("ToggleOutput should focus the output pane, got %v", sh.focus)
	}

	tm = pump(tm, keyMsg(Keys.Clear.Keys()[0]))
	if len(sh.Logs) != 0 {
		t.Fatalf("Clear should clear the logs, got %d", len(sh.Logs))
	}
	if sh.focus != focusList {
		t.Fatal("clearing should return focus to the list")
	}
}

// TestOutputToggle checks the o key hides/shows the output box independently of the
// log contents (force-show), and hiding while focused returns focus to the list.
func TestOutputToggle(t *testing.T) {
	tm := sized(newCoreTestRouter())
	sh := tm.(Router).sh
	sh.AppendLog("hello") // AppendLog reveals the box
	if !sh.OutputShown {
		t.Fatal("appending a log should show the output box")
	}

	tm = pump(tm, keyMsg(Keys.ToggleOutput.Keys()[0])) // focus the pane
	tm = pump(tm, keyMsg(Keys.Output.Keys()[0]))       // o hides it
	if sh.OutputShown {
		t.Fatal("o should hide the output box")
	}
	if sh.focus != focusList {
		t.Fatal("hiding the output while focused should return focus to the list")
	}

	pump(tm, keyMsg(Keys.Output.Keys()[0])) // o shows it again
	if !sh.OutputShown {
		t.Fatal("o should show the output box again")
	}
}

// keyMsg builds a tea.KeyMsg whose String() matches the given key string, so tests
// can drive the router from central-keymap key strings.
func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}
