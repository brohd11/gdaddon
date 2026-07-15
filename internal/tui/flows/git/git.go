// Package git holds the git flows shared by more than one tab: the single-repo streaming
// task (used by the per-addon Git submenu on the Project tab) and the project-wide git menu
// (Actions ▸ Git, and "V" on the Project list). It sits at the flows layer because two tabs
// reach it; it names no tab type, only appctx + the addon operations.
//
// The contract is part 2's, unchanged and applied in bulk: pull is fast-forward-only, and any
// repo that would need a decision (a divergence, a conflict) fails with git's own words in the
// log and the batch moves on to the next. Nothing here merges, rebases, or resolves.
package git

import (
	"context"
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
)

// Task runs one git operation on one repo, streaming its output into the shared log (report's
// lines land there and the pane reveals itself). It's a *stay* task: the whole point is to
// read what git said — especially when it refused — so the screen holds until esc rather than
// yanking the output away on completion.
//
// verb is the past tense for the success status ("pulled"); an empty verb means the operation
// only reports (Status) and gets no status line of its own. On success it broadcasts
// GitRefresh so the Project list's markers settle.
func Task(label, verb, dir string, op func(context.Context, string, addon.Reporter) error) *components.TaskScreen {
	run := func(ctx context.Context, sh *core.Shared, report func(string, ...any), done chan<- core.TaskEvent) {
		done <- core.TaskEvent{Done: true, Err: op(ctx, dir, report)}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) core.Action {
		if ev.Err != nil {
			// git's own words are already in the log above; the status line only has to say
			// where to go next. Nothing was changed — --ff-only and a failed push guarantee it.
			return core.SetStatusAndLog(failureLabel(verb) + " — resolve it in a terminal (t)")
		}
		if verb == "" {
			return core.PropagateAll(appctx.GitRefresh{})
		}
		return core.Seq(
			core.SetStatus(verb),
			core.PropagateAll(appctx.GitRefresh{}),
		)
	}
	onDismiss := func(*core.Shared) core.Action { return core.PopTo() } // back to the hub
	return components.NewStayTask(label, "done — esc to go back", run, onDone, onDismiss)
}

// failureLabel names the failed operation from its success verb ("pulled" → "pull failed").
func failureLabel(verb string) string {
	switch verb {
	case "":
		return "git status failed"
	case "fetched":
		return "fetch failed"
	case "pulled":
		return "pull failed"
	case "pushed":
		return "push failed"
	case "committed":
		return "commit failed"
	}
	return verb + " failed"
}

// ---------- all-repos menu ----------

// scope selects which git checkouts the all-repos operations act on. Default is clones — the
// repos you develop. Submodules are parent-managed (pulling one dirties the parent's recorded
// pointer), so acting on them is an opt-in step, not the default.
type scope int

const (
	scopeClones scope = iota
	scopeSubmodules
	scopeAll
)

func (s scope) label() string {
	switch s {
	case scopeSubmodules:
		return "submodules"
	case scopeAll:
		return "all"
	default:
		return "clones"
	}
}

func (s scope) next() scope { return (s + 1) % 3 }

// matches reports whether an entry is a git checkout this scope selects. "all" still means
// git checkouts only — a package is never one, even though targetsFor also guards that.
func (s scope) matches(a addon.Addon) bool {
	switch s {
	case scopeClones:
		return a.IsClone()
	case scopeSubmodules:
		return a.IsSubmodule()
	default:
		return a.IsGitWorkdir()
	}
}

// AllRepos is the project-wide git menu: fetch, pull, or push every checkout in the manifest,
// narrowed by a cycling scope. Opened from Actions ▸ Git and "V" on the Project list. PopStop
// makes it the hub the confirm/task sub-flows return to.
func AllRepos(sh *core.Shared) *components.PickerScreen {
	return screen(sh, scopeClones)
}

