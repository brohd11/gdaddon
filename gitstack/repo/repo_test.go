package repo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// git runs a git command in dir with a deterministic commit identity, failing the test on
// error. Tests here talk to real git against local paths only — no network.
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// setIdentity gives dir a repo-local commit identity, so production ops that run without
// GIT_AUTHOR_*/COMMITTER_* env (GitCommit) can commit on a runner with no ambient identity.
func setIdentity(t *testing.T, dir string) {
	t.Helper()
	git(t, dir, "config", "user.email", "t@t")
	git(t, dir, "config", "user.name", "t")
}

// commit writes a file in dir and commits it, advancing that repo's branch by one.
func commit(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", name)
}

// initRepo creates a git repo at dir with an origin remote and a first commit.
func initRepo(t *testing.T, dir, origin, branch string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	git(t, dir, "init", "-q", "-b", branch)
	git(t, dir, "remote", "add", "origin", origin)
	commit(t, dir, "f")
}

// makeSubmodule turns dir into a submodule-shaped checkout: a real git repo whose `.git`
// is a gitdir-pointer file (not a directory), the layout git submodules use.
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

// upstreamClone builds a local "remote" repo with one commit and a clone tracking it. The
// remote is an ordinary (non-bare) repo, so a commit made in it advances the branch the
// clone tracks — which is how these tests simulate someone else pushing, with no network.
func upstreamClone(t *testing.T) (remote, work string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	base := t.TempDir()
	remote = filepath.Join(base, "remote")
	if err := os.Mkdir(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, remote, "init", "-q", "-b", "main")
	setIdentity(t, remote)
	commit(t, remote, "seed")

	work = filepath.Join(base, "work")
	git(t, base, "clone", "-q", remote, work)
	setIdentity(t, work)
	return remote, work
}

func TestFindGitRepos(t *testing.T) {
	base := t.TempDir()
	initRepo(t, base, "https://github.com/owner/root.git", "main") // top-level: excluded

	foo := filepath.Join(base, "addons", "foo") // depth 2
	os.MkdirAll(foo, 0o755)
	initRepo(t, foo, "https://github.com/owner/foo.git", "main")

	deep := filepath.Join(base, "a", "b", "c") // depth 3
	os.MkdirAll(deep, 0o755)
	initRepo(t, deep, "https://github.com/owner/deep.git", "main")

	// A submodule-shaped checkout (.git is a file, not a dir) must be found too.
	sub := filepath.Join(base, "addons", "sub") // depth 2
	os.MkdirAll(sub, 0o755)
	makeSubmodule(t, sub, "https://github.com/owner/sub.git", "main")

	os.MkdirAll(filepath.Join(base, "addons", "plain"), 0o755) // non-repo: ignored

	got, err := FindGitRepos(base, 5)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		filepath.Join("addons", "foo"): true,
		filepath.Join("addons", "sub"): true,
		filepath.Join("a", "b", "c"):   true,
	}
	if len(got) != len(want) {
		t.Fatalf("FindGitRepos = %v, want keys %v", got, want)
	}
	for _, rel := range got {
		if rel == "." || rel == "" {
			t.Errorf("base repo should be excluded; got %q", rel)
		}
		if !want[rel] {
			t.Errorf("unexpected repo %q", rel)
		}
	}

	// maxDepth caps the walk: depth-2 keeps the addons/* repos, drops the depth-3 deep repo.
	shallow, err := FindGitRepos(base, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(shallow) != 2 {
		t.Fatalf("FindGitRepos(depth=2) = %v, want the 2 depth-2 repos", shallow)
	}
}

func TestGitSyncStatus(t *testing.T) {
	t.Run("non-checkout folder", func(t *testing.T) {
		if got := GitSyncStatus(t.TempDir()); got.Tracking {
			t.Errorf("GitSyncStatus(plain dir) = %+v, want zero value", got)
		}
	})

	t.Run("clean clone", func(t *testing.T) {
		_, work := upstreamClone(t)
		got := GitSyncStatus(work)
		if !got.Tracking || got.Ahead != 0 || got.Behind != 0 {
			t.Errorf("GitSyncStatus(fresh clone) = %+v, want tracking and in sync", got)
		}
	})

	t.Run("ahead after a local commit", func(t *testing.T) {
		_, work := upstreamClone(t)
		commit(t, work, "local")
		got := GitSyncStatus(work)
		if got.Ahead != 1 || got.Behind != 0 {
			t.Errorf("GitSyncStatus(unpushed commit) = %+v, want ahead 1", got)
		}
	})

	t.Run("behind only after a fetch", func(t *testing.T) {
		remote, work := upstreamClone(t)
		commit(t, remote, "theirs")

		// The count is a local read of remote-tracking refs, so a new upstream commit is
		// invisible until something updates them.
		if got := GitSyncStatus(work); got.Behind != 0 {
			t.Fatalf("GitSyncStatus(before fetch) = %+v, want behind 0 (stale refs)", got)
		}
		if err := GitFetch(context.Background(), work); err != nil {
			t.Fatalf("GitFetch: %v", err)
		}
		if got := GitSyncStatus(work); got.Behind != 1 || got.Ahead != 0 {
			t.Errorf("GitSyncStatus(after fetch) = %+v, want behind 1", got)
		}
	})
}

