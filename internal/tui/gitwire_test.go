package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gdaddon/internal/tui/appctx"
	"gdaddon/internal/tui/tabs/project"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// gitProject builds a Godot project root with one clone-kind addon that is a real git
// checkout, plus the manifest describing it, and returns the root. It's the fixture the
// git-wiring e2e needs: appctx.New must discover the manifest and Inspect must read the
// checkout as a present git workdir, so the "v"/"V" keys have something to act on.
func gitProject(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "addons", "myrepo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("remote", "add", "origin", "https://github.com/owner/myrepo.git")
	os.WriteFile(filepath.Join(dir, "plugin.cfg"), []byte("[plugin]\nname=\"MyRepo\"\nversion=\"1.0.0\"\n"), 0o644)
	run("add", ".")
	run("commit", "-q", "-m", "init")

	manifest := "myrepo:\n" +
		"    url: https://github.com/owner/myrepo.git\n" +
		"    path: addons/myrepo\n" +
		"    tag: main\n" +
		"    kind: clone\n"
	if err := os.WriteFile(filepath.Join(root, "addon_manifest.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// routerAt builds the router the same way newTestRouter does but rooted at a real project,
// so the Project tab inspects a live manifest.
func routerAt(root string) core.Router {
	sh := core.NewShared(appctx.New(root, "dev"))
	sh.Chrome = &core.Chrome{Header: core.NewHeaderPane(appctx.Header), Output: components.NewLogPane(), Status: components.NewStatusLine()}
	return core.NewRouter(sh, []core.TabEntry{
		{Title: appctx.TitleProject, New: func(sh *core.Shared) core.Screen { return project.NewProjectScreen(sh) }},
	})
}

// TestGitSubmenuWiring drives the seam this refactor moved: "v" on a present git checkout
// must open the shared per-repo Git submenu (repoui.RepoMenu, a PickerScreen titled with the
// repo name), and "V" the shared all-repos menu (repoui.AllReposMenu). Both screens now live
// in the gitstack/repoui module; this confirms gdaddon still reaches them and they render.
func TestGitSubmenuWiring(t *testing.T) {
	tm := sized(routerAt(gitProject(t)))

	// Sanity: the checkout is the highlighted row (the list holds only addon rows).
	if _, ok := tm.(core.Router).Top().(*project.ProjectScreen); !ok {
		t.Fatalf("want the Project root on top, got %T", tm.(core.Router).Top())
	}

	// "v" opens the per-repo Git submenu.
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if _, ok := tm.(core.Router).Top().(*components.PickerScreen); !ok {
		t.Fatalf("v on a git checkout should open the Git submenu (PickerScreen), got %T", tm.(core.Router).Top())
	}
	// The framed view renders the submenu on top: its "Git" breadcrumb, the repo title, and
	// its git-command rows. Only the first few rows fit at this window size — the list
	// paginates the rest — so assert on ones above the fold rather than on a row whose
	// position shifts whenever the menu gains an entry.
	if out := tm.View(); !strings.Contains(out, "myrepo") || !strings.Contains(out, "Git") ||
		!strings.Contains(out, "Status") || !strings.Contains(out, "Diff") {
		t.Errorf("submenu should be the repo's Git menu with git commands:\n%s", out)
	}

	// esc back to the root, then "V" opens the all-repos menu.
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEsc})
	if _, ok := tm.(core.Router).Top().(*project.ProjectScreen); !ok {
		t.Fatalf("esc should return to the Project root, got %T", tm.(core.Router).Top())
	}
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'V'}})
	if _, ok := tm.(core.Router).Top().(*components.PickerScreen); !ok {
		t.Fatalf("V should open the all-repos Git menu (PickerScreen), got %T", tm.(core.Router).Top())
	}
	if out := tm.View(); !strings.Contains(out, "all repos") {
		t.Errorf("the all-repos menu title should say so:\n%s", out)
	}
}

// TestRootGitKeyWiring: "ctrl+v" opens the project repo's own Git page (the shared
// repoui.RepoMenu, handed the root) when the project root is itself a checkout — and stays
// put with a status-line explanation when it isn't.
func TestRootGitKeyWiring(t *testing.T) {
	root := gitProject(t)
	// Make the project root itself a checkout (gitProject only inits the nested addon).
	init := exec.Command("git", "-C", root, "init", "-q", "-b", "main")
	if out, err := init.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	tm := sized(routerAt(root))
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyCtrlV})
	if _, ok := tm.(core.Router).Top().(*components.PickerScreen); !ok {
		t.Fatalf("ctrl+v should open the project repo's Git page (PickerScreen), got %T", tm.(core.Router).Top())
	}
	if out := tm.View(); !strings.Contains(out, "Git") ||
		!strings.Contains(out, "Status") || !strings.Contains(out, "Diff") {
		t.Errorf("ctrl+v should open the root's Git menu with git commands:\n%s", out)
	}

	// esc back to the Project root.
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEsc})
	if _, ok := tm.(core.Router).Top().(*project.ProjectScreen); !ok {
		t.Fatalf("esc should return to the Project root, got %T", tm.(core.Router).Top())
	}
}

// TestRootGitKeyNotACheckout: with a non-git project root, ctrl+v doesn't navigate — the
// Project root stays on top and the status line explains why. (No pump: the status
// auto-clear rides the returned cmd's timer, which a pump would run synchronously.)
func TestRootGitKeyNotACheckout(t *testing.T) {
	tm := sized(routerAt(gitProject(t))) // project root is not a checkout
	tm, _ = tm.Update(tea.KeyMsg{Type: tea.KeyCtrlV})

	if _, ok := tm.(core.Router).Top().(*project.ProjectScreen); !ok {
		t.Fatalf("ctrl+v on a non-checkout root should not navigate, got %T", tm.(core.Router).Top())
	}
	if out := tm.View(); !strings.Contains(out, "not a git checkout") {
		t.Errorf("the status line should explain why nothing opened:\n%s", out)
	}
}

