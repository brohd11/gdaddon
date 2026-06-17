package components

import (
	"fmt"

	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// RunFunc executes a streaming background task: report() pipes progress lines into
// the log, and the terminating core.TaskEvent (Done:true) is sent on done.
type RunFunc func(sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent)

// TaskScreen runs a streaming background task and shows its log. It is context-
// agnostic: the calling tab supplies run (the work), onDone (what to do with the
// terminating event), and — for tasks that stay on the log until dismissed — a
// doneLabel + onDismiss. It names no domain type.
type TaskScreen struct {
	label, doneLabel string
	stay             bool
	run              RunFunc
	onDone           func(*core.Shared, core.TaskEvent) core.Action
	onDismiss        func(*core.Shared) core.Action
	done             bool
}

// NewTask builds a task that navigates away as soon as it finishes (install,
// install-all): onDone returns the navigation Action for the terminating event.
func NewTask(label string, run RunFunc, onDone func(*core.Shared, core.TaskEvent) core.Action) *TaskScreen {
	return &TaskScreen{label: label, run: run, onDone: onDone}
}

// NewStayTask builds a task that stays on the log after finishing (archive) until
// the user dismisses it: onDone records the result, onDismiss runs on esc/enter/q.
func NewStayTask(label, doneLabel string, run RunFunc,
	onDone func(*core.Shared, core.TaskEvent) core.Action, onDismiss func(*core.Shared) core.Action) *TaskScreen {
	return &TaskScreen{label: label, doneLabel: doneLabel, stay: true, run: run, onDone: onDone, onDismiss: onDismiss}
}

func (s *TaskScreen) Init(sh *core.Shared) tea.Cmd { return startTask(sh, s.run) }

func (s *TaskScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	switch msg := msg.(type) {
	case core.TaskEvent:
		if !msg.Done {
			sh.Log(msg.Line)
			return s, core.Async(waitForEvent(sh.Events))
		}
		s.done = true
		act := s.onDone(sh, msg)
		if s.stay {
			return s, core.Action{} // wait for the user to dismiss (esc/enter/q)
		}
		return s, act

	case tea.KeyMsg:
		if s.stay && s.done {
			k := msg.String()
			if core.MatchKey(k, core.Keys.Back) || core.MatchKey(k, core.Keys.Select) || core.MatchKey(k, core.Keys.Quit) {
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
	if s.stay && s.done {
		label = s.doneLabel
	}
	return fmt.Sprintf("\n  %s %s", glyph, label)
}

func (s *TaskScreen) HelpView(sh *core.Shared) string {
	if s.stay && s.done {
		return sh.BindingHelp([]key.Binding{core.Hint("back", core.Keys.Back)})
	}
	return sh.NoteHelp("non-interactive · working…")
}

func (s *TaskScreen) SetSize(sh *core.Shared, width, bodyHeight int) {}

// ---------- streaming task pump ----------

// startTask spawns run in the background, piping report() lines into the output
// log via the shared events channel, and returns the spinner tick + the wait for
// the first event.
func startTask(sh *core.Shared, run RunFunc) tea.Cmd {
	sh.Events = make(chan core.TaskEvent)
	ch := sh.Events
	go func() {
		report := func(format string, args ...any) {
			ch <- core.TaskEvent{Line: fmt.Sprintf(format, args...)}
		}
		run(sh, report, ch)
	}()
	return tea.Batch(sh.Spinner.Tick, waitForEvent(ch))
}

func waitForEvent(events chan core.TaskEvent) tea.Cmd {
	return func() tea.Msg { return <-events }
}
