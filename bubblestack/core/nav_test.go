package core

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// initScreen counts Init calls, so push/replace can be checked to have fired Init.
type initScreen struct {
	stubScreen
	inits int
}

func (s *initScreen) Init(*Shared) tea.Cmd { s.inits++; return nil }

// popStopScreen opts into PopStopper, marking a PopTo boundary (a command hub).
type popStopScreen struct{ stubScreen }

func (popStopScreen) PopStop() bool { return true }

// recvScreen records a broadcast payload and optionally returns a follow-up Action,
// for exercising PropagateAll's broadcast + same-tick cascade.
type recvScreen struct {
	stubScreen
	got    any
	gotCnt int
	follow Action
}

func (s *recvScreen) Receive(_ *Shared, payload any) Action {
	s.got = payload
	s.gotCnt++
	return s.follow
}

// twoTabRouter builds a router with two named tabs whose roots are the given screens.
func twoTabRouter(t0, t1 Screen) Router {
	sh := NewShared(nil)
	return NewRouter(sh, []TabEntry{
		{Title: "Tab0", New: func(*Shared) Screen { return t0 }},
		{Title: "Tab1", New: func(*Shared) Screen { return t1 }},
	})
}

func stackLen(tm tea.Model) int { return len(tm.(Router).stack) }

func TestNavReplace(t *testing.T) {
	tm := sized(newCoreTestRouter())
	repl := &initScreen{}

	tm = pump(tm, Replace(repl))
	if got := stackLen(tm); got != 1 {
		t.Fatalf("replace at root should keep depth 1, got %d", got)
	}
	if tm.(Router).Top() != repl {
		t.Fatal("replace should swap the top screen")
	}
	if repl.inits != 1 {
		t.Fatalf("replacement Init should fire once, got %d", repl.inits)
	}

	// Replace deeper: push then replace keeps depth, swaps top.
	tm = pump(tm, Push(stubScreen{}))
	repl2 := &initScreen{}
	tm = pump(tm, Replace(repl2))
	if got := stackLen(tm); got != 2 {
		t.Fatalf("replace should keep depth 2, got %d", got)
	}
	if tm.(Router).Top() != repl2 || repl2.inits != 1 {
		t.Fatal("replace should swap top and fire its Init")
	}
}

func TestNavPopTo(t *testing.T) {
	// With a PopStopper mid-stack, PopTo unwinds to it.
	tm := sized(newCoreTestRouter())
	tm = pump(tm, Push(popStopScreen{}))
	tm = pump(tm, Push(stubScreen{}))
	tm = pump(tm, Push(stubScreen{}))
	if got := stackLen(tm); got != 4 {
		t.Fatalf("setup want depth 4, got %d", got)
	}
	tm = pump(tm, PopTo())
	if got := stackLen(tm); got != 2 {
		t.Fatalf("PopTo should stop at the PopStopper (depth 2), got %d", got)
	}
	if _, ok := tm.(Router).Top().(popStopScreen); !ok {
		t.Fatal("PopTo should leave the PopStopper on top")
	}

	// With no PopStopper, PopTo unwinds to the root.
	tm = sized(newCoreTestRouter())
	tm = pump(tm, Push(stubScreen{}))
	tm = pump(tm, Push(stubScreen{}))
	tm = pump(tm, PopTo())
	if got := stackLen(tm); got != 1 {
		t.Fatalf("PopTo with no stopper should reach the root, got %d", got)
	}
}

func TestNavResetToRoot(t *testing.T) {
	tm := sized(newCoreTestRouter())
	tm = pump(tm, Push(stubScreen{}))
	tm = pump(tm, Push(stubScreen{}))
	tm = pump(tm, ResetToRoot())
	if got := stackLen(tm); got != 1 {
		t.Fatalf("ResetToRoot should collapse to the root, got %d", got)
	}
}

func TestNavSeq(t *testing.T) {
	// Two pushes in one Seq, applied in order this tick.
	tm := sized(newCoreTestRouter())
	a, b := stubScreen{}, &initScreen{}
	tm = pump(tm, Seq(Push(a), Push(b)))
	if got := stackLen(tm); got != 3 {
		t.Fatalf("Seq(Push,Push) should grow depth to 3, got %d", got)
	}
	if tm.(Router).Top() != b {
		t.Fatal("Seq should apply its actions in order (b ends on top)")
	}

	// A zero (empty) Action in the sequence is skipped.
	tm = sized(newCoreTestRouter())
	tm = pump(tm, Seq(Push(stubScreen{}), Action{}, Push(stubScreen{})))
	if got := stackLen(tm); got != 3 {
		t.Fatalf("empty Actions should be skipped, want depth 3, got %d", got)
	}
}

func TestNavShowTab(t *testing.T) {
	tm := sized(twoTabRouter(stubScreen{}, stubScreen{}))
	// Push on tab0 so it has a non-trivial stack to unwind.
	tm = pump(tm, Push(stubScreen{}))
	if tm.(Router).active != 0 || stackLen(tm) != 2 {
		t.Fatalf("setup: active=%d depth=%d", tm.(Router).active, stackLen(tm))
	}

	tm = pump(tm, ShowTab("Tab1"))
	r := tm.(Router)
	if r.active != 1 {
		t.Fatalf("ShowTab should switch active to 1, got %d", r.active)
	}
	if len(r.stack) != 1 || r.stack[0] != r.roots[1] {
		t.Fatal("ShowTab should unwind to the target tab's root")
	}

	// An unknown title is a no-op.
	tm = pump(tm, ShowTab("Nope"))
	if tm.(Router).active != 1 {
		t.Fatal("ShowTab with an unknown title should be a no-op")
	}
}

func TestNavPropagateAll(t *testing.T) {
	root := &recvScreen{}
	tm := sized(twoTabRouter(root, stubScreen{}))

	tm = pump(tm, PropagateAll("ping"))
	if root.gotCnt != 1 || root.got != "ping" {
		t.Fatalf("root should receive the payload once, got cnt=%d val=%v", root.gotCnt, root.got)
	}
}

func TestNavPropagateFollowUp(t *testing.T) {
	// A receiver that returns ShowTab must have its follow-up applied this same tick.
	root := &recvScreen{follow: ShowTab("Tab1")}
	tm := sized(twoTabRouter(root, stubScreen{}))

	tm = pump(tm, PropagateAll("focus"))
	if got := tm.(Router).active; got != 1 {
		t.Fatalf("receiver's ShowTab follow-up should switch tabs, active=%d", got)
	}
}

func TestNavRefreshRoots(t *testing.T) {
	builds := 0
	sh := NewShared(nil)
	r := NewRouter(sh, []TabEntry{
		{Title: "Tab0", New: func(*Shared) Screen { builds++; return stubScreen{} }},
	})
	if builds != 1 {
		t.Fatalf("NewRouter should build each root once, got %d", builds)
	}
	tm := sized(r)
	tm = pump(tm, RefreshRoots())
	if builds != 2 {
		t.Fatalf("RefreshRoots should rebuild each root, builds=%d", builds)
	}
	if tm.(Router).stack[0] != tm.(Router).roots[0] {
		t.Fatal("RefreshRoots should re-point stack[0] at the rebuilt root")
	}
}

func TestAsyncAction(t *testing.T) {
	cmd := func() tea.Msg { return nil }
	a := Async(cmd)
	if a.Msg != nil {
		t.Errorf("Async should carry no control message, got %v", a.Msg)
	}
	if a.Cmd == nil {
		t.Error("Async should carry the cmd in the async lane")
	}
}
