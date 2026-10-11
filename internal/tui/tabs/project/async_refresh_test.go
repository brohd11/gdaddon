package project

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
)

// fakeGitProject builds n clone entries under a checkout root with a git stub that logs
// each call and reports every checkout dirty on branch main. It returns the root and the
// call log path.
func fakeGitProject(t *testing.T, n int) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell git probe")
	}
	base, bin := t.TempDir(), t.TempDir()
	log := filepath.Join(t.TempDir(), "calls")
	write := func(path, data string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(bin, "git"), `#!/bin/sh
printf '%s\n' "$2" >> "$ASYNC_TEST_LOG"
case "$3" in
  rev-parse) echo main ;;
  status) echo " M plugin.cfg" ;;
esac
`, 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ASYNC_TEST_LOG", log)
	var manifest strings.Builder
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("repo%02d", i)
		if err := os.MkdirAll(filepath.Join(base, name, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&manifest, "%s:\n  url: https://github.com/u/%s\n  path: %s\n  kind: clone\n  tag: main\n", name, name, name)
	}
	if err := os.Mkdir(filepath.Join(base, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(base, "addon_manifest.yml"), manifest.String(), 0o644)
	return base, log
}

func gitCalls(t *testing.T, log string) int {
	t.Helper()
	data, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(strings.Fields(string(data)))
}

// TestDeferredStartupLandsAsync: the deferred context runs no git, its rows draw pending
// with no state, and the startup GitRefresh lands every entry, un-mutes the rows and ends
// with "Refreshed".
func TestDeferredStartupLandsAsync(t *testing.T) {
	base, log := fakeGitProject(t, 4)
	c := appctx.NewDeferred(base, "dev")
	if n := gitCalls(t, log); n != 0 {
		t.Fatalf("the deferred context ran %d git queries", n)
	}
	sh := core.NewShared(c)
	screen := NewProjectScreen(sh)
	screen.SetSize(sh, 100, 30)
	for _, it := range screen.List().Items() {
		if row := it.(components.Item); !row.Pending || row.Desc != "refreshing…" || row.Pick != nil {
			t.Fatalf("an uninspected row must be pending with no state or actions: %+v", row)
		}
	}
	statuses := drain(sh, screen, screen.Receive(sh, appctx.GitRefresh{}))
	if len(statuses) == 0 || statuses[len(statuses)-1] != "Refreshed" {
		t.Fatalf("refresh must finish with Refreshed, got %q", statuses)
	}
	if len(c.Pending) != 0 || c.RootPending || c.Refresh.Running() || c.RootRepo == nil {
		t.Fatal("every entry and the root must land")
	}
	for _, it := range screen.List().Items() {
		row := it.(components.Item)
		if row.Pending || !strings.Contains(row.Name, "uncommitted changes") {
			t.Fatalf("landed rows must un-mute and carry their markers: %+v", row)
		}
	}
}

// TestSupersededRefreshSkipsGit: two reloads in one tick (RefreshAll sends ProjectDirty and
// PathRefresh) inspect the project once, not twice.
func TestSupersededRefreshSkipsGit(t *testing.T) {
	const n = 6
	base, log := fakeGitProject(t, n)
	c := appctx.NewDeferred(base, "dev")
	sh := core.NewShared(c)
	screen := NewProjectScreen(sh)
	first := screen.Receive(sh, appctx.GitRefresh{})
	second := screen.Receive(sh, appctx.GitRefresh{})
	drain(sh, screen, core.Seq(first, second))
	// Three queries per clone (branch, status, divergence) and per root describe.
	if got, want := gitCalls(t, log), 3*(n+1); got != want {
		t.Fatalf("got %d git queries, want %d (one pass)", got, want)
	}
	if c.Refresh.Running() || len(c.Pending) != 0 {
		t.Fatal("the current pass must complete")
	}
}
