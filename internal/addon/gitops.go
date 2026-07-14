package addon

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// This file holds the git operations that *change* a repo (pull/push/commit) or stream
// their output to a UI, as opposed to gitscan.go's read-only probes. They exist so the
// routine half of addon development — see what changed, pull, commit, push — can happen
// without leaving gdaddon. They are deliberately not a git client: anything needing a
// decision (a diverged branch, a conflict, a rebase) fails and leaves the repo untouched
// for the user to sort out in a real terminal.

// GitStream runs a git command in dir, relaying its output to report one line at a time as
// it arrives, and returns a non-nil error when git exits non-zero (with git's own last
// words folded in, since that's the part worth reading). stdout and stderr are interleaved
// the way a terminal would show them — git writes progress and errors to stderr, and a
// caller streaming to a log wants both. ctx cancellation kills the subprocess, which is how
// the TUI's task-abort works.
func GitStream(ctx context.Context, dir string, report Reporter, args ...string) error {
	w := &lineWriter{report: report}
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = gitEnv()
	cmd.Stdout = w
	cmd.Stderr = w

	err := cmd.Run()
	w.flush() // a final line with no trailing newline (git's "fatal: …" often has none)
	if err != nil {
		if last := w.last; last != "" {
			return fmt.Errorf("%w: %s", err, last)
		}
		return err
	}
	return nil
}

// GitStatus streams the working tree's state: `status -sb`, the short form with a branch
// header ("## main...origin/main [behind 1]") — the same facts as a full `git status`
// without the paragraphs of hints, which read poorly in a log pane.
func GitStatus(ctx context.Context, dir string, report Reporter) error {
	return GitStream(ctx, dir, report, "status", "-sb")
}

// GitPull fast-forwards the checkout to its upstream. --ff-only is the whole point: when
// the branch has diverged (local commits *and* new upstream ones) git aborts and changes
// nothing, rather than starting a merge that could leave conflict markers in the working
// tree — or block on an editor — with the user still inside a TUI. Reconciling a divergence
// is a decision, so it belongs in a terminal.
func GitPull(ctx context.Context, dir string, report Reporter) error {
	return GitStream(ctx, dir, report, "pull", "--ff-only")
}

// GitPush pushes the current branch to its upstream. A branch with no upstream fails here
// (git asks for `--set-upstream`), which is the intended outcome: that's a decision about
// where the branch lives, not a routine push.
func GitPush(ctx context.Context, dir string, report Reporter) error {
	return GitStream(ctx, dir, report, "push")
}

// GitCommit commits the working tree with message. stageAll decides what "all" means — the
// distinction the commit form makes the user choose, because git's own answer is a trap:
//
//   - false: `commit -a` stages modifications and deletions to *tracked* files. A file you
//     just created is untracked, so it is NOT committed.
//   - true: `add -A` first, so new files are included too.
//
// The message is passed as an exec argument, never through a shell, so quotes, newlines,
// and `$` in it need no escaping.
func GitCommit(ctx context.Context, dir, message string, stageAll bool, report Reporter) error {
	if stageAll {
		if err := GitStream(ctx, dir, report, "add", "-A"); err != nil {
			return err
		}
	}
	return GitStream(ctx, dir, report, "commit", "-a", "-m", message)
}

// lineWriter turns a subprocess's byte stream into whole lines for a Reporter. git delimits
// its progress output ("Receiving objects:  47%…") with carriage returns rather than
// newlines, so both count as line breaks — otherwise a clone's entire progress meter would
// arrive as one unreadable line. It keeps the last line reported so a failing command can
// quote git's parting words in its error.
type lineWriter struct {
	report Reporter
	buf    []byte
	last   string
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexAny(w.buf, "\r\n")
		if i < 0 {
			break
		}
		w.emit(string(w.buf[:i]))
		w.buf = w.buf[i+1:]
	}
	return len(p), nil
}

// flush emits whatever partial line is left once the command has exited.
func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		w.emit(string(w.buf))
		w.buf = nil
	}
}

func (w *lineWriter) emit(line string) {
	line = strings.TrimRight(line, " \t")
	if line == "" {
		return
	}
	w.last = line
	// "%s" rather than the line as a format string: git's output can contain a literal %.
	w.report("%s", line)
}
