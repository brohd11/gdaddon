package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/source"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/goutil/strutil"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

// plannedDep is one dependency the plan will add: resolved url, tag (empty for repo-only
// or default-branch clones) and kind. Committing it is manifest IO only.
type plannedDep struct {
	name string
	url  string
	tag  string
	kind addon.Kind
}

// depPlan is the resolved result shown before writing: entries to add, satisfied ones,
// and skipped ones (stale, ambiguous or unresolvable), which are never changed.
type depPlan struct {
	add       []plannedDep
	satisfied int
	skipped   []string
	err       error
}

// suppressKey toggles suppression on the highlighted dependency row.
var suppressKey = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "suppress"))

// newDepsScreen is an addon's Dependencies hub: each declared dependency with its status,
// "Add all missing", and per-row add and suppress. It rebuilds on refresh broadcasts and
// is a PopStop hub.
func newDepsScreen(st addon.Status, sh *core.Shared) *components.PickerScreen {
	return components.NewPicker(depsItems(st, sh), components.PickerOpts{
		Crumb:   "Dependencies",
		Title:   st.Addon.Label(),
		PopStop: true,
		Help:    []key.Binding{suppressKey},
		Refresh: func(sh *core.Shared, payload any) ([]list.Item, bool) {
			if _, ok := payload.(appctx.ProjectDirty); !ok {
				return nil, false
			}
			return depsItems(st, sh), true
		},
	})
}

// depsItems builds the Dependencies list: a lead "Add all missing" row (when any
// non-suppressed dep is absent from the manifest) followed by one row per declared dep.
func depsItems(st addon.Status, sh *core.Shared) []list.Item {
	statuses := appctx.Of(sh).DepStatuses[st.Addon.Name]
	var items []list.Item
	if n := addableCount(statuses); n > 0 {
		items = append(items, components.Item{
			Name: fmt.Sprintf("＋ Add all missing (%d)", n),
			Desc: "add every missing (non-suppressed) dependency to the manifest (Install All to install)",
			Pick: func(sh *core.Shared) core.Action { return core.Push(newGetDepsLoading(st, sh)) },
		})
	}
	for _, ds := range statuses {
		items = append(items, depRow(st, ds))
	}
	return items
}

// addableCount is how many declared deps "Add all missing" would add: missing from the
// manifest and not suppressed.
func addableCount(statuses []addon.DepStatus) int {
	n := 0
	for _, ds := range statuses {
		if ds.State == addon.DepMissing && !ds.Suppressed {
			n++
		}
	}
	return n
}

// depRow renders one dependency: its repo id + status tag, opening a small add/suppress
// submenu on enter and toggling suppression on `s` (Item.Keys).
func depRow(st addon.Status, ds addon.DepStatus) components.Item {
	return components.Item{
		Name:   ds.Dep.RepoID + "  " + depStatusTag(ds),
		Desc:   depRowDesc(ds),
		Filter: ds.Dep.RepoID,
		Pick:   func(sh *core.Shared) core.Action { return core.Push(newDepActionSubmenu(st, ds)) },
		Keys: func(sh *core.Shared, k string) (core.Action, bool) {
			if core.MatchKey(k, suppressKey) {
				return suppressToggle(sh, st.Addon.Name, ds.Dep.RepoID), true
			}
			return core.Action{}, false
		},
	}
}

// depStatusTag is the bracketed status shown after a dep's repo id.
func depStatusTag(ds addon.DepStatus) string {
	if ds.Suppressed {
		return "[suppressed]"
	}
	switch ds.State {
	case addon.DepInstalled:
		return "[installed]"
	case addon.DepNotInstalled:
		return "[not installed]"
	case addon.DepOutdated:
		return "[outdated]"
	default:
		return "[missing]"
	}
}

// depRowDesc summarizes the required vs. locally-recorded version.
func depRowDesc(ds addon.DepStatus) string {
	req := ds.Dep.Tag
	if req == "" {
		req = "any version"
	}
	desc := "needs " + req
	if ds.LocalTag != "" {
		desc += " · have " + ds.LocalTag
	}
	return desc
}

// newDepActionSubmenu offers "Add to manifest" (when missing and not suppressed) and a
// suppress toggle, returning to the Dependencies screen.
func newDepActionSubmenu(st addon.Status, ds addon.DepStatus) *components.PickerScreen {
	var items []list.Item
	if ds.State == addon.DepMissing && !ds.Suppressed {
		items = append(items, components.Item{
			Name: "＋ Add to manifest",
			Desc: "resolve and add just this dependency (Install All to install)",
			Pick: func(sh *core.Shared) core.Action { return core.Replace(newAddOneDepLoading(st, ds.Dep, sh)) },
		})
	}
	supName, supDesc := "⊘ Suppress", "ignore this dependency — drop it from the warning"
	if ds.Suppressed {
		supName, supDesc = "⦿ Unsuppress", "resume warning about this dependency"
	}
	items = append(items, components.Item{
		Name: supName,
		Desc: supDesc,
		Pick: func(sh *core.Shared) core.Action {
			return core.Seq(suppressToggle(sh, st.Addon.Name, ds.Dep.RepoID), core.PopTo())
		},
	})
	return components.NewPicker(items, components.PickerOpts{
		Crumb: "Dependency",
		Title: ds.Dep.RepoID,
	})
}