// screen builds the menu at a given scope. Cycling scope replaces the screen with a fresh one
// at the next scope, so the rows (and their live counts) rebuild from the new filter.
func screen(sh *core.Shared, sc scope) *components.PickerScreen {
	targets := targetsFor(sh, sc)
	return components.NewPicker(menuItems(sc, targets), components.PickerOpts{
		Title:   "Git — all repos (" + sc.label() + ")",
		Crumb:   "Git all",
		PopStop: true,
		// Rebuild after any git op reports back, so popping out of a finished pull doesn't land
		// on rows that still say "2 behind".
		Refresh: func(sh *core.Shared, payload any) ([]list.Item, bool) {
			if _, ok := payload.(appctx.GitRefresh); !ok {
				return nil, false
			}
			return menuItems(sc, targetsFor(sh, sc)), true
		},
	})
}

func menuItems(sc scope, targets []target) []list.Item {
	behind, ahead := 0, 0
	for _, t := range targets {
		if t.sync.Behind > 0 {
			behind++
		}
		if t.sync.Ahead > 0 {
			ahead++
		}
	}
	n := len(targets)
	noun := sc.label()

	op := func(label, verb string, run func(context.Context, string, addon.Reporter) error, desc string) components.Item {
		return components.Item{
			Name: label,
			Desc: desc,
			Pick: func(sh *core.Shared) core.Action {
				if len(targetsFor(sh, sc)) == 0 {
					return core.SetStatusAndLog("no " + noun + " to " + verb)
				}
				return core.Push(newBatchConfirm(sc, verb, label, run))
			},
		}
	}

	return []list.Item{
		op("⟳ Fetch all", "fetch", func(ctx context.Context, dir string, _ addon.Reporter) error {
			return addon.GitFetch(ctx, dir)
		}, fmt.Sprintf("update remote refs for %d %s", n, noun)),
		op("⇩ Pull all", "pull", addon.GitPull,
			fmt.Sprintf("fast-forward — %d of %d %s behind origin", behind, n, noun)),
		op("⇧ Push all", "push", addon.GitPush,
			fmt.Sprintf("push local commits — %d of %d %s ahead", ahead, n, noun)),
		components.Item{
			Name: "⚙ Scope: " + sc.label(),
			Desc: "enter to cycle → " + sc.next().label(),
			Pick: func(sh *core.Shared) core.Action { return core.Replace(screen(sh, sc.next())) },
		},
	}
}

// ---------- targets ----------

// target is one repo an all-repos operation will act on: its display name, working path, and
// the divergence read (from the part-1 cache) for the confirm annotations.
type target struct {
	name string
	dir  string
	sync addon.GitSync
}

// targetsFor inspects the manifest and returns the present git checkouts in scope. Read fresh
// each time (menu build, confirm, run) rather than captured, since the tree moves under us.
func targetsFor(sh *core.Shared, sc scope) []target {
	c := appctx.Of(sh)
	statuses, _ := addon.Inspect(c.ManifestPath, c.ProjectRoot)
	var out []target
	for _, s := range statuses {
		if !s.Addon.IsGitWorkdir() || !s.Present() || !sc.matches(s.Addon) {
			continue
		}
		out = append(out, target{
			name: s.Addon.Name,
			dir:  s.FullPath,
			sync: c.GitSync[s.Addon.Name],
		})
	}
	return out
}

// ---------- confirm + batch ----------

// newBatchConfirm lists every repo the operation will touch, then runs the batch on confirm.
func newBatchConfirm(sc scope, verb, label string, run func(context.Context, string, addon.Reporter) error) *components.DialogScreen {
	return components.CreateConfirmScreen(components.ConfirmSimple{
		Crumb:  "Confirm",
		Render: func(sh *core.Shared) string { return sh.Box(confirmBody(verb, targetsFor(sh, sc))) },
		OnYesLamda: func(sh *core.Shared) core.Action {
			return core.Replace(newBatchTask(sc, verb, label, run))
		},
	})
}

