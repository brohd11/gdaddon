package components

import (
	"context"
	"fmt"

	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// RunFunc executes a streaming background task: report() pipes progress lines into
// the log, and the terminating core.TaskEvent (Done:true) is sent on done. ctx is
// cancelled when the user aborts the task (esc), so a cancellable RunFunc should
// thread it into its network/process work and return promptly once it fires.
type RunFunc func(ctx context.Context, sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent)

// TaskScreen runs a streaming background task and shows its log. It is context-
// agnostic: the calling tab supplies run (the work), onDone (what to do with the
// terminating event), and — for tasks that stay on the log until dismissed — a
// doneLabel + onDismiss. It names no domain type.
//
// While the task runs, esc requests an abort: the run's context is cancelled and the
// screen waits for the run to unwind, then stays on the log showing "aborted" until
// the user dismisses it (rather than running onDone's success navigation).
type TaskScreen struct {
	label, doneLabel string
	Crumb            string
	CrumbShort       string // optional short breadcrumb segment; defaults to label
	stay             bool
	run              RunFunc
	onDone           func(*core.Shared, core.TaskEvent) core.Action
	onDismiss        func(*core.Shared) core.Action
	done             bool
	cancel           context.CancelFunc
	aborting         bool
}

var _ core.Crumber = (*TaskScreen)(nil)

// CrumbLabel contributes the task's label as its breadcrumb segment.
func (s *TaskScreen) CrumbLabel(short bool) string {
	return crumbSeg(short, s.CrumbShort, "Task", "Task")
}

// NewTask builds a task that navigates away as soon as it finishes (install,
// install-all): onDone returns the navigation Action for the terminating event.
func NewTask(label string, run RunFunc, onDone func(*core.Shared, core.TaskEvent) core.Action) *TaskScreen {
	return &TaskScreen{label: label, run: run, onDone: onDone}
}

// NewStayTask builds a task that stays on the log after finishing (archive) until
// the user dismisses it: onDone records the result, onDismiss runs on esc/enter.
func NewStayTask(label, doneLabel string, run RunFunc,
	onDone func(*core.Shared, core.TaskEvent) core.Action, onDismiss func(*core.Shared) core.Action) *TaskScreen {
	return &TaskScreen{label: label, doneLabel: doneLabel, stay: true, run: run, onDone: onDone, onDismiss: onDismiss}
}

func (s *TaskScreen) Init(sh *core.Shared) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	return startTask(ctx, sh, s.run)
}

func (s *TaskScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	switch msg := msg.(type) {
	case core.TaskEvent:
		if !msg.Done {
			sh.Log(msg.Line)
			return s, core.Async(waitForEvent(sh.Events))
		}
		s.done = true
		if s.aborting {
			// The run unwound after the abort. Stay on the log with an "aborted"
			// notice until the user dismisses it, rather than running onDone's
			// success navigation (which would land as if the work had completed).
			return s, core.SetStatusAndLog("aborted")
		}
		act := s.onDone(sh, msg)
		// A non-stay task navigates away via act (e.g. a ShowTab). A stay-task remains
		// on its log until the user dismisses it (esc/enter), but act is still applied
		// — it's expected to be non-navigational for a stay-task (e.g. a broadcast that
		// reloads another tab), so returning s keeps the screen on top.
		return s, act

	case tea.KeyMsg:
		k := msg.String()
		// While the task is still running, esc requests an abort: cancel the run's
		// context and wait for its terminating event (handled above) to unwind it.
		if !s.done && !s.aborting && core.MatchKey(k, core.Keys.Back) {
			s.aborting = true
			if s.cancel != nil {
				s.cancel()
			}
			return s, core.SetStatusAndLog("aborting…")
		}
		if s.done && (core.MatchKey(k, core.Keys.Back) || core.MatchKey(k, core.Keys.Select)) {
			// An aborted task (any kind) and a finished stay-task both linger on the
			// log until dismissed. Aborted tasks fall back to a plain Pop when the
			// caller supplied no onDismiss (non-stay tasks).
			if s.aborting && s.onDismiss == nil {
				return s, core.Pop()
			}
			if s.aborting || s.stay {
				return s, s.onDismiss(sh)
			}
		}
	}
	return s, core.Action{}
}

// View renders just the spinner/progress line; the streaming log is drawn by the
// router as shared output chrome below it.
func (s *TaskScreen) View(sh *core.Shared) string {
	glyph := sh.Spinner.View()
	if s.done {
		glyph = "•"
	}
	label := s.label
	switch {
	case s.aborting && s.done:
		label = "aborted — esc to go back"
	case s.aborting:
		label = "aborting…"
	case s.stay && s.done:
		label = s.doneLabel
	}
	return fmt.Sprintf("\n  %s %s", glyph, label)
}

func (s *TaskScreen) HelpView(sh *core.Shared) string {
	if s.done && (s.aborting || s.stay) {
		return sh.BindingHelp([]key.Binding{core.Hint("back", core.Keys.Back)})
	}
	if s.aborting {
		return sh.NoteHelp("aborting…")
	}
	return sh.BindingHelp([]key.Binding{core.Hint("abort", core.Keys.Back)})
}

func (s *TaskScreen) SetSize(sh *core.Shared, width, bodyHeight int) {}

// ---------- streaming task pump ----------

// startTask spawns run in the background, piping report() lines into the output
// log via the shared events channel, and returns the spinner tick + the wait for
// the first event.
func startTask(ctx context.Context, sh *core.Shared, run RunFunc) tea.Cmd {
	sh.Events = make(chan core.TaskEvent)
	ch := sh.Events
	go func() {
		report := func(format string, args ...any) {
			ch <- core.TaskEvent{Line: fmt.Sprintf(format, args...)}
		}
		run(ctx, sh, report, ch)
	}()
	return tea.Batch(sh.Spinner.Tick, waitForEvent(ch))
}

func waitForEvent(events chan core.TaskEvent) tea.Cmd {
	return func() tea.Msg { return <-events }
}
