package repoui

import (
	"context"
	"fmt"
	"strings"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repo"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
)

// The per-repo Git submenu: the routine half of working on a checkout — see what changed,
// fetch, pull, commit, push — without leaving the app for a terminal. It is deliberately not
// a git client. Every operation here either succeeds on the boring path or fails having
// changed nothing, and says so; a divergence, a conflict, or a rebase is a decision, and
// decisions belong in a real terminal.

// stageOptions is the commit form's staging toggle, in index order. The default (index 0) is
// the conservative one — see repo.GitCommit for why the distinction is load-bearing.
var stageOptions = []string{"tracked changes (-a)", "all, incl. new files (-A)"}

const stageAllIndex = 1

// RepoMenu builds the Git command hub for one checkout. Each row's Desc reads the repo's
// current local git state (recomputed via the engine on build), so the menu itself answers
// "what shape is this repo in" before you pick anything. PopStop makes it the hub the
// sub-flows (task screens, the commit form) return to. It rebuilds on RefreshMsg so popping
// out of a finished pull doesn't land on rows that still say "3 behind".
func RepoMenu(sh *core.Shared, r repo.Repo) *components.PickerScreen {
	return components.NewPicker(repoItems(r), components.PickerOpts{
		Title:   r.Name,
		Crumb:   "Git",
		PopStop: true,
		Refresh: func(sh *core.Shared, payload any) ([]list.Item, bool) {
			if _, ok := payload.(RefreshMsg); !ok {
				return nil, false
			}
			return repoItems(r), true
		},
	})
}

func repoItems(r repo.Repo) []list.Item {
	name, dir := r.Name, r.Dir
	// Recomputed fresh on every build (open and post-op refresh), so the descriptions reflect
	// the repo's real state without a caller-owned cache to keep current. All local reads.
	sync := repo.GitSyncStatus(dir)
	dirty := repo.HasUncommittedChanges(dir)

	// A task row: run op, stream git's output to the log, refresh the state on success.
	task := func(label, verb string, op func(context.Context, string, repo.Reporter) error) func(*core.Shared) core.Action {
		return func(*core.Shared) core.Action {
			return core.Push(Task(label, verb, dir, op))
		}
	}

	return []list.Item{
		components.Item{
			Name: "⟳ Fetch",
			Desc: "update this repo's remote refs (the whole project: \"f\" on the list)",
			Pick: task("fetching "+name+"…", "fetched", func(ctx context.Context, dir string, report repo.Reporter) error {
				report("fetching %s…", name)
				return repo.GitFetch(ctx, dir)
			}),
		},
		components.Item{
			Name: "◉ Status",
			Desc: statusDesc(dirty),
			Pick: task("reading status…", "", repo.GitStatus),
		},
		components.Item{
			Name: "⇩ Pull",
			Desc: pullDesc(sync),
			Pick: task("pulling "+name+"…", "pulled", repo.GitPull),
		},
		components.Item{
			Name: "⇧ Push",
			Desc: pushDesc(sync),
			Pick: task("pushing "+name+"…", "pushed", repo.GitPush),
		},
		components.Item{
			Name: "✎ Commit",
			Desc: commitDesc(dir),
			Pick: func(sh *core.Shared) core.Action { return core.Push(newCommitForm(r)) },
		},
	}
}

// The row descriptions read the current state, so the menu is a status report in itself. The
// ahead/behind counts carry the same caveat as any marker: they're as fresh as the last
// fetch, which is why Fetch sits at the top of this menu.

func statusDesc(dirty bool) string {
	if dirty {
		return "show the working tree (it has uncommitted changes)"
	}
	return "show the working tree"
}

func pullDesc(sync repo.GitSync) string {
	if sync.Behind > 0 {
		return fmt.Sprintf("fast-forward — %d commit(s) behind origin", sync.Behind)
	}
	return "fast-forward — nothing to pull (as of the last fetch)"
}

func pushDesc(sync repo.GitSync) string {
	if sync.Ahead > 0 {
		return fmt.Sprintf("push %d local commit(s) to origin", sync.Ahead)
	}
	return "nothing to push"
}

func commitDesc(dir string) string {
	changes, err := repo.GitChanges(dir)
	if err != nil || len(changes) == 0 {
		return "working tree is clean"
	}
	return fmt.Sprintf("commit %d changed file(s)", len(changes))
}

// ---------- commit ----------

