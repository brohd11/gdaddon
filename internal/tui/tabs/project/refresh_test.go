package project

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	gitflow "github.com/brohd11/gdaddon/internal/tui/flows/git"
	"github.com/brohd11/gitstack/repoui"
)

func TestTargetedRefreshThirtyRepos(t *testing.T) {
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
	// Log every invocation, including those hidden in rendering and scope providers.
	write(filepath.Join(bin, "git"), `#!/bin/sh
printf '%s\n' "$2" >> "$REFRESH_TEST_LOG"
case "$3" in
  rev-parse) printf '%s\n' "$REFRESH_TEST_BRANCH" ;;
  rev-list) printf '%s\n' "$REFRESH_TEST_SYNC" ;;
  status) printf '%s\n' "$REFRESH_TEST_DIRTY" ;;
esac
`, 0o755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("REFRESH_TEST_LOG", log)
	t.Setenv("REFRESH_TEST_BRANCH", "main")
	t.Setenv("REFRESH_TEST_SYNC", "1 2")
	t.Setenv("REFRESH_TEST_DIRTY", " M plugin.cfg")
	var manifest strings.Builder
	for i := 0; i < 30; i++ {
		name := fmt.Sprintf("repo%02d", i)
		dir := filepath.Join(base, name)
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		write(filepath.Join(dir, "plugin.cfg"), "[plugin]\nversion=\"1.0.0\"\n", 0o644)
		fmt.Fprintf(&manifest, "%s:\n  url: https://github.com/u/%s\n  path: %s\n  kind: clone\n  tag: main\n", name, name, name)
		if i == 1 {
			manifest.WriteString("  is_dependency: true\n")
		}
	}
	if err := os.Mkdir(filepath.Join(base, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(base, "addon_manifest.yml")
	write(manifestPath, manifest.String(), 0o644)
	c := &appctx.Ctx{ProjectRoot: base, ManifestPath: manifestPath}
	sh := core.NewShared(c)
	screen := NewProjectScreen(sh)
	screen.SetSize(sh, 100, 30)
	screen.List().SetFilterText("repo00")
	screen.SetCompact(true)
	batchMenu := gitflow.AllRepos(sh)
	if !c.OrphanDeps["repo01"] {
		t.Fatal("expected initially unused dependency")
	}
	write(log, "", 0o644)
	t.Setenv("REFRESH_TEST_BRANCH", "dev")
	t.Setenv("REFRESH_TEST_SYNC", "")
	t.Setenv("REFRESH_TEST_DIRTY", "")
	target := filepath.Join(base, "repo00")
	write(filepath.Join(target, "plugin.cfg"), "[plugin]\nversion=\"2.0.0\"\ndeps=[\"u/repo01\", \"u/missing\"]\n", 0o644)
	msg := appctx.GitRepoRefresh{Dir: target}
	if act := screen.Receive(sh, msg); act.Cmd != nil {
		t.Fatal("targeted refresh started async/network work")
	}
	batchMenu.Receive(sh, msg)
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(calls) != 6 {
		t.Fatalf("got %d git queries, want 3 each for target and root: %s", len(calls), data)
	}
	for _, dir := range calls {
		if dir != target && dir != base {
			t.Fatalf("queried unrelated repo %s", dir)
		}
	}
	statuses := c.ProjectStatuses()
	if statuses[0].LiveBranch != "dev" || statuses[0].State != addon.StateBranchChanged || statuses[0].LocalVersion != "2.0.0" {
		t.Fatalf("stale target status: %+v", statuses[0])
	}
	if statuses[1].LiveBranch != "main" || !c.GitDirty["repo01"] || !c.GitSync["repo01"].Tracking {
		t.Fatal("unrelated repo cache changed")
	}
	if c.GitDirty["repo00"] || c.GitSync["repo00"].Tracking || c.RootRepo.Dirty {
		t.Fatal("stale target/parent git markers")
	}
	if c.OrphanDeps["repo01"] || len(c.DepStatuses["repo00"]) != 2 || c.DepStatuses["repo00"][1].State != addon.DepMissing {
		t.Fatal("dependency warnings were not recomputed")
	}
	if screen.List().FilterValue() != "repo00" || !screen.Compact() || len(screen.List().VisibleItems()) != 1 {
		t.Fatal("refresh dropped filter/density")
	}
	if !strings.Contains(screen.List().VisibleItems()[0].FilterValue(), "branch changed") {
		t.Fatal("target row was not rebuilt")
	}
	write(log, "", 0o644)
	for _, msg := range []appctx.GitRepoRefresh{{}, {Dir: filepath.Join(base, "unknown")}} {
		screen.Receive(sh, msg)
	}
	data, err = os.ReadFile(log)
	if err != nil || len(data) != 0 {
		t.Fatalf("unknown target queried git: %s, %v", data, err)
	}
	// The root action must reload manifest changes and all git state, without
	// starting the release-update check that a manual ProjectDirty would start.
	write(manifestPath, manifest.String()+"new:\n  url: https://github.com/u/new\n  path: new\n", 0o644)
	if act := screen.Receive(sh, appctx.GitRepoRefresh{Dir: base}); act.Cmd != nil {
		t.Fatal("root git refresh started network work")
	}
	if len(c.ProjectStatuses()) != 31 || c.GitDirty["repo01"] {
		t.Fatal("root action did not reload the whole project")
	}
	// Both batch completion forms retain full local refreshes.
	for _, payload := range []any{appctx.GitRefresh{}, repoui.FetchDoneMsg{}} {
		t.Setenv("REFRESH_TEST_DIRTY", " M plugin.cfg")
		screen.Receive(sh, payload)
		if !c.GitDirty["repo01"] {
			t.Fatalf("%T did not reload unrelated repo", payload)
		}
		t.Setenv("REFRESH_TEST_DIRTY", "")
		c.RefreshProject()
	}
}
