// Package repo is the domain-neutral git engine: read-only status probes and mutating
// operations over plain filesystem paths, with progress surfaced through a Reporter
// callback. It knows nothing of any manifest, addon, or app that drives it — it operates
// on directories and git, so it can back any tool that views or works a tree of git
// checkouts. The screens that render it live in the sibling repoui package.
//
// This file holds the read-only half: discovery (FindGitRepos), working-tree state
// (HasUncommittedChanges, GitChanges), upstream divergence (GitSyncStatus), and the one
// network call (GitFetch) plus its fan-out (FetchAll). The mutating/streaming operations
// (pull/push/commit) are in ops.go.
package repo

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Reporter is a sink for human-readable progress lines. A CLI prints them to stdout; a
// TUI funnels them into its log — the engine only calls it, never assumes where they go.
type Reporter func(format string, args ...any)

// Repo is one git checkout a caller wants viewed or operated on: its display name and
// working directory, plus optional cached state (the checked-out branch and the last-read
// divergence/dirty flags) that a screen can render without re-reading git. The engine's own
// operations take a plain dir; Repo is the value the caller carries a repo around as.
type Repo struct {
	Name   string
	Dir    string
	Branch string  // checked-out branch, for display ("" when unknown/detached)
	Sync   GitSync // cached divergence from upstream, as of the caller's last read
	Dirty  bool    // cached: working tree has uncommitted changes
}

// maxConcurrentFetch caps how many fetches FetchAll runs at once, so a large tree can't
// fire hundreds of parallel git processes (and trip host rate limits).
const maxConcurrentFetch = 8

// HasUncommittedChanges reports whether dir is a git checkout (a standalone clone or
// a submodule) with a dirty working tree (modified or untracked files). False when
// dir isn't a checkout (no `.git` entry) or the tree is clean.
func HasUncommittedChanges(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return false
	}
	return gitOutput(dir, "status", "--porcelain") != ""
}

// GitSync is a checkout's divergence from its upstream tracking branch, as of the last
// fetch. Reading it touches no network — it compares HEAD against the remote-tracking ref
// git already has on disk, so it's the same cost class as HasUncommittedChanges and cheap
// enough to recompute on every inspect. The flip side is that it stays stale until
// something runs GitFetch, which is git's own model (`git status` says "behind" off the
// same stale ref).
type GitSync struct {
	Ahead    int  // local commits not on the upstream (unpushed)
	Behind   int  // upstream commits not local (unpulled)
	Tracking bool // false when dir isn't a checkout, HEAD is detached, or the branch has no upstream
}

// GitSyncStatus reports dir's divergence from its upstream. The zero value (Tracking
// false) covers every case with nothing to compare: not a checkout, a detached HEAD, or a
// branch that tracks nothing — gitOutput folds all of those into an empty string.
func GitSyncStatus(dir string) GitSync {
	// An empty dir would resolve ".git" against the process's cwd — which may well be a
	// repo — and report a wholly unrelated checkout's divergence.
	if dir == "" {
		return GitSync{}
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return GitSync{}
	}
	// --left-right --count over the symmetric difference prints "<left>\t<right>": commits
	// reachable from the upstream but not HEAD (behind), then from HEAD but not the
	// upstream (ahead).
	out := gitOutput(dir, "rev-list", "--left-right", "--count", "@{upstream}...HEAD")
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return GitSync{}
	}
	behind, err1 := strconv.Atoi(fields[0])
	ahead, err2 := strconv.Atoi(fields[1])
	if err1 != nil || err2 != nil {
		return GitSync{}
	}
	return GitSync{Ahead: ahead, Behind: behind, Tracking: true}
}

// GitFetch updates dir's remote-tracking refs from its remote, so a following
// GitSyncStatus reports a current ahead/behind. It's the one network-bound git call in
// this file, hence the ctx (cancel/deadline) and the explicit error — gitOutput can't
// serve here, since it has neither. GIT_TERMINAL_PROMPT=0 (as on the clone paths) makes a
// repo whose credentials aren't cached fail fast rather than block forever on an
// interactive prompt no TUI can answer.
func GitFetch(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "fetch")
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	return nil
}

