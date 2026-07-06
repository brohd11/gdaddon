package components

import (
	"context"
	"reflect"
	"testing"

	"github.com/brohd11/bubblestack/core"
)

// The task tests drive Update directly with TaskEvents and keys, skipping Init so no
// goroutine/sh.Events channel is involved — the abort/done/dismiss branching is pure.

func noopRun(context.Context, *core.Shared, func(string, ...any), chan<- core.TaskEvent) {}

func TestTaskProgressEventReArms(t *testing.T) {
	s := NewTask("install", noopRun, func(*core.Shared, core.TaskEvent) core.Action { return core.Action{} })
	_, act := s.Update(core.NewShared(nil), core.TaskEvent{Line: "downloading"})
	if act.Cmd == nil {
		t.Error("a non-terminating event should re-arm the wait (Async cmd)")
	}
}

func TestTaskDoneRunsOnDone(t *testing.T) {
	ran := false
	s := NewTask("install", noopRun, func(*core.Shared, core.TaskEvent) core.Action {
		ran = true
		return core.ShowTab("Project")
	})
	_, act := s.Update(core.NewShared(nil), core.TaskEvent{Done: true})
	if !ran {
		t.Fatal("a terminating event should run onDone")
	}
	if !reflect.DeepEqual(act, core.ShowTab("Project")) {
		t.Errorf("Done should return onDone's Action, got %+v", act)
	}
}

func TestTaskAbortSkipsOnDone(t *testing.T) {
	ranOnDone := false
	s := NewTask("install", noopRun, func(*core.Shared, core.TaskEvent) core.Action { ranOnDone = true; return core.Action{} })
	sh := core.NewShared(nil)

	// esc while running requests an abort.
	_, act := s.Update(sh, keyMsg("esc"))
	if !reflect.DeepEqual(act, core.SetStatusAndLog("aborting…")) {
		t.Fatalf("esc should request an abort, got %+v", act)
	}

	// The run then unwinds with its terminating event: stay on the log as "aborted",
	// and do NOT run onDone's success navigation.
	_, act = s.Update(sh, core.TaskEvent{Done: true})
	if !reflect.DeepEqual(act, core.SetStatusAndLog("aborted")) {
		t.Fatalf("an aborted run should report aborted, got %+v", act)
	}
	if ranOnDone {
		t.Error("onDone must not run after an abort")
	}

	// esc on the finished-aborted non-stay task falls back to a plain Pop.
	_, act = s.Update(sh, keyMsg("esc"))
	if !reflect.DeepEqual(act, core.Pop()) {
		t.Errorf("dismissing an aborted non-stay task should pop, got %+v", act)
	}
}

func TestStayTaskLingersThenDismisses(t *testing.T) {
	dismissed := false
	s := NewStayTask("archiving", "archived", noopRun,
		func(*core.Shared, core.TaskEvent) core.Action { return core.Action{} },
		func(*core.Shared) core.Action { dismissed = true; return core.Pop() })
	sh := core.NewShared(nil)

	// Finishing keeps the screen (act is non-navigational); it lingers on its log.
	s.Update(sh, core.TaskEvent{Done: true})
	// enter (Select) then dismisses via onDismiss.
	_, act := s.Update(sh, keyMsg("enter"))
	if !dismissed || !reflect.DeepEqual(act, core.Pop()) {
		t.Errorf("a finished stay-task should dismiss via onDismiss, got dismissed=%v act=%+v", dismissed, act)
	}
}

func TestTaskViewLabels(t *testing.T) {
	sh := core.NewShared(nil)
	s := NewStayTask("archiving", "archived", noopRun,
		func(*core.Shared, core.TaskEvent) core.Action { return core.Action{} },
		func(*core.Shared) core.Action { return core.Pop() })
	if !containsAll(s.View(sh), "archiving") {
		t.Errorf("a running stay-task should show its label, got %q", s.View(sh))
	}
	s.Update(sh, core.TaskEvent{Done: true})
	if !containsAll(s.View(sh), "archived") {
		t.Errorf("a finished stay-task should show its doneLabel, got %q", s.View(sh))
	}
}