// suppressToggle adds or removes repoID from the addon's suppress_deps, refreshes the cache
// and broadcasts ProjectDirty. It does no navigation.
func suppressToggle(sh *core.Shared, name, repoID string) core.Action {
	c := appctx.Of(sh)
	next, added := toggleID(currentSuppress(c, name), repoID)
	if err := addon.SetSuppressDeps(c.ManifestPath, name, next); err != nil {
		return core.StatusErr(err)
	}
	c.RefreshProject()
	verb := "unsuppressed"
	if added {
		verb = "suppressed"
	}
	return core.Seq(
		core.SetStatus(verb+" "+repoID),
		core.PropagateAll(appctx.ProjectDirty{}),
	)
}

// currentSuppress reads the declaring addon's current suppress_deps from the freshly
// parsed manifest cache.
func currentSuppress(c *appctx.Ctx, name string) []string {
	for _, a := range c.ProjectAddons {
		if a.Name == name {
			return a.SuppressDeps
		}
	}
	return nil
}

// toggleID removes id from ids if present (added=false) or appends it (added=true),
// returning a fresh slice.
func toggleID(ids []string, id string) (next []string, added bool) {
	for _, x := range ids {
		if x == id {
			continue
		}
		next = append(next, x)
	}
	if len(next) == len(ids) {
		return append(next, id), true
	}
	return next, false
}

// addResult carries a single-dep add's outcome back from the loading screen.
type addResult struct {
	status string
	err    error
}

// newAddOneDepLoading resolves and adds one dependency off the UI thread, then returns to
// the Dependencies screen.
func newAddOneDepLoading(st addon.Status, d addon.Dependency, sh *core.Shared) *components.LoadingScreen {
	manifestPath := appctx.Of(sh).ManifestPath
	onResult := func(sh *core.Shared, msg tea.Msg) core.Action {
		res, ok := msg.(addResult)
		if !ok {
			return core.Action{}
		}
		if res.err != nil {
			return core.SeqErr(res.err, core.PopTo())
		}
		appctx.Of(sh).RefreshProject()
		return core.Seq(core.SetStatusAndLog(res.status), core.PropagateAll(appctx.ProjectDirty{}), core.PopTo())
	}
	return components.NewLoadingScreen(d.RepoID, "resolving "+d.RepoID+"…", resolveOneDepCmd(manifestPath, d), onResult)
}

func resolveOneDepCmd(manifestPath string, d addon.Dependency) func(context.Context) tea.Cmd {
	return func(parent context.Context) tea.Cmd {
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(parent, addon.DepsResolveTimeout)
			defer cancel()
			entry, added, err := addon.AddDepEntry(ctx, manifestPath, d, true)
			if err != nil {
				return addResult{err: err}
			}
			if !added {
				return addResult{err: fmt.Errorf("no asset for %s %s", d.RepoID, d.Tag)}
			}
			return addResult{status: fmt.Sprintf("added %s %s", entry.Name, addon.DepLabel(entry.Kind, entry.Tag))}
		}
	}
}

// newGetDepsLoading resolves the addon's declared dependencies into a depPlan off the UI
// thread and opens a confirm; nothing is written before it.
func newGetDepsLoading(st addon.Status, sh *core.Shared) *components.LoadingScreen {
	c := appctx.Of(sh)
	manifestPath, name := c.ManifestPath, st.Addon.Name

	onResult := func(sh *core.Shared, msg tea.Msg) core.Action {
		plan, ok := msg.(depPlan)
		if !ok {
			return core.Action{}
		}
		if plan.err != nil {
			return core.SeqErr(plan.err, core.Pop())
		}
		// Nothing addable (all satisfied / only skipped): no confirm, just report.
		if len(plan.add) == 0 {
			return core.Seq(core.SetStatusAndLog(plan.nothingToAdd(name)), core.Pop())
		}
		return core.Replace(newGetDepsConfirm(name, manifestPath, plan))
	}
	return components.NewLoadingScreen(name, "resolving dependencies…",
		resolveDepsCmd(manifestPath, c.ProjectRoot, st.Addon), onResult)
}

