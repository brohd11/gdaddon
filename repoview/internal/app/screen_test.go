package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// twoRepoTree builds a directory with two real git checkouts under it: "alpha" (clean, on
// main) and "beta" (a dirty working tree). It's the fixture the screen e2e drives.
func twoRepoTree(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	base := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	initRepo := func(name string) string {
		dir := filepath.Join(base, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		git(dir, "init", "-q", "-b", "main")
		git(dir, "remote", "add", "origin", "https://github.com/owner/"+name+".git")
		os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644)
		git(dir, "add", ".")
		git(dir, "commit", "-q", "-m", "init")
		return dir
	}
	initRepo("alpha")
	beta := initRepo("beta")
	os.WriteFile(filepath.Join(beta, "wip.txt"), []byte("uncommitted"), 0o644) // make beta dirty
	return base
}

// router builds the repoview router the same way Run does, rooted at a real scanned tree.
func router(root string) core.Router {
	sh := core.NewShared(New(root, 5))
	sh.Chrome = &core.Chrome{Header: core.NewHeaderPane(Header), Output: components.NewLogPane(), Status: components.NewStatusLine()}
	return core.NewRouter(sh, []core.TabEntry{
		{Title: "Repos", New: func(sh *core.Shared) core.Screen { return NewReposScreen(sh) }},
	})
}

func sized(tm tea.Model) tea.Model {
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	return tm
}

// pump delivers msg, then runs the returned command and feeds its (single, non-batch) result
// back — enough to drive the navigation commands (push/pop) and broadcasts.
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

// TestReposScreenWiring drives the whole tool: the scanned list renders with the right markers,
// enter opens a repo's shared git submenu, V the all-repos menu, a the Actions menu, and a git
// flow's RefreshMsg rebuilds the list.
func TestReposScreenWiring(t *testing.T) {
	tm := sized(router(twoRepoTree(t)))

	if _, ok := tm.(core.Router).Top().(*ReposScreen); !ok {
		t.Fatalf("want the Repos screen on top, got %T", tm.(core.Router).Top())
	}
	// The list shows both repos, and the dirty one carries the marker.
	if out := tm.View(); !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") ||
		!strings.Contains(out, "uncommitted changes") {
		t.Errorf("repo list should show both repos and beta's dirty marker:\n%s", out)
	}

	// enter on the highlighted row (alpha, first in the sorted scan) opens its git submenu.
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tm.(core.Router).Top().(*components.PickerScreen); !ok {
		t.Fatalf("enter should open the per-repo Git submenu (PickerScreen), got %T", tm.(core.Router).Top())
	}
	if out := tm.View(); !strings.Contains(out, "alpha") || !strings.Contains(out, "Pull") {
		t.Errorf("submenu should be the repo's Git menu:\n%s", out)
	}

	// esc back, then V opens the all-repos menu.
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEsc})
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'V'}})
	if _, ok := tm.(core.Router).Top().(*components.PickerScreen); !ok {
		t.Fatalf("V should open the all-repos Git menu, got %T", tm.(core.Router).Top())
	}
	if out := tm.View(); !strings.Contains(out, "all repos") {
		t.Errorf("all-repos menu title should say so:\n%s", out)
	}

	// esc back, then a opens the Actions menu with Theme + Refresh.
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEsc})
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if _, ok := tm.(core.Router).Top().(*components.PickerScreen); !ok {
		t.Fatalf("a should open the Actions menu, got %T", tm.(core.Router).Top())
	}
	if out := tm.View(); !strings.Contains(out, "Theme") || !strings.Contains(out, "Refresh") {
		t.Errorf("Actions menu should list Theme and Refresh:\n%s", out)
	}

	// esc back to the root; a git flow's RefreshMsg must rebuild the list without panicking.
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEsc})
	tm = pump(tm, core.PropagateAll(RescanMsg{}))
	if _, ok := tm.(core.Router).Top().(*ReposScreen); !ok {
		t.Fatalf("after refresh, want the Repos screen on top, got %T", tm.(core.Router).Top())
	}
	if out := tm.View(); !strings.Contains(out, "alpha") {
		t.Errorf("rebuilt list should still show the repos:\n%s", out)
	}
}
