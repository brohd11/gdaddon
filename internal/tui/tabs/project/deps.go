package project

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// depsResolveTimeout caps the batch of release-listing fetches so a slow host
// can't leave the Get-dependencies action pending forever.
const depsResolveTimeout = 30 * time.Second

// plannedDep is one dependency the plan will add to the manifest: a resolved
// install url + tag (tag empty for a tagless repo-only add). Resolving (the network
// asset lookup) happens before the confirm; committing it is pure manifest IO.
type plannedDep struct {
	name string
	url  string
	tag  string
}

// depPlan is the resolved outcome of reading an addon's declared dependencies,
// computed off the UI thread and shown in the confirm before anything is written.
// add: entries to be created; satisfied: already present at a sufficient tag;
// skipped: present-but-stale, ambiguous, or unresolvable — surfaced, never changed.
type depPlan struct {
	add       []plannedDep
	satisfied int
	skipped   []string
	err       error
}

// suppressKey toggles suppression on the highlighted dependency row.
var suppressKey = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "suppress"))

// newDepsScreen is the per-addon Dependencies hub: it lists every dependency the addon
// declares (from the cached, install-aware appctx.DepStatuses) with its status, a lead
// "Add all missing" row, and per-row add/suppress. It's a Receiver (Refresh) so any
// add/suppress broadcast rebuilds it in place, and a PopStop boundary so the per-dep
// sub-actions return here.
func newDepsScreen(st addon.Status, sh *core.Shared) *components.PickerScreen {
	return components.NewPicker(depsItems(st, sh), components.PickerOpts{
		Crumb:   "Dependencies",
		Title:   st.Addon.Name,
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

// newDepActionSubmenu is the per-dep action list: "Add to manifest" (only when the dep
// is missing and not suppressed) plus a suppress/unsuppress toggle. It is not a PopStop
// boundary, so its actions PopTo back to the Dependencies screen.
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

// suppressToggle adds or removes repoID from the declaring addon's suppress_deps list,
// refreshes the project cache synchronously (so every Receiver reads fresh state), and
// broadcasts ProjectDirty — the Dependencies screen rebuilds its rows and the project
// list re-evaluates its missing-deps marker. It carries no navigation, so the `s`
// keypress stays on the screen while the submenu path adds its own PopTo.
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

// newAddOneDepLoading resolves and adds a single dependency to the manifest off the UI
// thread (a tagged dep needs a network asset lookup), then refreshes and PopTo's back to
// the Dependencies screen. Mirrors the batch resolveDepsCmd/commitDeps for one dep.
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
			name := addon.DeriveName(d.RepoURL)
			if d.Tag == "" {
				if err := addon.AddEntry(manifestPath, name, addon.NormalizeRepoURL(d.RepoURL), ""); err != nil {
					return addResult{err: err}
				}
				return addResult{status: fmt.Sprintf("added %s (no version)", name)}
			}
			ctx, cancel := context.WithTimeout(parent, depsResolveTimeout)
			defer cancel()
			asset, ok := addon.ResolveDepAsset(ctx, d)
			if !ok {
				return addResult{err: fmt.Errorf("no asset for %s %s", d.RepoID, d.Tag)}
			}
			if err := addon.AddEntryFull(manifestPath, addon.Addon{Name: name, URL: asset.URL, Tag: d.Tag}); err != nil {
				return addResult{err: err}
			}
			return addResult{status: fmt.Sprintf("added %s %s", name, d.Tag)}
		}
	}
}

// newGetDepsLoading reads the addon's declared plugin.cfg dependencies and resolves
// each (off the UI thread, with the network asset lookups) into a depPlan. It then
// opens a confirm listing what will be added; nothing is written until confirmed.
// Install All performs the actual install afterward.
func newGetDepsLoading(st addon.Status, sh *core.Shared) *components.LoadingScreen {
	c := appctx.Of(sh)
	manifestPath, addonDir, name := c.ManifestPath, st.FullPath, st.Addon.Name

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
	return components.NewLoadingScreen(name, "resolving dependencies…", resolveDepsCmd(manifestPath, addonDir, name), onResult)
}

