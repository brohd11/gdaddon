package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/brohd11/gdaddon/internal/tui/appctx"
)

func TestSubmoduleRefreshUpdatesParentMarker(t *testing.T) {
	source := filepath.Join(gitProject(t), "addons", "myrepo")
	root := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(root, "init", "-q", "-b", "main")
	git(root, "-c", "protocol.file.allow=always", "submodule", "add", source, "addons/child")
	manifest := filepath.Join(root, "addon_manifest.yml")
	if err := os.WriteFile(manifest, []byte("child:\n  url: https://github.com/u/child\n  path: addons/child\n  kind: submodule\n  tag: main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(root, "add", ".")
	git(root, "commit", "-qm", "initial")
	c := &appctx.Ctx{ProjectRoot: root, ManifestPath: manifest}
	c.RefreshProject()
	if c.RootRepo.Dirty || c.GitDirty["child"] {
		t.Fatal("fixture must start clean")
	}
	child := filepath.Join(root, "addons", "child")
	if err := os.WriteFile(filepath.Join(child, "new.txt"), []byte("change"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.RefreshRepo(appctx.GitRepoRefresh{Dir: child})
	if !c.RootRepo.Dirty || !c.GitDirty["child"] {
		t.Fatal("child and parent must both show uncommitted changes")
	}
	git(child, "add", ".")
	git(child, "commit", "-qm", "child change")
	c.RefreshRepo(appctx.GitRepoRefresh{Dir: child})
	if !c.RootRepo.Dirty || c.GitDirty["child"] {
		t.Fatal("committed child must be clean while parent gitlink remains dirty")
	}
	git(root, "add", "addons/child")
	git(root, "commit", "-qm", "record child")
	c.RefreshRepo(appctx.GitRepoRefresh{Dir: root})
	if c.RootRepo.Dirty {
		t.Fatal("root commit did not clear parent marker")
	}
}
