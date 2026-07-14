package addon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// quietReport is a Reporter that discards its lines, for tests that only care about the
// operation's effect on the repo.
func quietReport(string, ...any) {}

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

// write puts content in dir/name without committing it.
func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// head is dir's current commit sha, for asserting an operation left it where it was.
func head(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// bareUpstreamClone builds a bare "remote" repo (the only kind you can push to — pushing to
// a non-bare repo's checked-out branch is refused) seeded with one commit, plus a clone
// tracking it. Local paths only; no network.
func bareUpstreamClone(t *testing.T) (remote, work string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	base := t.TempDir()

	seed := filepath.Join(base, "seed")
	if err := os.Mkdir(seed, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, seed, "init", "-q", "-b", "main")
	commit(t, seed, "seed")

	remote = filepath.Join(base, "remote.git")
	git(t, base, "clone", "-q", "--bare", seed, remote)

	work = filepath.Join(base, "work")
	git(t, base, "clone", "-q", remote, work)
	return remote, work
}

// changeCodes reduces GitChanges to a "code path" set for order-independent assertions.
func changeCodes(t *testing.T, dir string) map[string]string {
	t.Helper()
	changes, err := GitChanges(dir)
	if err != nil {
		t.Fatalf("GitChanges: %v", err)
	}
	out := make(map[string]string, len(changes))
	for _, c := range changes {
		out[c.Path] = c.Code
	}
	return out
}

func TestGitChanges(t *testing.T) {
	t.Run("not a checkout", func(t *testing.T) {
		got, err := GitChanges(t.TempDir())
		if err != nil || got != nil {
			t.Errorf("GitChanges(plain dir) = %v, %v; want nil, nil", got, err)
		}
	})

	t.Run("clean tree", func(t *testing.T) {
		_, work := bareUpstreamClone(t)
		if got := changeCodes(t, work); len(got) != 0 {
			t.Errorf("GitChanges(clean) = %v, want empty", got)
		}
	})

	t.Run("modified, deleted, staged, untracked", func(t *testing.T) {
		_, work := bareUpstreamClone(t)
		commit(t, work, "tracked")            // a second tracked file to modify
		commit(t, work, "doomed")             // and one to delete
		write(t, work, "tracked", "modified") // unstaged modification
		if err := os.Remove(filepath.Join(work, "doomed")); err != nil {
			t.Fatal(err)
		}
		write(t, work, "staged", "new")
		git(t, work, "add", "staged") // staged addition
		write(t, work, "loose", "new")

		got := changeCodes(t, work)
		want := map[string]string{
			"tracked": " M",
			"doomed":  " D",
			"staged":  "A ",
			"loose":   "??",
		}
		for path, code := range want {
			if got[path] != code {
				t.Errorf("GitChanges[%q] = %q, want %q (full: %v)", path, got[path], code, got)
			}
		}
		if len(got) != len(want) {
			t.Errorf("GitChanges returned %d entries, want %d: %v", len(got), len(want), got)
		}
	})

	t.Run("Untracked", func(t *testing.T) {
		if !(GitChange{Code: "??"}).Untracked() {
			t.Error(`Code "??" should read as untracked`)
		}
		if (GitChange{Code: " M"}).Untracked() {
			t.Error(`Code " M" should not read as untracked`)
		}
	})
}

// TestGitCommitStaging is the correctness point the commit form's toggle exists for: `-a`
// alone leaves a brand-new file out of the commit, which is exactly the surprise a user
// picturing "commit all" would get.
func TestGitCommitStaging(t *testing.T) {
	t.Run("tracked only leaves new files behind", func(t *testing.T) {
		_, work := bareUpstreamClone(t)
		write(t, work, "seed", "edited") // a tracked modification
		write(t, work, "brand_new", "hi")

		if err := GitCommit(context.Background(), work, "touch seed", false, quietReport); err != nil {
			t.Fatalf("GitCommit: %v", err)
		}

		got := changeCodes(t, work)
		if got["brand_new"] != "??" {
			t.Errorf("after commit -a, brand_new = %q, want it still untracked (%v)", got["brand_new"], got)
		}
		if _, ok := got["seed"]; ok {
			t.Errorf("after commit -a, the tracked edit should be committed, still see: %v", got)
		}
	})

	t.Run("stageAll includes new files", func(t *testing.T) {
		_, work := bareUpstreamClone(t)
		write(t, work, "seed", "edited")
		write(t, work, "brand_new", "hi")

		if err := GitCommit(context.Background(), work, "everything", true, quietReport); err != nil {
			t.Fatalf("GitCommit: %v", err)
		}
		if got := changeCodes(t, work); len(got) != 0 {
			t.Errorf("after add -A + commit, tree should be clean, got %v", got)
		}
	})

	t.Run("nothing to commit is an error", func(t *testing.T) {
		_, work := bareUpstreamClone(t)
		if err := GitCommit(context.Background(), work, "empty", false, quietReport); err == nil {
			t.Error("GitCommit on a clean tree = nil, want git's \"nothing to commit\" error")
		}
	})
}

func TestGitPull(t *testing.T) {
	t.Run("fast-forwards a behind clone", func(t *testing.T) {
		remote, work := bareUpstreamClone(t)

		// Someone else pushes: a second clone commits and pushes to the bare remote.
		other := filepath.Join(filepath.Dir(work), "other")
		git(t, filepath.Dir(work), "clone", "-q", remote, other)
		commit(t, other, "theirs")
		git(t, other, "push", "-q")

		if err := GitPull(context.Background(), work, quietReport); err != nil {
			t.Fatalf("GitPull: %v", err)
		}
		if got := GitSyncStatus(work); got.Behind != 0 || got.Ahead != 0 {
			t.Errorf("after pull, GitSyncStatus = %+v, want in sync", got)
		}
		if _, err := os.Stat(filepath.Join(work, "theirs")); err != nil {
			t.Errorf("pull did not bring their file: %v", err)
		}
	})

	t.Run("diverged branch aborts, touching nothing", func(t *testing.T) {
		remote, work := bareUpstreamClone(t)

		other := filepath.Join(filepath.Dir(work), "other")
		git(t, filepath.Dir(work), "clone", "-q", remote, other)
		commit(t, other, "theirs")
		git(t, other, "push", "-q")

		commit(t, work, "mine") // now diverged: 1 ahead, 1 behind
		before := head(t, work)

		err := GitPull(context.Background(), work, quietReport)
		if err == nil {
			t.Fatal("GitPull on a diverged branch = nil, want a fast-forward refusal")
		}
		if !strings.Contains(err.Error(), "fast-forward") {
			t.Errorf("GitPull error = %v, want it to name the fast-forward refusal", err)
		}
		if after := head(t, work); after != before {
			t.Errorf("a refused pull moved HEAD: %s → %s", before, after)
		}
		if got := changeCodes(t, work); len(got) != 0 {
			t.Errorf("a refused pull left the working tree dirty: %v", got)
		}
	})
}

func TestGitPush(t *testing.T) {
	_, work := bareUpstreamClone(t)
	commit(t, work, "mine")
	if got := GitSyncStatus(work); got.Ahead != 1 {
		t.Fatalf("setup: GitSyncStatus = %+v, want ahead 1", got)
	}

	if err := GitPush(context.Background(), work, quietReport); err != nil {
		t.Fatalf("GitPush: %v", err)
	}
	// A push updates the local remote-tracking ref, so the count settles with no fetch.
	if got := GitSyncStatus(work); got.Ahead != 0 {
		t.Errorf("after push, GitSyncStatus = %+v, want ahead 0", got)
	}
}

func TestGitStream(t *testing.T) {
	_, work := bareUpstreamClone(t)

	t.Run("relays output line by line", func(t *testing.T) {
		var lines []string
		report := func(format string, args ...any) {
			lines = append(lines, sprintf(format, args...))
		}
		if err := GitStatus(context.Background(), work, report); err != nil {
			t.Fatalf("GitStatus: %v", err)
		}
		// `status -sb` on a clean clone prints exactly its branch header.
		if len(lines) != 1 || !strings.HasPrefix(lines[0], "## main") {
			t.Errorf("GitStatus lines = %q, want a single branch header", lines)
		}
	})

	t.Run("a failing command errors and quotes git", func(t *testing.T) {
		var lines []string
		report := func(format string, args ...any) { lines = append(lines, sprintf(format, args...)) }

		err := GitStream(context.Background(), work, report, "checkout", "no-such-branch")
		if err == nil {
			t.Fatal("GitStream on a bad command = nil, want an error")
		}
		if !strings.Contains(err.Error(), "no-such-branch") {
			t.Errorf("error = %v, want git's own message folded in", err)
		}
		if len(lines) == 0 {
			t.Error("a failing command reported no lines; git's stderr should have streamed")
		}
	})
}
