package components

import (
	"reflect"
	"testing"

	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// fetchResult is a stand-in for a caller's fetch message (releasesMsg / branchesMsg).
type fetchResult struct{ ok bool }

func TestLoadingEscCancelsAndPops(t *testing.T) {
	cancelled := false
	s := &LoadingScreen{
		Title:    "Fetching",
		OnResult: func(*core.Shared, tea.Msg) core.Action { return core.Action{} },
	}
	// Init normally installs the cancel; set it directly since we drive Update alone.
	s.cancel = func() { cancelled = true }

	_, act := s.Update(core.NewShared(nil), keyMsg("esc"))
	if !cancelled {
		t.Error("esc should cancel the in-flight fetch")
	}
	if !reflect.DeepEqual(act, core.Seq(core.SetStatus("cancelled"), core.Pop())) {
		t.Errorf("esc should report cancelled and pop, got %+v", act)
	}
}

func TestLoadingNonBackKeyIsInert(t *testing.T) {
	s := &LoadingScreen{OnResult: func(*core.Shared, tea.Msg) core.Action { return core.Action{} }}
	_, act := s.Update(core.NewShared(nil), keyMsg("x"))
	if !reflect.DeepEqual(act, core.Action{}) {
		t.Errorf("a non-Back key should be inert, got %+v", act)
	}
}

func TestLoadingResultRoutesToOnResult(t *testing.T) {
	var got tea.Msg
	s := &LoadingScreen{OnResult: func(_ *core.Shared, msg tea.Msg) core.Action {
		got = msg
		return core.Pop()
	}}
	_, act := s.Update(core.NewShared(nil), fetchResult{ok: true})
	if got != (fetchResult{ok: true}) {
		t.Errorf("a non-key message should be forwarded to OnResult, got %v", got)
	}
	if !reflect.DeepEqual(act, core.Pop()) {
		t.Errorf("OnResult's Action should be returned, got %+v", act)
	}
}