// maxConfirmList caps the repo list in the confirm body. A DialogScreen neither scrolls nor
// clips (part 2 established this), so an uncapped list would push the chrome off the terminal.
const maxConfirmList = 12

// confirmBody renders the confirm text: how many repos, then each with the divergence that
// makes it worth acting on. A pure function of its inputs, so it's testable and owns the cap.
func confirmBody(verb string, targets []target) string {
	head := fmt.Sprintf("%s %d repo(s)", titleWord(verb), len(targets))
	if verb == "pull" {
		head += " — fast-forward only"
	}
	lines := []string{head + ":", ""}

	shown := len(targets)
	if shown > maxConfirmList {
		shown = maxConfirmList
	}
	// Pad the name column so the annotations line up.
	width := 0
	for _, t := range targets[:shown] {
		if len(t.name) > width {
			width = len(t.name)
		}
	}
	for _, t := range targets[:shown] {
		lines = append(lines, fmt.Sprintf("  %-*s  %s", width, t.name, syncNote(verb, t.sync)))
	}
	if n := len(targets) - shown; n > 0 {
		lines = append(lines, fmt.Sprintf("  … and %d more", n))
	}

	if verb == "pull" {
		lines = append(lines, "", "A repo that has diverged will fail and be skipped; nothing else is touched.")
	}
	return strings.Join(lines, "\n")
}

// titleWord capitalizes the first letter of an ASCII verb ("pull" → "Pull").
func titleWord(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// pastTense renders an operation verb for the completion summary ("pull" → "pulled").
func pastTense(verb string) string {
	switch verb {
	case "fetch":
		return "fetched"
	case "pull":
		return "pulled"
	case "push":
		return "pushed"
	}
	return verb + "ed"
}

// syncNote annotates a repo in the confirm with the count relevant to the operation.
func syncNote(verb string, s addon.GitSync) string {
	switch verb {
	case "pull":
		if s.Behind > 0 {
			return fmt.Sprintf("%d behind origin", s.Behind)
		}
		return "up to date"
	case "push":
		if s.Ahead > 0 {
			return fmt.Sprintf("%d to push", s.Ahead)
		}
		return "nothing to push"
	default:
		return ""
	}
}

// newBatchTask runs verb over every in-scope repo, sequentially, streaming each repo's output
// under its own header. Sequential is deliberate: interleaved output from concurrent pulls is
// unreadable, and reading what git said is the whole point of this screen (the concurrent,
// no-confirm path is the "f" key). ctx is checked between repos so esc abandons the rest.
func newBatchTask(sc scope, verb, label string, op func(context.Context, string, addon.Reporter) error) *components.TaskScreen {
	var done, failed int
	run := func(ctx context.Context, sh *core.Shared, report func(string, ...any), doneCh chan<- core.TaskEvent) {
		targets := targetsFor(sh, sc)
		for i, t := range targets {
			if ctx.Err() != nil {
				report("aborted — %d repo(s) not reached", len(targets)-i)
				break
			}
			if i > 0 {
				report("")
			}
			report("── %s ──", t.name)
			if err := op(ctx, t.dir, report); err != nil {
				report("  %s failed: %v", verb, err)
				failed++
				continue
			}
			done++
		}
		doneCh <- core.TaskEvent{Done: true}
	}
	onDone := func(sh *core.Shared, ev core.TaskEvent) core.Action {
		summary := fmt.Sprintf("%s %d repo(s)", pastTense(verb), done)
		if failed > 0 {
			summary += fmt.Sprintf(" · %d failed", failed)
		}
		return core.Seq(
			core.SetStatusAndLog(summary, failed > 0),
			core.PropagateAll(appctx.GitRefresh{}),
		)
	}
	onDismiss := func(*core.Shared) core.Action { return core.PopTo() }
	return components.NewStayTask(label+"…", "done — esc to go back", run, onDone, onDismiss)
}