// newCommitForm asks for the message and what to stage. The staging toggle is not a
// convenience: `git commit -a` stages only *tracked* files, so a file you just created would
// silently miss the commit. Rather than pick a surprise for the user, the form makes the
// choice explicit and the confirm screen shows its consequences.
func newCommitForm(r repo.Repo) *components.FormScreen {
	msgF := components.NewTextField("message", "Message: ", "what changed?")
	stageF := components.NewToggleField("stage", "Stage:   ", stageOptions, "|")

	return components.NewForm(components.FormOpts{
		Crumb: "Commit",
		Fields: []components.FormField{
			components.NewHeading("Commit " + r.Name),
			components.NewSpacer(),
			msgF, stageF,
		},
		Focus: "message",
		Help: []key.Binding{
			core.Hint("field", core.Keys.PrevField, core.Keys.NextField),
			core.Hint("stage", core.Keys.Left, core.Keys.Right),
			core.Hint("commit", core.Keys.Select),
			core.Hint("cancel", core.Keys.Back),
		},
		OnSubmit: func(sh *core.Shared, f *components.FormScreen) core.Action {
			msg := strings.TrimSpace(f.Value("message"))
			if msg == "" {
				return core.Seq(
					core.SetStatusAndLog("a commit message is required"),
					core.Async(f.Focus("message")),
				)
			}
			// The toggle's value comes off the captured field: FormScreen.Value reads text
			// fields only.
			stageAll := stageF.Index() == stageAllIndex

			changes, err := repo.GitChanges(r.Dir)
			if err != nil {
				return core.SeqErr(err, core.Async(f.Focus("message")))
			}
			if len(commitable(changes, stageAll)) == 0 {
				return core.SetStatusAndLog("nothing to commit in this mode")
			}
			return core.Push(newCommitConfirm(r, msg, stageAll))
		},
	})
}

// commitable is the subset of changes the chosen staging mode will actually commit: with
// `-a`, tracked changes only; with `add -A`, everything.
func commitable(changes []repo.GitChange, stageAll bool) []repo.GitChange {
	out := make([]repo.GitChange, 0, len(changes))
	for _, c := range changes {
		if stageAll || !c.Untracked() {
			out = append(out, c)
		}
	}
	return out
}

// newCommitConfirm shows exactly what the commit will contain — and, when the mode excludes
// them, exactly which new files it will leave behind.
func newCommitConfirm(r repo.Repo, msg string, stageAll bool) *components.DialogScreen {
	changes, _ := repo.GitChanges(r.Dir) // re-read: the tree may have moved since the form opened
	return components.CreateConfirmScreen(components.ConfirmSimple{
		// No Crumb: it defaults to "Conf", so the trail reads "Git › Commit › Conf" rather
		// than repeating "Commit" twice.
		Text: commitBody(r, changes, msg, stageAll),
		OnYes: core.Replace(Task("committing "+r.Name+"…", "committed", r.Dir,
			func(ctx context.Context, dir string, report repo.Reporter) error {
				return repo.GitCommit(ctx, dir, msg, stageAll, report)
			})),
	})
}

// maxCommitList caps each file list in the confirm body. A DialogScreen neither scrolls nor
// clips (its SetSize is a no-op), so a repo with a hundred changed files would push the
// status line and help bar off the terminal; the cap is what keeps the box a box.
const maxCommitList = 10

// commitBody renders the confirm text: the files this commit will contain, then — only when
// the mode leaves them out — the untracked files it won't, named so the omission is a choice
// rather than a surprise.
func commitBody(r repo.Repo, changes []repo.GitChange, msg string, stageAll bool) string {
	included := commitable(changes, stageAll)

	head := fmt.Sprintf("Commit %d file(s) in %s", len(included), r.Name)
	if r.Branch != "" {
		head += " on " + r.Branch
	}

	lines := []string{head + ":", ""}
	lines = append(lines, fileLines(included)...)

	if !stageAll {
		var untracked []repo.GitChange
		for _, c := range changes {
			if c.Untracked() {
				untracked = append(untracked, c)
			}
		}
		if len(untracked) > 0 {
			lines = append(lines, "", "Not included — new files, which \"-a\" does not stage.")
			lines = append(lines, "Pick \"all, incl. new files\" to commit these too:", "")
			lines = append(lines, fileLines(untracked)...)
		}
	}

	return strings.Join(append(lines, "", "message: "+msg), "\n")
}

// fileLines renders up to maxCommitList "  XY path" rows, then says how many it left out.
func fileLines(changes []repo.GitChange) []string {
	n := len(changes)
	shown := n
	if shown > maxCommitList {
		shown = maxCommitList
	}
	lines := make([]string, 0, shown+1)
	for _, c := range changes[:shown] {
		lines = append(lines, fmt.Sprintf("  %s  %s", c.Code, c.Path))
	}
	if n > shown {
		lines = append(lines, fmt.Sprintf("  … and %d more", n-shown))
	}
	return lines
}