func TestGitFetchCancelled(t *testing.T) {
	_, work := upstreamClone(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := GitFetch(ctx, work); err == nil {
		t.Error("GitFetch with a cancelled context = nil, want an error")
	}
}

func TestFetchAll(t *testing.T) {
	remote, work := upstreamClone(t)
	commit(t, remote, "theirs")

	// Callers pass only the checkouts worth fetching; the engine fetches exactly those.
	got := FetchAll(context.Background(), []Repo{{Name: "clone", Dir: work}})
	if len(got) != 1 {
		t.Fatalf("FetchAll returned %d results, want 1: %+v", len(got), got)
	}
	r := got[0]
	if r.Name != "clone" || r.Err != nil {
		t.Fatalf("FetchAll result = %+v, want a clean fetch of \"clone\"", r)
	}
	if r.Sync.Behind != 1 {
		t.Errorf("FetchAll result sync = %+v, want behind 1 (the fetch should have revealed it)", r.Sync)
	}
}

func TestCurrentBranch(t *testing.T) {
	if got := CurrentBranch(t.TempDir()); got != "" {
		t.Errorf("CurrentBranch(non-checkout) = %q, want empty", got)
	}
	dir := t.TempDir()
	initRepo(t, dir, "https://github.com/owner/repo.git", "trunk")
	if got := CurrentBranch(dir); got != "trunk" {
		t.Errorf("CurrentBranch = %q, want trunk", got)
	}
}

func TestScan(t *testing.T) {
	base := t.TempDir()
	initRepo(t, base, "https://github.com/owner/root.git", "main") // top-level: excluded

	a := filepath.Join(base, "alpha")
	os.MkdirAll(a, 0o755)
	initRepo(t, a, "https://github.com/owner/alpha.git", "main")

	b := filepath.Join(base, "beta")
	os.MkdirAll(b, 0o755)
	initRepo(t, b, "https://github.com/owner/beta.git", "dev")
	os.WriteFile(filepath.Join(b, "wip.txt"), []byte("x"), 0o644) // dirty

	repos, err := Scan(base, 5)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Repo{}
	for _, r := range repos {
		byName[r.Name] = r
	}
	if len(byName) != 2 {
		t.Fatalf("Scan found %d repos, want 2 (base excluded): %v", len(byName), repos)
	}
	if r := byName["alpha"]; r.Branch != "main" || r.Dirty {
		t.Errorf("alpha = %+v, want branch main, clean", r)
	}
	if r := byName["beta"]; r.Branch != "dev" || !r.Dirty {
		t.Errorf("beta = %+v, want branch dev, dirty", r)
	}
	if r := byName["beta"]; r.Dir != b {
		t.Errorf("beta.Dir = %q, want absolute %q", r.Dir, b)
	}
}

func TestGitChangesAndCommit(t *testing.T) {
	_, work := upstreamClone(t)

	// A tracked modification and a new untracked file.
	os.WriteFile(filepath.Join(work, "seed"), []byte("changed"), 0o644)
	os.WriteFile(filepath.Join(work, "new.txt"), []byte("new"), 0o644)

	changes, err := GitChanges(work)
	if err != nil {
		t.Fatal(err)
	}
	var untracked int
	for _, c := range changes {
		if c.Untracked() {
			untracked++
		}
	}
	if len(changes) != 2 || untracked != 1 {
		t.Fatalf("GitChanges = %+v, want 2 changes incl. 1 untracked", changes)
	}

	// commit -a (stageAll false) commits the tracked change; the untracked file stays.
	if err := GitCommit(context.Background(), work, "tracked only", false, func(string, ...any) {}); err != nil {
		t.Fatalf("GitCommit: %v", err)
	}
	after, _ := GitChanges(work)
	if len(after) != 1 || !after[0].Untracked() {
		t.Errorf("after commit -a, remaining = %+v, want just the untracked file", after)
	}
}
