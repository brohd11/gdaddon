package core

import "testing"

// The status helpers ride on Chrome.Status/Output; these use the fake panes from
// router_test.go to observe the effects without a live terminal.

func TestWriteStatusSetsAndLogs(t *testing.T) {
	st, out := &fakeStatus{}, &fakeOutput{}
	sh := NewShared(nil)
	sh.Chrome = &Chrome{Status: st, Output: out}

	sh.WriteStatus("working")
	if st.msg != "working" {
		t.Errorf("WriteStatus should set the status line, got %q", st.msg)
	}
	if len(out.logs) != 0 {
		t.Errorf("WriteStatus without the log flag should not log, got %v", out.logs)
	}

	sh.WriteStatus("saved", true)
	if len(out.logs) != 1 || out.logs[0] != "saved" {
		t.Errorf("WriteStatus(line, true) should also log the line, got %v", out.logs)
	}

	sh.ClearStatus()
	if st.Shown() {
		t.Error("ClearStatus should clear the status line")
	}
}

func TestStatusHelpersNoChromeAreNoops(t *testing.T) {
	sh := NewShared(nil) // no Chrome
	// These must not panic without chrome.
	sh.WriteStatus("x", true)
	sh.SetStatus("y")
	sh.ClearStatus()
	sh.Log("z")
}