func resolveDepsCmd(manifestPath, addonDir, name string) func(context.Context) tea.Cmd {
	return func(parent context.Context) tea.Cmd {
		return func() tea.Msg {
			deps, err := addon.Dependencies(addonDir)
			if err != nil {
				return depPlan{err: err}
			}
			entries, err := addon.Parse(manifestPath)
			if err != nil {
				return depPlan{err: err}
			}
			byRepo := addon.IndexByRepo(entries)
			suppressed := suppressSet(entries, name)

			ctx, cancel := context.WithTimeout(parent, depsResolveTimeout)
			defer cancel()

			var plan depPlan
			for _, d := range deps {
				// A suppressed (optional) dependency is never planned for add.
				if suppressed[d.RepoID] {
					continue
				}
				// Tagless dependency: any present copy satisfies it; when absent, plan to
				// add the repo version-less (Install All clones it; user can pin later).
				if d.Tag == "" {
					if _, ok := byRepo[d.RepoID]; ok {
						plan.satisfied++
						continue
					}
					plan.add = append(plan.add, plannedDep{name: addon.DeriveName(d.RepoURL), url: addon.NormalizeRepoURL(d.RepoURL)})
					continue
				}

				if existing, ok := byRepo[d.RepoID]; ok {
					if sat, _ := d.SatisfiedByTag(existing.Tag); sat {
						plan.satisfied++
					} else {
						plan.skipped = append(plan.skipped, fmt.Sprintf("%s has %s, needs %s", d.RepoID, tagOrNone(existing.Tag), d.Tag))
					}
					continue
				}
				asset, ok := addon.ResolveDepAsset(ctx, d)
				if !ok {
					plan.skipped = append(plan.skipped, d.RepoID+" (no asset for "+d.Tag+")")
					continue
				}
				plan.add = append(plan.add, plannedDep{name: addon.DeriveName(d.RepoURL), url: asset.URL, tag: d.Tag})
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

// suppressSet builds the suppressed-RepoID lookup for the addon named name from parsed
// manifest entries (read fresh so a just-suppressed dep is honored immediately).
func suppressSet(entries []addon.Addon, name string) map[string]bool {
	set := map[string]bool{}
	for _, a := range entries {
		if a.Name == name {
			for _, id := range a.SuppressDeps {
				set[id] = true
			}
			break
		}
	}
	return set
}

// newGetDepsConfirm lists the dependencies that will be added (and notes how many
// are already satisfied / skipped), committing them only on confirm (OnYesLamda defers
// the manifest writes to the Yes press).
func newGetDepsConfirm(name, manifestPath string, plan depPlan) *components.DialogScreen {
	return components.CreateConfirmScreen(components.ConfirmSimple{
		Crumb:      "Add Dependencies",
		Text:       depsConfirmBody(name, plan),
		OnYesLamda: func(sh *core.Shared) core.Action { return commitDeps(sh, name, manifestPath, plan) },
	})
}

func depsConfirmBody(name string, plan depPlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Add %d dependenc%s for %s\n", len(plan.add), plural(len(plan.add), "y", "ies"), name)
	for _, p := range plan.add {
		tag := p.tag
		if tag == "" {
			tag = "(no version)"
		}
		fmt.Fprintf(&b, "\n  ▸ %s   %s", p.name, tag)
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

// commitDeps writes the planned entries to the manifest (pure IO — the assets were
// resolved before the confirm), refreshes the project cache, and broadcasts ProjectDirty
// so both the Dependencies screen (rebuilt via its Refresh) and the project list marker
// update, then PopTo's back to the Dependencies screen to show the new statuses.
func commitDeps(sh *core.Shared, name, manifestPath string, plan depPlan) core.Action {
	added, failed := 0, 0
	for _, p := range plan.add {
		// AddEntryFull with an empty tag behaves like a bare AddEntry.
		if err := addon.AddEntryFull(manifestPath, addon.Addon{Name: p.name, URL: p.url, Tag: p.tag}); err != nil {
			failed++
			continue
		}
		added++
	}
	appctx.Of(sh).RefreshProject()
	status := fmt.Sprintf("%s: added %d dependenc%s", name, added, plural(added, "y", "ies"))
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

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
