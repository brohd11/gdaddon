package addon

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

// gitKind classifies a scanned plugin folder by its `.git` entry.
type gitKind int

const (
	gitNone      gitKind = iota // no .git entry: not its own checkout
	gitRepo                     // .git is a directory: a standalone clone
	gitSubmodule                // .git is a file: a parent-managed submodule
)

// gitProbe classifies dir by its `.git` entry and, for a real checkout (a standalone
// repo or a submodule), returns its origin remote (ssh scp form normalized to https)
// and checked-out branch ("" on a detached HEAD). The `.git`-presence check is what
// keeps a plain subfolder of the project repo from resolving to the project's own
// remote: such a folder has no `.git` of its own, so it reads as gitNone. A submodule
// (its `.git` is a gitdir-pointer file) is distinguished from a standalone clone (a
// `.git` directory) but probed the same way — `git -C` works inside either.
func gitProbe(dir string) (kind gitKind, remote, branch string) {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	if err != nil {
		return gitNone, "", ""
	}
	if info.IsDir() {
		kind = gitRepo
	} else {
		kind = gitSubmodule
	}

	remote = normalizeGitRemote(gitOutput(dir, "remote", "get-url", "origin"))
	if b := gitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD"); b != "" && b != "HEAD" {
		branch = b
	}
	return kind, remote, branch
}

// gitCheckedOutBranch returns the branch currently checked out in dir, or "" when dir
// isn't a git checkout (no `.git` entry), the HEAD is detached, or git can't be read.
// It's the branch half of gitProbe without the remote lookup, cheap enough for Inspect
// to call per git entry when detecting branch drift.
func gitCheckedOutBranch(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return ""
	}
	if b := gitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD"); b != "" && b != "HEAD" {
		return b
	}
	return ""
}

// isGitCheckout reports whether dir is its own git checkout (has a `.git` entry —
// a directory for a standalone clone, a file for a submodule). The same presence
// test HasUncommittedChanges/gitCheckedOutBranch use, without reading git.
func isGitCheckout(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

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

// FetchResult is one entry's outcome from FetchAll: the fetch error (nil on success) and
// the divergence read back afterwards, so a caller can report what the fetch revealed
// without re-reading git itself.
type FetchResult struct {
	Name string
	Err  error
	Sync GitSync
}

// FetchAll fetches every present git checkout (clone or submodule) among statuses,
// concurrently but capped at maxConcurrentChecks, and reads each one's post-fetch
// divergence. Non-git and not-installed entries are skipped (nothing to fetch). ctx bounds
// the whole batch — a cancel aborts the in-flight fetches. Results are name-sorted so the
// caller's output is deterministic.
func FetchAll(ctx context.Context, statuses []Status) []FetchResult {
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrentChecks)
	var out []FetchResult
	for _, s := range statuses {
		if !s.Addon.IsGitWorkdir() || !s.Present() {
			continue
		}
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
		}(s.Addon.Name, s.FullPath)
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

// gitOutput runs a read-only `git -C dir <args...>` and returns its trimmed stdout,
// or "" on any error (a folder may be a repo with no origin, etc.).
func gitOutput(dir string, args ...string) string {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitEnv is the environment every git subprocess gdaddon spawns runs with. Both settings
// exist to guarantee git can never sit waiting for input gdaddon has no way to supply:
// GIT_TERMINAL_PROMPT=0 makes a repo with uncached credentials fail instead of prompting,
// and GIT_EDITOR=true makes any path that would open an editor (a merge message, say)
// take the empty-editor exit rather than hanging behind the TUI forever.
func gitEnv() []string {
	return append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true")
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

// normalizeGitRemote converts a git origin url into an https tracking url: an scp-form
// `git@host:owner/repo[.git]` becomes `https://host/owner/repo[.git]`; an `https://…`
// remote passes through. Returns "" for an empty/unrecognized value. The Track form's
// NormalizeRepoURL handles any `.git` suffixing at use.
func normalizeGitRemote(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if rest, ok := strings.CutPrefix(raw, "git@"); ok {
		if host, path, found := strings.Cut(rest, ":"); found && host != "" && path != "" {
			return "https://" + host + "/" + strings.TrimPrefix(path, "/")
		}
		return ""
	}
	if strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://") {
		return raw
	}
	return ""
}
