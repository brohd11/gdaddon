package addon

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

// setIdentity gives dir a repo-local commit identity, so production git operations that run
// without GIT_AUTHOR_*/COMMITTER_* env (addon.GitCommit) can commit on a CI runner whose
// account has no ambient git identity — the "empty ident name" failure otherwise.
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

func TestGitSyncStatus(t *testing.T) {
	t.Run("non-checkout folder", func(t *testing.T) {
		if got := GitSyncStatus(t.TempDir()); got.Tracking {
			t.Errorf("GitSyncStatus(plain dir) = %+v, want zero value", got)
		}
	})

	t.Run("checkout with no upstream", func(t *testing.T) {
		dir := t.TempDir()
		initRepo(t, dir, "https://github.com/owner/repo.git", "main")
		// An origin remote alone isn't an upstream: nothing has been fetched, so the branch
		// tracks nothing and there's no ref to compare against.
		if got := GitSyncStatus(dir); got.Tracking {
			t.Errorf("GitSyncStatus(untracked branch) = %+v, want Tracking false", got)
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

		// The whole feature rests on this: the count is a local read of remote-tracking refs,
		// so a new upstream commit is invisible until something updates them.
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

	t.Run("diverged", func(t *testing.T) {
		remote, work := upstreamClone(t)
		commit(t, remote, "theirs")
		commit(t, work, "mine")
		if err := GitFetch(context.Background(), work); err != nil {
			t.Fatalf("GitFetch: %v", err)
		}
		if got := GitSyncStatus(work); got.Behind != 1 || got.Ahead != 1 {
			t.Errorf("GitSyncStatus(diverged) = %+v, want behind 1, ahead 1", got)
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

	statuses := []Status{
		// A package entry: no .git, nothing to fetch — skipped.
		{Addon: Addon{Name: "pkg"}, State: StateInstalled, FullPath: t.TempDir()},
		// A clone that isn't on disk yet — skipped (not Present).
		{Addon: Addon{Name: "absent", Kind: KindClone}, State: StateMissing, FullPath: filepath.Join(t.TempDir(), "nope")},
		{Addon: Addon{Name: "clone", Kind: KindClone}, State: StateUnversioned, FullPath: work},
	}

	got := FetchAll(context.Background(), statuses)
	if len(got) != 1 {
		t.Fatalf("FetchAll returned %d results, want only the present checkout: %+v", len(got), got)
	}
	r := got[0]
	if r.Name != "clone" || r.Err != nil {
		t.Fatalf("FetchAll result = %+v, want a clean fetch of \"clone\"", r)
	}
	if r.Sync.Behind != 1 {
		t.Errorf("FetchAll result sync = %+v, want behind 1 (the fetch should have revealed it)", r.Sync)
	}
}
