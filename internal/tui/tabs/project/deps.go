package project

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

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
			return core.Seq(core.SetStatusAndLog("error: "+plan.err.Error()), core.Pop())
		}
		// Nothing addable (all satisfied / only skipped): no confirm, just report.
		if len(plan.add) == 0 {
			return core.Seq(core.SetStatusAndLog(plan.nothingToAdd(name)), core.Pop())
		}
		return core.Replace(newGetDepsConfirm(name, manifestPath, plan))
	}
	return components.NewLoadingScreen(name, "resolving dependencies…", resolveDepsCmd(manifestPath, addonDir), onResult)
}

func resolveDepsCmd(manifestPath, addonDir string) tea.Cmd {
	return func() tea.Msg {
		deps, err := addon.Dependencies(addonDir)
		if err != nil {
			return depPlan{err: err}
		}
		entries, err := addon.Parse(manifestPath)
		if err != nil {
			return depPlan{err: err}
		}
		byRepo := make(map[string]addon.Addon, len(entries))
		for _, e := range entries {
			if id, err := source.RepoID(e.URL); err == nil {
				byRepo[id] = e
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), depsResolveTimeout)
		defer cancel()

		var plan depPlan
		for _, d := range deps {
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
			asset, ok := resolveDepAsset(ctx, d)
			if !ok {
				plan.skipped = append(plan.skipped, d.RepoID+" (no asset for "+d.Tag+")")
				continue
			}
			plan.add = append(plan.add, plannedDep{name: addon.DeriveName(d.RepoURL), url: asset.URL, tag: d.Tag})
		}
		return plan
	}
}

// nothingToAdd is the status shown when the plan has no entries to add.
func (p depPlan) nothingToAdd(name string) string {
	if p.satisfied+len(p.skipped) == 0 {
		return name + ": no dependencies declared"
	}
	return fmt.Sprintf("%s deps: nothing to add (%d satisfied, %d skipped)", name, p.satisfied, len(p.skipped))
}

// newGetDepsConfirm lists the dependencies that will be added (and notes how many
// are already satisfied / skipped), committing them only on confirm.
func newGetDepsConfirm(name, manifestPath string, plan depPlan) *components.ConfirmScreen {
	return &components.ConfirmScreen{
		Crumb:  core.HeaderTitle(name, "", "Get dependencies"),
		Render: func(sh *core.Shared) string { return sh.Box(depsConfirmBody(name, plan)) },
		OnYes:  func(sh *core.Shared) core.Action { return commitDeps(name, manifestPath, plan) },
		Help:   confirmHelp,
	}
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
// resolved before the confirm) and broadcasts ProjectDirty so the list reloads.
func commitDeps(name, manifestPath string, plan depPlan) core.Action {
	added, failed := 0, 0
	for _, p := range plan.add {
		var err error
		if p.tag == "" {
			err = addon.AddEntry(manifestPath, p.name, p.url, "")
		} else {
			err = addon.AddEntryWithVersion(manifestPath, p.name, p.url, "", "", p.tag)
		}
		if err != nil {
			failed++
			continue
		}
		added++
	}
	status := fmt.Sprintf("%s: added %d dependenc%s", name, added, plural(added, "y", "ies"))
	if failed > 0 {
		status += fmt.Sprintf(" (%d failed)", failed)
	}
	return core.Seq(
		core.SetStatusAndLog(status),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.ShowTab(appctx.TitleProject),
	)
}

// resolveDepAsset finds the dependency's required release and picks its install
// asset (source.DependencyAsset: the single uploaded build, or the generated source
// archive when none was uploaded; ambiguous multi-upload releases yield ok=false).
func resolveDepAsset(ctx context.Context, d addon.Dependency) (source.Asset, bool) {
	listing, err := source.AvailableVersions(ctx, d.RepoURL)
	if err != nil || listing == nil {
		return source.Asset{}, false
	}
	for _, rel := range listing.Releases {
		if tagEqual(rel.Tag, d.Tag) {
			return source.DependencyAsset(rel)
		}
	}
	return source.Asset{}, false
}

// tagEqual matches a required tag against a release tag, tolerating a leading "v"
// on either side (e.g. "1.2.0" matches "v1.2.0").
func tagEqual(a, b string) bool {
	return a == b || strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
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