// resolveDepsCmd builds a depPlan off the UI thread. Classification is addon.PlanDeps (the
// shared matching, including the upstream-rename fallback); this adds the network part,
// resolving each addable dependency's asset.
func resolveDepsCmd(manifestPath, projectRoot string, a addon.Addon) func(context.Context) tea.Cmd {
	return func(parent context.Context) tea.Cmd {
		return func() tea.Msg {
			entries, err := addon.Parse(manifestPath)
			if err != nil {
				return depPlan{err: err}
			}
			// Plan against the freshly parsed entry, which has the latest SuppressDeps; fall back to
			// the given one if it left the manifest.
			target := a
			if fresh, ok := addon.IndexByName(entries)[a.Name]; ok {
				target = fresh
			}
			classified, err := addon.PlanDeps(target, projectRoot, entries)
			if err != nil {
				return depPlan{err: err}
			}

			ctx, cancel := context.WithTimeout(parent, addon.DepsResolveTimeout)
			defer cancel()

			plan := depPlan{satisfied: len(classified.Satisfied)}
			for _, s := range classified.Stale {
				plan.skipped = append(plan.skipped,
					fmt.Sprintf("%s has %s, needs %s", s.Dep.RepoID, tagOrNone(s.Recorded), s.Dep.Tag))
			}
			for _, d := range classified.Add {
				// Only a tagged package has a release asset to resolve. d is reassigned:
				// an `@latest` spec resolves to the tag the confirm shows and commitDeps writes.
				var asset source.Asset
				if d.Tag != "" && !d.IsClone() {
					var ok bool
					if d, asset, ok = addon.ResolveDepAsset(ctx, d); !ok {
						plan.skipped = append(plan.skipped, d.RepoID+" (no asset for "+d.Tag+")")
						continue
					}
				}
				e := addon.DepEntry(d, asset, true)
				plan.add = append(plan.add, plannedDep{name: e.Name, url: e.URL, tag: e.Tag, kind: e.Kind})
			}
			return plan
		}
	}
}

// nothingToAdd is the status shown when the plan has no entries to add.
func (p depPlan) nothingToAdd(name string) string {
	if p.satisfied+len(p.skipped) == 0 {
		return name + ": no dependencies declared"
	}
	return fmt.Sprintf("%s deps: nothing to add (%d satisfied, %d skipped)", name, p.satisfied, len(p.skipped))
}

// newGetDepsConfirm lists what will be added (and how many are satisfied or skipped),
// writing only on confirm.
func newGetDepsConfirm(name, manifestPath string, plan depPlan) *components.DialogScreen {
	return components.CreateConfirmScreen(components.ConfirmSimple{
		Crumb:       "Add Dependencies",
		Text:        depsConfirmBody(name, plan),
		OnYesLambda: func(sh *core.Shared) core.Action { return commitDeps(sh, name, manifestPath, plan) },
	})
}

func depsConfirmBody(name string, plan depPlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Add %d dependenc%s for %s\n", len(plan.add), strutil.Plural(len(plan.add), "y", "ies"), name)
	for _, p := range plan.add {
		fmt.Fprintf(&b, "\n  ▸ %s   %s", p.name, addon.DepLabel(p.kind, p.tag))
	}
	var notes []string
	if plan.satisfied > 0 {
		notes = append(notes, fmt.Sprintf("%d satisfied", plan.satisfied))
	}
	if len(plan.skipped) > 0 {
		notes = append(notes, fmt.Sprintf("%d skipped", len(plan.skipped)))
	}
	if len(notes) > 0 {
		fmt.Fprintf(&b, "\n\n%s", strings.Join(notes, " · "))
	}
	return b.String()
}

// commitDeps writes the planned entries, refreshes the cache, broadcasts ProjectDirty and
// returns to the Dependencies screen.
func commitDeps(sh *core.Shared, name, manifestPath string, plan depPlan) core.Action {
	added, failed := 0, 0
	for _, p := range plan.add {
		// AddEntryFull with an empty tag behaves like a bare AddEntry. Dependency marks
		// the entry's provenance so it can later flag as an unused dependency.
		if err := addon.AddEntryFull(manifestPath, addon.Addon{Name: p.name, URL: p.url, Tag: p.tag, Kind: p.kind, Dependency: true}); err != nil {
			failed++
			continue
		}
		added++
	}
	appctx.Of(sh).RefreshProject()
	status := fmt.Sprintf("%s: added %d dependenc%s", name, added, strutil.Plural(added, "y", "ies"))
	if failed > 0 {
		status += fmt.Sprintf(" (%d failed)", failed)
	}
	return core.Seq(
		core.SetStatusAndLog(status),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.PopTo(),
	)
}

func tagOrNone(tag string) string {
	if tag == "" {
		return "no tag"
	}
	return tag
}
