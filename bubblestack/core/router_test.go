package core

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// stubScreen is a minimal Screen for exercising the router's stack/chrome plumbing
// without pulling in any domain screen.
type stubScreen struct{}

func (stubScreen) Init(*Shared) tea.Cmd { return nil }
func (stubScreen) Update(*Shared, tea.Msg) (Screen, Action) {
	return stubScreen{}, Action{}
}
func (stubScreen) View(*Shared) string       { return "stub" }
func (stubScreen) HelpView(*Shared) string   { return "" }
func (stubScreen) SetSize(*Shared, int, int) {}
func (stubScreen) Filtering() bool           { return false }

// filterScreen is a stubScreen that reports it is capturing filter text, so a key the
// router would otherwise consume globally must pass through to it instead.
type filterScreen struct{ stubScreen }

func (filterScreen) Filtering() bool { return true }

// fakeOutput is a minimal core.Output (plus the Log and Wrapper capabilities) for
// exercising the router's output key/layout plumbing without importing components
// (core ← components forbids it).
type fakeOutput struct {
	logs  []string
	shown bool
	wrap  bool
}

func (f *fakeOutput) Log(s string, show bool) { f.logs = append(f.logs, s); f.shown = show }
func (f *fakeOutput) Shown() bool             { return f.shown }
func (f *fakeOutput) Toggle()                 { f.shown = !f.shown }
func (f *fakeOutput) Hide()                   { f.shown = false }
func (f *fakeOutput) Clear()                  { f.logs = nil; f.shown = false }
func (f *fakeOutput) SetSize(_, _ int)        {}
func (f *fakeOutput) Height() int             { return 0 }
func (f *fakeOutput) View(bool) string        { return "OUT" }
func (f *fakeOutput) Update(tea.Msg) tea.Cmd  { return nil }
func (f *fakeOutput) GotoBottom()             {}
func (f *fakeOutput) ToggleWrap()             { f.wrap = !f.wrap }
func (f *fakeOutput) Wrapped() bool           { return f.wrap }

// plainOutput is an Output that does NOT implement Wrapper (no embedding — promoted
// methods would satisfy the interface): the Wrap key must pass through to the screen.
type plainOutput struct {
	shown bool
}

func (p *plainOutput) Log(_ string, show bool) { p.shown = show }
func (p *plainOutput) Shown() bool             { return p.shown }
func (p *plainOutput) Toggle()                 { p.shown = !p.shown }
func (p *plainOutput) Hide()                   { p.shown = false }
func (p *plainOutput) Clear()                  { p.shown = false }
func (p *plainOutput) SetSize(_, _ int)        {}
func (p *plainOutput) Height() int             { return 0 }
func (p *plainOutput) View(bool) string        { return "OUT" }
func (p *plainOutput) Update(tea.Msg) tea.Cmd  { return nil }
func (p *plainOutput) GotoBottom()             {}

// fakeStatus is a minimal core.Status for exercising the router's status rendering and
// auto-clear plumbing without importing components.
type fakeStatus struct {
	msg string
	gen int
}

func (f *fakeStatus) Set(line string) { f.msg = line; f.gen++ }
func (f *fakeStatus) Clear()          { f.msg = "" }
func (f *fakeStatus) Shown() bool     { return f.msg != "" }
func (f *fakeStatus) Height() int     { return 0 }
func (f *fakeStatus) View() string    { return f.msg }
func (f *fakeStatus) Gen() int        { return f.gen }

func newCoreTestRouter() Router {
	// nil App: stubScreen reads no context, so the router needs no domain dependency.
	// Chrome carries fake output/status panes (no header) to exercise the output keys
	// and the status rendering.
	sh := NewShared(nil)
	sh.Chrome = &Chrome{Output: &fakeOutput{}, Status: &fakeStatus{}}
	return NewRouter(sh, []TabEntry{{Title: "Stub", New: func(*Shared) Screen { return stubScreen{} }}})
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
	out := sh.Chrome.Output.(*fakeOutput)
	sh.Log("hello")
	tm = sized(tm) // re-lay-out with the log present

	tm = pump(tm, keyMsg(Keys.ToggleOutput.Keys()[0]))
	if !sh.Chrome.outputFocused {
		t.Fatal("ToggleOutput should focus the output pane")
	}

	tm = pump(tm, keyMsg(Keys.Clear.Keys()[0]))
	if len(out.logs) != 0 {
		t.Fatalf("Clear should clear the logs, got %d", len(out.logs))
	}
	if sh.Chrome.outputFocused {
		t.Fatal("clearing should return focus to the list")
	}
}