// FetchResult is one repo's outcome from FetchAll: the fetch error (nil on success) and
// the divergence read back afterwards, so a caller can report what the fetch revealed
// without re-reading git itself.
type FetchResult struct {
	Name string
	Err  error
	Sync GitSync
}

// FetchAll fetches every repo in repos, concurrently but capped at maxConcurrentFetch, and
// reads each one's post-fetch divergence. Callers pass only the checkouts worth fetching
// (this engine does not filter). ctx bounds the whole batch — a cancel aborts the in-flight
// fetches. Results are name-sorted so the caller's output is deterministic.
func FetchAll(ctx context.Context, repos []Repo) []FetchResult {
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrentFetch)
	var out []FetchResult
	for _, r := range repos {
		wg.Add(1)
		go func(name, dir string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res := FetchResult{Name: name, Err: GitFetch(ctx, dir)}
			res.Sync = GitSyncStatus(dir)
			mu.Lock()
			out = append(out, res)
			mu.Unlock()
		}(r.Name, r.Dir)
	}
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// FindGitRepos returns base-relative paths of every git checkout nested under base
// (excluding base itself), up to maxDepth directory levels deep. A directory is a
// checkout when it has a `.git` entry (a directory for a standalone clone, a file
// for a submodule — same test as HasUncommittedChanges). It descends into found
// repos so nested submodules are reported too, but never walks into `.git`
// internals, and skips unreadable entries rather than failing the whole walk.
func FindGitRepos(base string, maxDepth int) ([]string, error) {
	var repos []string
	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == ".git" {
			return filepath.SkipDir
		}
		if pathDepth(base, path) > maxDepth {
			return filepath.SkipDir
		}
		if path == base {
			return nil
		}
		if _, err := os.Stat(filepath.Join(path, ".git")); err == nil {
			if rel, err := filepath.Rel(base, path); err == nil {
				repos = append(repos, rel)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return repos, nil
}

// GitChange is one entry of `git status --porcelain`: the two-character status code (index
// column, then worktree column) and the repo-relative path. Code "??" marks an untracked
// file — the distinction the commit flow turns on, since `commit -a` will not include one.
type GitChange struct {
	Code string
	Path string
}

// Untracked reports whether the change is a file git isn't tracking yet.
func (c GitChange) Untracked() bool { return c.Code == "??" }

// GitChanges lists everything `git status --porcelain` reports in dir: staged changes,
// unstaged modifications, and untracked files. Empty (not an error) for a clean tree or a
// folder that isn't a checkout — the same tolerant reading as HasUncommittedChanges, which
// is just the "is this list empty" question. core.quotepath=false keeps non-ASCII paths
// readable rather than \NNN-escaped.
func GitChanges(dir string) ([]GitChange, error) {
	if dir == "" {
		return nil, nil
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return nil, nil
	}
	out, err := exec.Command("git", "-C", dir, "-c", "core.quotepath=false", "status", "--porcelain").Output()
	if err != nil {
		return nil, fmt.Errorf("could not read git status in %s: %w", dir, err)
	}

	var changes []GitChange
	for _, line := range strings.Split(string(out), "\n") {
		// "XY path" — the code is fixed-width, so the path starts at column 3. A rename
		// arrives as "R  old -> new"; the whole "old -> new" is kept as the path, which is
		// what a reader wants to see anyway.
		if len(line) < 4 {
			continue
		}
		changes = append(changes, GitChange{
			Code: line[:2],
			Path: strings.TrimSpace(line[3:]),
		})
	}
	return changes, nil
}

// gitOutput runs a read-only `git -C dir <args...>` and returns its trimmed stdout,
// or "" on any error (a folder may be a repo with no origin, etc.).
func gitOutput(dir string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitEnv is the environment every git subprocess this engine spawns runs with. Both
// settings exist to guarantee git can never sit waiting for input a TUI has no way to
// supply: GIT_TERMINAL_PROMPT=0 makes a repo with uncached credentials fail instead of
// prompting, and GIT_EDITOR=true makes any path that would open an editor (a merge message,
// say) take the empty-editor exit rather than hanging behind the UI forever.
func gitEnv() []string {
	return append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true")
}

// pathDepth is the number of path segments from base to path (0 when equal).
func pathDepth(base, path string) int {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(os.PathSeparator)) + 1
}
