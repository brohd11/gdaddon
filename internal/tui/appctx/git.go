package appctx

import (
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gitstack/repo"
	"github.com/brohd11/gitstack/repoui"
)

// refreshGitChecks recomputes each present checkout's dirty state and divergence (local
// reads, no network).
func (c *Ctx) refreshGitChecks(statuses []addon.Status) {
	c.GitDirty = make(map[string]bool)
	c.GitSync = make(map[string]addon.GitSync)
	for _, s := range statuses {
		if s.Addon.IsGitWorkdir() {
			c.recordGitState(s)
		}
	}
}

// recordGitState replaces one checkout's entries in GitDirty and GitSync.
func (c *Ctx) recordGitState(s addon.Status) {
	delete(c.GitDirty, s.Addon.Name)
	delete(c.GitSync, s.Addon.Name)
	if !s.Present() {
		return
	}
	if addon.HasUncommittedChanges(s.FullPath) {
		c.GitDirty[s.Addon.Name] = true
	}
	if sync := addon.GitSyncStatus(s.FullPath); sync.Tracking {
		c.GitSync[s.Addon.Name] = sync
	}
}

// RefreshRepo refreshes a known addon and enclosing checkout markers. Root
// operations may change the manifest or package layout and retain a full reload.
func (c *Ctx) RefreshRepo(msg repoui.RepoRefreshMsg) {
	if msg.Targets(c.ProjectRoot) {
		c.loadProject()
		return
	}
	statuses := c.ProjectStatuses()
	known := false
	for _, s := range statuses {
		known = known || (s.Addon.IsGitWorkdir() && msg.Targets(s.FullPath))
	}
	if !known {
		return
	}
	if c.GitDirty == nil {
		c.GitDirty = make(map[string]bool)
	}
	if c.GitSync == nil {
		c.GitSync = make(map[string]addon.GitSync)
	}
	for i, s := range statuses {
		if !s.Addon.IsGitWorkdir() || !msg.Affects(s.FullPath) {
			continue
		}
		if msg.Targets(s.FullPath) {
			s = addon.InspectOne(s.Addon, c.ProjectRoot)
			statuses[i] = s
		}
		c.recordGitState(s)
	}
	if c.RootRepo != nil && msg.Affects(c.RootRepo.Dir) {
		root, ok := repo.DescribeRoot(c.ProjectRoot)
		c.RootRepo = nil
		if ok {
			c.RootRepo = &root
		}
	}
	c.refreshDepChecks(statuses)
	c.OrphanDeps = addon.OrphanDeps(statuses)
}
