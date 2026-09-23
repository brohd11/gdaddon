package addon

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/brohd11/gitstack/repo"
)

// Manifest-side git probes: classify a plugin folder by its `.git` entry and read its
// origin and branch, so Inspect and ScanInstalled can tell clones, submodules and plain
// folders apart.

// gitKind classifies a scanned plugin folder by its `.git` entry.
type gitKind int

const (
	gitNone      gitKind = iota // no .git entry: not its own checkout
	gitRepo                     // .git is a directory: a standalone clone
	gitSubmodule                // .git is a file: a parent-managed submodule
)

// gitProbe classifies dir by its own `.git` entry (a directory for a clone, a file for a
// submodule) and returns its origin (scp form as https) and branch ("" when detached). A
// folder without its own `.git` is gitNone, so it never reports the project repo's remote.
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

	remote = normalizeGitRemote(repo.GitOutput(dir, "remote", "get-url", "origin"))
	branch = repo.CurrentBranch(dir) // "" on a detached HEAD, which is what this reports too
	return kind, remote, branch
}

// isGitCheckout reports whether dir has its own `.git` entry, without running git.
func isGitCheckout(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// normalizeGitRemote converts an origin url to https (`git@host:owner/repo` becomes
// `https://host/owner/repo`); "" if unrecognized.
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