// TestOutputToggle checks the o key hides/shows the output box independently of the
// log contents (force-show), and hiding while focused returns focus to the list.
func TestOutputToggle(t *testing.T) {
	tm := sized(newCoreTestRouter())
	sh := tm.(Router).sh
	out := sh.Chrome.Output.(*fakeOutput)
	sh.Log("hello") // logging reveals the box
	if !out.Shown() {
		t.Fatal("appending a log should show the output box")
	}

	tm = pump(tm, keyMsg(Keys.ToggleOutput.Keys()[0])) // focus the pane
	tm = pump(tm, keyMsg(Keys.Output.Keys()[0]))       // o hides it
	if out.Shown() {
		t.Fatal("o should hide the output box")
	}
	if sh.Chrome.outputFocused {
		t.Fatal("hiding the output while focused should return focus to the list")
	}

	pump(tm, keyMsg(Keys.Output.Keys()[0])) // o shows it again
	if !out.Shown() {
		t.Fatal("o should show the output box again")
	}
}

// TestOutputWrapKey checks the w key flips the pane's render mode from any screen —
// the pane need not hold focus (it is a global chrome key like o) — and that it is a
// plain toggle.
func TestOutputWrapKey(t *testing.T) {
	tm := sized(newCoreTestRouter())
	sh := tm.(Router).sh
	out := sh.Chrome.Output.(*fakeOutput)
	sh.Log("hello")

	tm = pump(tm, keyMsg(Keys.Wrap.Keys()[0])) // unfocused: still consumed
	if !out.Wrapped() {
		t.Fatal("w should wrap the output pane without focusing it first")
	}
	tm = pump(tm, keyMsg(Keys.Wrap.Keys()[0]))
	if out.Wrapped() {
		t.Fatal("w should toggle wrap back off")
	}

	tm = pump(tm, keyMsg(Keys.ToggleOutput.Keys()[0])) // focus the pane
	pump(tm, keyMsg(Keys.Wrap.Keys()[0]))
	if !out.Wrapped() {
		t.Fatal("w should wrap while the pane holds focus too")
	}
}

// TestWrapKeyPassesThrough checks the two cases where w must NOT be swallowed: a top
// screen capturing filter text (it is typing a literal w), and an Output that doesn't
// implement Wrapper (nothing to toggle).
func TestWrapKeyPassesThrough(t *testing.T) {
	t.Run("filtering screen", func(t *testing.T) {
		sh := NewShared(nil)
		sh.Chrome = &Chrome{Output: &fakeOutput{}}
		r := NewRouter(sh, []TabEntry{{Title: "Filter", New: func(*Shared) Screen { return filterScreen{} }}})
		sh.Log("hello")

		pump(sized(r), keyMsg(Keys.Wrap.Keys()[0]))
		if sh.Chrome.Output.(*fakeOutput).Wrapped() {
			t.Fatal("w must reach a filtering screen as a literal key, not toggle wrap")
		}
	})

	t.Run("output without Wrapper", func(t *testing.T) {
		sh := NewShared(nil)
		sh.Chrome = &Chrome{Output: &plainOutput{}}
		r := NewRouter(sh, []TabEntry{{Title: "Stub", New: func(*Shared) Screen { return stubScreen{} }}})
		sh.Log("hello")

		// The key is simply not consumed; the run must not panic on the assertion.
		pump(sized(r), keyMsg(Keys.Wrap.Keys()[0]))
	})
}

// maskScreen is a stub screen that claims the whole canvas via ChromeMasker.
type maskScreen struct{ stubScreen }

func (maskScreen) ChromeMask() ChromeMask { return FullscreenMask() }

// TestChromeMaskSuppressesChrome checks a screen returning FullscreenMask hides the
// chrome the router would otherwise draw (here the output pane), and that popping
// back to an unmasked screen restores it — the per-screen suppression lever.
func TestChromeMaskSuppressesChrome(t *testing.T) {
	tm := sized(newCoreTestRouter())
	r := tm.(Router)
	r.sh.Log("hello") // reveal the output pane
	r.sh.Chrome.Status.Set("working")
	if r.belowChrome(r.currentMask()) == "" {
		t.Fatal("output/status should render under an unmasked screen")
	}

	tm = pump(tm, Push(maskScreen{}))
	r = tm.(Router)
	if got := r.belowChrome(r.currentMask()); got != "" {
		t.Fatalf("FullscreenMask should suppress the below chrome, got %q", got)
	}
	if got := r.topChrome(r.currentMask()); got != "" {
		t.Fatalf("FullscreenMask should suppress the top chrome, got %q", got)
	}

	tm = pump(tm, Pop())
	r = tm.(Router)
	if r.belowChrome(r.currentMask()) == "" {
		t.Fatal("popping back to the unmasked screen should restore the chrome")
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
