// Package repoui holds the domain-neutral git-viewing screens built on bubblestack and the
// sibling repo engine: a single-repo streaming task, the per-repo command submenu (status /
// fetch / pull / commit / push), and the all-repos batch menu. It names no manifest, addon,
// or app type — a consumer hands it repo.Repo values (and, for the batch menu, scope
// providers) and reacts to the RefreshMsg it broadcasts. gdaddon composes these behind a
// thin adapter; a plain repo viewer would wire them to a directory scan.
//
// The contract mirrors the engine's: pull is fast-forward-only, and any repo that would need
// a decision (a divergence, a conflict) fails with git's own words in the log and nothing is
// changed. Nothing here merges, rebases, or resolves — that belongs in a real terminal.
package repoui

import (
	"context"
	"strings"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repo"
)

// RefreshMsg is broadcast (via core.PropagateAll) after a git operation changes a checkout,
// so any screen showing git state can settle. repoui's own menus rebuild their rows on it;
// a consumer that caches git state (like gdaddon's project list) handles it to recompute
// dirty / ahead / behind. It carries no payload — it's a pure "reload yourself" marker.
type RefreshMsg struct{}

// Task runs one git operation on one repo, streaming its output into the shared log (report's
// lines land there and the pane reveals itself). It's a *stay* task: the whole point is to
// read what git said — especially when it refused — so the screen holds until esc rather than
// yanking the output away on completion.
//
// verb is the past tense for the success status ("pulled"); an empty verb means the operation
// only reports (Status) and gets no status line of its own. On success it broadcasts
// RefreshMsg so any list's markers settle.
func Task(label, verb, dir string, op func(context.Context, string, repo.Reporter) error) *components.TaskScreen {
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
			return core.PropagateAll(RefreshMsg{})
		}
		return core.Seq(
			core.SetStatus(verb),
			core.PropagateAll(RefreshMsg{}),
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

// pastTense renders an operation verb for a completion summary ("pull" → "pulled").
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

// titleWord capitalizes the first letter of an ASCII verb ("pull" → "Pull").
func titleWord(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
