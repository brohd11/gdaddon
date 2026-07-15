package project

import (
	"context"
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"
	gitflow "gdaddon/internal/tui/flows/git"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
)

// The per-addon Git submenu: the routine half of working on an addon you develop — see what
// changed, fetch, pull, commit, push — without leaving gdaddon for a terminal. It is
// deliberately not a git client. Every operation here either succeeds on the boring path or
// fails having changed nothing, and says so; a divergence, a conflict, or a rebase is a
// decision, and decisions belong in a real terminal ("t" opens one at the addon's folder).

// stageOptions is the commit form's staging toggle, in index order. The default (index 0) is
// the conservative one — see addon.GitCommit for why the distinction is load-bearing.
var stageOptions = []string{"tracked changes (-a)", "all, incl. new files (-A)"}

const stageAllIndex = 1

// newGitSubmenu builds the Git command hub for a present git checkout. Each row's Desc is
// read from the cached local git state (part 1's Ctx.GitSync / Ctx.GitDirty), so the menu
// itself answers "what shape is this repo in" before you pick anything. PopStop makes it the
// hub the sub-flows (task screens, the commit form) return to.
func newGitSubmenu(st addon.Status, sh *core.Shared) *components.PickerScreen {
	return components.NewPicker(gitItems(st, sh), components.PickerOpts{
		Title:   st.Addon.Name,
		Crumb:   "Git",
		PopStop: true,
		// Rebuild the rows (and their state descriptions) once an operation reports back, so
		// popping out of a finished pull doesn't land on rows that still say "3 behind".
		Refresh: func(sh *core.Shared, payload any) ([]list.Item, bool) {
			if _, ok := payload.(appctx.GitRefresh); !ok {
				return nil, false
			}
			return gitItems(st, sh), true
		},
	})
}

func gitItems(st addon.Status, sh *core.Shared) []list.Item {
	c := appctx.Of(sh)
	name := st.Addon.Name
	sync, dirty := c.GitSync[name], c.GitDirty[name]
	dir := st.FullPath

	// A task row: run op, stream git's output to the log, refresh the state on success.
	task := func(label, verb string, op func(context.Context, string, addon.Reporter) error) func(*core.Shared) core.Action {
		return func(*core.Shared) core.Action {
			return core.Push(gitflow.Task(label, verb, dir, op))
		}
	}

	return []list.Item{
		components.Item{
			Name: "⟳ Fetch",
			Desc: "update this repo's remote refs (the whole project: \"f\" on the list)",
			Pick: task("fetching "+name+"…", "fetched", func(ctx context.Context, dir string, report addon.Reporter) error {
				report("fetching %s…", name)
				return addon.GitFetch(ctx, dir)
			}),
		},
		components.Item{
			Name: "◉ Status",
			Desc: statusDesc(dirty),
			Pick: task("reading status…", "", addon.GitStatus),
		},
		components.Item{
			Name: "⇩ Pull",
			Desc: pullDesc(sync),
			Pick: task("pulling "+name+"…", "pulled", addon.GitPull),
		},
		components.Item{
			Name: "⇧ Push",
			Desc: pushDesc(sync),
			Pick: task("pushing "+name+"…", "pushed", addon.GitPush),
		},
		components.Item{
			Name: "✎ Commit",
			Desc: commitDesc(dir),
			Pick: func(sh *core.Shared) core.Action { return core.Push(newCommitForm(st)) },
		},
	}
}

// The row descriptions read the cached state, so the menu is a status report in itself. The
// ahead/behind counts carry the same caveat as the row markers: they're as fresh as the last
// fetch, which is why Fetch sits at the top of this menu.

func statusDesc(dirty bool) string {
	if dirty {
		return "show the working tree (it has uncommitted changes)"
	}
	return "show the working tree"
}

func pullDesc(sync addon.GitSync) string {
	if sync.Behind > 0 {
		return fmt.Sprintf("fast-forward — %d commit(s) behind origin", sync.Behind)
	}
	return "fast-forward — nothing to pull (as of the last fetch)"
}

func pushDesc(sync addon.GitSync) string {
	if sync.Ahead > 0 {
		return fmt.Sprintf("push %d local commit(s) to origin", sync.Ahead)
	}
	return "nothing to push"
}

func commitDesc(dir string) string {
	changes, err := addon.GitChanges(dir)
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
func newCommitForm(st addon.Status) *components.FormScreen {
	msgF := components.NewTextField("message", "Message: ", "what changed?")
	stageF := components.NewToggleField("stage", "Stage:   ", stageOptions, "|")

	return components.NewForm(components.FormOpts{
		Crumb: "Commit",
		Fields: []components.FormField{
			components.NewHeading("Commit " + st.Addon.Name),
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

			changes, err := addon.GitChanges(st.FullPath)
			if err != nil {
				return core.SeqErr(err, core.Async(f.Focus("message")))
			}
			if len(commitable(changes, stageAll)) == 0 {
				return core.SetStatusAndLog("nothing to commit in this mode")
			}
			return core.Push(newCommitConfirm(st, msg, stageAll))
		},
	})
}

// commitable is the subset of changes the chosen staging mode will actually commit: with
// `-a`, tracked changes only; with `add -A`, everything.
func commitable(changes []addon.GitChange, stageAll bool) []addon.GitChange {
	out := make([]addon.GitChange, 0, len(changes))
	for _, c := range changes {
		if stageAll || !c.Untracked() {
			out = append(out, c)
		}
	}
	return out
}

// newCommitConfirm shows exactly what the commit will contain — and, when the mode excludes
// them, exactly which new files it will leave behind.
func newCommitConfirm(st addon.Status, msg string, stageAll bool) *components.DialogScreen {
	changes, _ := addon.GitChanges(st.FullPath) // re-read: the tree may have moved since the form opened
	return components.CreateConfirmScreen(components.ConfirmSimple{
		// No Crumb: it defaults to "Conf", so the trail reads "Git › Commit › Conf" rather
		// than repeating "Commit" twice.
		Text: commitBody(st, changes, msg, stageAll),
		OnYes: core.Replace(gitflow.Task("committing "+st.Addon.Name+"…", "committed", st.FullPath,
			func(ctx context.Context, dir string, report addon.Reporter) error {
				return addon.GitCommit(ctx, dir, msg, stageAll, report)
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
func commitBody(st addon.Status, changes []addon.GitChange, msg string, stageAll bool) string {
	included := commitable(changes, stageAll)

	branch := st.LiveBranch
	if branch == "" {
		branch = st.Addon.Tag
	}
	head := fmt.Sprintf("Commit %d file(s) in %s", len(included), st.Addon.Name)
	if branch != "" {
		head += " on " + branch
	}

	lines := []string{head + ":", ""}
	lines = append(lines, fileLines(included)...)

	if !stageAll {
		var untracked []addon.GitChange
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
func fileLines(changes []addon.GitChange) []string {
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
