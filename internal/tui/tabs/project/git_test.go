package project

import (
	"fmt"
	"strings"
	"testing"

	"gdaddon/internal/addon"
)

func st(name, branch string) addon.Status {
	return addon.Status{Addon: addon.Addon{Name: name, Kind: addon.KindClone}, LiveBranch: branch}
}

var sampleChanges = []addon.GitChange{
	{Code: " M", Path: "timeline.gd"},
	{Code: " D", Path: "old.gd"},
	{Code: "??", Path: "new_event.gd"},
}

func TestCommitBodyTrackedOnly(t *testing.T) {
	body := commitBody(st("dialogic", "main"), sampleChanges, "fix timeline crash", false)

	if !strings.Contains(body, "Commit 2 file(s) in dialogic on main:") {
		t.Errorf("body should count only the 2 tracked files and name the branch:\n%s", body)
	}
	for _, want := range []string{"timeline.gd", "old.gd", "message: fix timeline crash"} {
		if !strings.Contains(body, want) {
			t.Errorf("body is missing %q:\n%s", want, body)
		}
	}
	// The whole point of the -a mode's confirm: the new file is named as excluded, not
	// quietly dropped.
	if !strings.Contains(body, "Not included") || !strings.Contains(body, "new_event.gd") {
		t.Errorf("body must name the untracked file it will leave out:\n%s", body)
	}
}

func TestCommitBodyStageAll(t *testing.T) {
	body := commitBody(st("dialogic", "main"), sampleChanges, "everything", true)

	if !strings.Contains(body, "Commit 3 file(s)") {
		t.Errorf("stageAll should count the untracked file too:\n%s", body)
	}
	if !strings.Contains(body, "new_event.gd") {
		t.Errorf("stageAll should list the untracked file as included:\n%s", body)
	}
	// Nothing is excluded in this mode, so the warning section would be a lie.
	if strings.Contains(body, "Not included") {
		t.Errorf("stageAll excludes nothing; there should be no exclusion notice:\n%s", body)
	}
}

// TestCommitBodyCaps guards the reason the list is capped at all: a DialogScreen neither
// scrolls nor clips, so an uncapped list would shove the chrome off the terminal.
func TestCommitBodyCaps(t *testing.T) {
	var many []addon.GitChange
	for i := 0; i < 25; i++ {
		many = append(many, addon.GitChange{Code: " M", Path: fmt.Sprintf("file%02d.gd", i)})
	}
	body := commitBody(st("big", "main"), many, "sweeping change", false)

	if n := strings.Count(body, ".gd"); n != maxCommitList {
		t.Errorf("body lists %d files, want it capped at %d:\n%s", n, maxCommitList, body)
	}
	if !strings.Contains(body, "… and 15 more") {
		t.Errorf("body should say how many it left out:\n%s", body)
	}
	if !strings.Contains(body, "Commit 25 file(s)") {
		t.Errorf("the count must still be the true total, not the shown subset:\n%s", body)
	}
}

func TestCommitBodyCleanNoBranch(t *testing.T) {
	// A detached/unknown branch just omits the " on <branch>" clause rather than printing an
	// empty one.
	body := commitBody(st("addon", ""), sampleChanges[:1], "msg", false)
	if strings.Contains(body, " on :") || strings.Contains(body, "on \n") {
		t.Errorf("an unknown branch should be omitted, not printed empty:\n%s", body)
	}
	if !strings.Contains(body, "Commit 1 file(s) in addon:") {
		t.Errorf("unexpected header:\n%s", body)
	}
}

func TestCommitable(t *testing.T) {
	if got := commitable(sampleChanges, false); len(got) != 2 {
		t.Errorf("commitable(-a) = %v, want the 2 tracked changes", got)
	}
	if got := commitable(sampleChanges, true); len(got) != 3 {
		t.Errorf("commitable(-A) = %v, want all 3", got)
	}
	// An untracked-only tree has nothing to commit under -a; the form relies on this to stop
	// the user before a confirm that would list nothing.
	untrackedOnly := []addon.GitChange{{Code: "??", Path: "new.gd"}}
	if got := commitable(untrackedOnly, false); len(got) != 0 {
		t.Errorf("commitable(-a) over untracked-only = %v, want empty", got)
	}
}
