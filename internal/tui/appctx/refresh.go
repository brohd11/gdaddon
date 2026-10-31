package appctx

import (
	"github.com/brohd11/gdaddon/internal/addon"

	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gitstack/repo"

	tea "charm.land/bubbletea/v2"
)

// maxInspect caps concurrent per-addon inspections (each up to three git processes).
const maxInspect = 8

// addonInspectedMsg is one manifest entry's fresh inspection from refresh pass gen,
// broadcast so it reaches the Project root whatever is pushed over it. Hand it to
// ApplyRefreshResult.
type addonInspectedMsg struct {
	gen    int64
	status addon.Status
	dirty  bool
	sync   addon.GitSync
}

// rootDescribedMsg is the project root checkout's fresh state (nil when it is not one).
type rootDescribedMsg struct {
	gen  int64
	root *repo.Repo
}

// beginProject is the synchronous half of a project reload: it re-parses the manifest
// (cheap), keeps each known entry's cached inspection and git markers so rows can show
// them muted, adds a placeholder for new entries, and marks everything Pending.
func (c *Ctx) beginProject() {
	defer c.indexProject()
	c.ListUpdates.Reset()
	c.DirtyProjectRows = make(map[string]bool)
	c.projectLoaded = true
	prev := make(map[string]addon.Status, len(c.projectStatuses))
	for _, s := range c.projectStatuses {
		prev[s.Addon.Name] = s
	}
	previousUnseen := c.unseen
	c.ProjectAddons, c.projectStatuses = nil, nil
	c.Pending = map[string]bool{}
	c.unseen = map[string]bool{}
	c.RootPending = true
	if c.GitDirty == nil {
		c.GitDirty = make(map[string]bool)
	}
	if c.GitSync == nil {
		c.GitSync = make(map[string]addon.GitSync)
	}
	if c.ManifestPath == "" {
		c.DepStatuses, c.OrphanDeps = nil, nil
		clear(c.GitDirty)
		clear(c.GitSync)
		return
	}
	addons, err := addon.Parse(c.ManifestPath)
	c.noteLoadErr("manifest", err)
	c.ProjectAddons = addons
	if err != nil {
		return
	}
	// Drop git markers of entries no longer in the manifest.
	names := make(map[string]bool, len(addons))
	for _, a := range addons {
		names[a.Name] = true
	}
	for name := range c.GitDirty {
		if !names[name] {
			delete(c.GitDirty, name)
		}
	}
	for name := range c.GitSync {
		if !names[name] {
			delete(c.GitSync, name)
		}
	}
	for _, a := range addons {
		s, ok := prev[a.Name]
		if !ok {
			s = addon.Status{Addon: a}
		}
		if !ok || previousUnseen[a.Name] {
			c.unseen[a.Name] = true
		}
		s.Addon = a
		c.projectStatuses = append(c.projectStatuses, s)
		c.Pending[a.Name] = true
	}
}

// StartProjectRefresh reloads the project without blocking: beginProject marks the rows
// pending at once, then each entry (and the root checkout) is inspected off the UI thread,
// landing on its own via ApplyRefreshResult. Dependency and orphan markers keep their
// previous values until the whole pass lands. A running pass is superseded.
func (c *Ctx) StartProjectRefresh() core.Action {
	c.beginProject()
	statuses := c.projectStatuses
	gen, act := c.Refresh.Start(len(statuses) + 1)
	projectRoot := c.ProjectRoot
	cmds := make([]tea.Cmd, 0, len(statuses)+1)
	for _, s := range statuses {
		a := s.Addon
		cmds = append(cmds, func() tea.Msg {
			if c.Refresh.Stale(gen) {
				return nil
			}
			m := addonInspectedMsg{gen: gen, status: addon.InspectOne(a, projectRoot)}
			if a.IsGitWorkdir() {
				m.dirty, m.sync = readGitState(m.status)
			}
			return core.PropagateAll(m)
		})
	}
	cmds = append(cmds, func() tea.Msg {
		if c.Refresh.Stale(gen) {
			return nil
		}
		m := rootDescribedMsg{gen: gen}
		if root, ok := repo.DescribeRoot(projectRoot); ok {
			m.root = &root
		}
		return core.PropagateAll(m)
	})
	return core.Seq(act, core.Async(core.BoundedBatch(maxInspect, cmds...)))
}

// ApplyRefreshResult stores a StartProjectRefresh result. handled is false for any other
// payload; changed reports a current result (the rows need rebuilding), and act carries the
// final status once the pass completes.
func (c *Ctx) ApplyRefreshResult(payload any) (handled, changed bool, act core.Action) {
	switch m := payload.(type) {
	case addonInspectedMsg:
		name := m.status.Addon.Name
		i, known := c.projectIndex[name]
		if c.Refresh.Stale(m.gen) || !known || !c.Pending[name] {
			return true, false, core.Action{}
		}
		ok, act := c.Refresh.Land(m.gen)
		if !ok {
			return true, false, act
		}
		c.projectStatuses[i] = m.status
		c.DirtyProjectRows[name] = true
		if m.status.Addon.IsGitWorkdir() {
			c.setGitState(name, m.dirty, m.sync)
		} else {
			c.setGitState(name, false, addon.GitSync{})
		}
		delete(c.Pending, name)
		delete(c.unseen, name)
		c.finishProject()
		return true, true, act
	case rootDescribedMsg:
		if c.Refresh.Stale(m.gen) || !c.RootPending {
			return true, false, core.Action{}
		}
		ok, act := c.Refresh.Land(m.gen)
		if !ok {
			return true, false, act
		}
		c.RootRepo, c.RootPending = m.root, false
		c.finishProject()
		return true, !c.Refresh.Running(), act
	}
	return false, false, core.Action{}
}

// finishProject recomputes the markers that need every inspection, once the pass lands.
func (c *Ctx) finishProject() {
	if c.Refresh.Running() {
		return
	}
	c.refreshDepChecks(c.projectStatuses)
	c.OrphanDeps = addon.OrphanDeps(c.projectStatuses)
	c.RankSnap = nil
}

// Unseen reports an entry the running refresh has not inspected yet and that has no
// earlier inspection to show, so its row has no state to draw.
func (c *Ctx) Unseen(name string) bool { return c.unseen[name] }
