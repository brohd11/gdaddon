package addon

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		if manifestDepth(base, path) > maxDepth {
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
