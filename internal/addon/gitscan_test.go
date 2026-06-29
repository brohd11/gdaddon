package addon

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNormalizeGitRemote(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"https://github.com/owner/repo.git", "https://github.com/owner/repo.git"},
		{"https://github.com/owner/repo", "https://github.com/owner/repo"},
		{"git@github.com:owner/repo.git", "https://github.com/owner/repo.git"},
		{"git@codeberg.org:owner/repo", "https://codeberg.org/owner/repo"},
		{"  https://github.com/owner/repo.git  ", "https://github.com/owner/repo.git"},
		{"", ""},
		{"ssh://git@github.com/owner/repo.git", ""}, // not the scp shorthand we handle
		{"garbage", ""},
	}
	for _, c := range cases {
		if got := normalizeGitRemote(c.raw); got != c.want {
			t.Errorf("normalizeGitRemote(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

// initRepo creates a git repo at dir with an origin remote and a branch, so gitProbe
// has something real to read. Skips the test when git isn't available.
func initRepo(t *testing.T, dir, origin, branch string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
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
	run("init", "-q", "-b", branch)
	run("remote", "add", "origin", origin)
	os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644)
	run("add", ".")
	run("commit", "-q", "-m", "init")
}

// makeSubmodule turns dir into a submodule-shaped checkout: a real git repo whose
// `.git` is a gitdir-pointer file (not a directory), the layout git submodules use.
// git -C dir then reads remote/branch normally, but the file form marks it a submodule.
func makeSubmodule(t *testing.T, dir, origin, branch string) {
	t.Helper()
	initRepo(t, dir, origin, branch)
	gitDir := filepath.Join(dir, ".git")
	moved := filepath.Join(t.TempDir(), "gitdir")
	if err := os.Rename(gitDir, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gitDir, []byte("gitdir: "+moved+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGitProbe(t *testing.T) {
	t.Run("non-checkout folder", func(t *testing.T) {
		dir := t.TempDir()
		if kind, remote, branch := gitProbe(dir); kind != gitNone || remote != "" || branch != "" {
			t.Errorf("got (%d, %q, %q), want gitNone", kind, remote, branch)
		}
	})

	t.Run("submodule (.git file) reports remote and branch", func(t *testing.T) {
		dir := t.TempDir()
		makeSubmodule(t, dir, "git@github.com:owner/sub.git", "main")
		kind, remote, branch := gitProbe(dir)
		if kind != gitSubmodule {
			t.Fatalf("kind = %d, want gitSubmodule", kind)
		}
		if remote != "https://github.com/owner/sub.git" {
			t.Errorf("remote = %q, want normalized https", remote)
		}
		if branch != "main" {
			t.Errorf("branch = %q, want main", branch)
		}
	})

	t.Run("standalone repo reports remote and branch", func(t *testing.T) {
		dir := t.TempDir()
		initRepo(t, dir, "git@github.com:owner/repo.git", "trunk")
		kind, remote, branch := gitProbe(dir)
		if kind != gitRepo {
			t.Fatalf("kind = %d, want gitRepo", kind)
		}
		if remote != "https://github.com/owner/repo.git" {
			t.Errorf("remote = %q, want normalized https", remote)
		}
		if branch != "trunk" {
			t.Errorf("branch = %q, want trunk", branch)
		}
	})
}

func TestScanInstalledGit(t *testing.T) {
	root := t.TempDir()

	// A standalone-repo addon: its own .git dir, an origin remote, a plugin.cfg.
	repo := filepath.Join(root, "addons", "repoaddon")
	os.MkdirAll(repo, 0o755)
	os.WriteFile(filepath.Join(repo, "plugin.cfg"), []byte("[plugin]\nname=\"RepoAddon\"\nversion=\"1.0.0\"\n"), 0o644)
	initRepo(t, repo, "https://github.com/owner/repoaddon.git", "main")

	// A submodule addon: .git is a file → surfaced as KindSubmodule (parent-managed).
	sub := filepath.Join(root, "addons", "subaddon")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(sub, "plugin.cfg"), []byte("[plugin]\nname=\"SubAddon\"\n"), 0o644)
	makeSubmodule(t, sub, "git@github.com:owner/subaddon.git", "main")

	found, err := ScanInstalled(root)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	var repoAddon, subAddon *Installed
	for i := range found {
		names = append(names, found[i].Name)
		switch found[i].Name {
		case "RepoAddon":
			repoAddon = &found[i]
		case "SubAddon":
			subAddon = &found[i]
		}
	}
	if repoAddon == nil {
		t.Fatalf("RepoAddon not found; got %v", names)
	}
	if repoAddon.Kind != KindClone {
		t.Errorf("standalone repo Kind = %q, want clone", repoAddon.Kind)
	}
	if repoAddon.Branch != "main" {
		t.Errorf("Branch = %q, want main", repoAddon.Branch)
	}
	if subAddon == nil {
		t.Fatalf("SubAddon (submodule) should be included; got %v", names)
	}
	if subAddon.Kind != KindSubmodule {
		t.Errorf("submodule Kind = %q, want submodule", subAddon.Kind)
	}
	if subAddon.Branch != "main" {
		t.Errorf("submodule Branch = %q, want main", subAddon.Branch)
	}
	if subAddon.SuggestedURL != "https://github.com/owner/subaddon.git" {
		t.Errorf("submodule SuggestedURL = %q, want the origin remote", subAddon.SuggestedURL)
	}
	if repoAddon.SuggestedURL != "https://github.com/owner/repoaddon.git" {
		t.Errorf("SuggestedURL = %q, want the origin remote", repoAddon.SuggestedURL)
	}
}
