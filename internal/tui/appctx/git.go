package appctx

import (
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gitstack/repo"
	"github.com/brohd11/gitstack/repoui"
)

// GitHookApp selects gdaddon's user commands for the shared Git screens.
func (*Ctx) GitHookApp() string { return "gdaddon" }

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
	dirty, sync := readGitState(s)
	c.setGitState(s.Addon.Name, dirty, sync)
}

// readGitState reads a present checkout's dirty state and divergence (local git reads, safe
// off the UI thread); an absent one reads clean and untracked.
func readGitState(s addon.Status) (dirty bool, sync addon.GitSync) {
	if !s.Present() {
		return false, addon.GitSync{}
	}
	return addon.HasUncommittedChanges(s.FullPath), addon.GitSyncStatus(s.FullPath)
}

// setGitState stores one checkout's git state, dropping clean/untracked entries.
func (c *Ctx) setGitState(name string, dirty bool, sync addon.GitSync) {
	delete(c.GitDirty, name)
	delete(c.GitSync, name)
	if dirty {
		c.GitDirty[name] = true
	}
	if sync.Tracking {
		c.GitSync[name] = sync
	}
}

// RefreshRepo refreshes a known addon and enclosing checkout markers. Root operations may
// change the manifest or package layout, so they report full and the caller starts a
// whole-project refresh.
func (c *Ctx) RefreshRepo(msg repoui.RepoRefreshMsg) (full bool) {
	if msg.Targets(c.ProjectRoot) {
		return true
	}
	statuses := c.ProjectStatuses()
	known := false
	for _, s := range statuses {
		known = known || (s.Addon.IsGitWorkdir() && msg.Targets(s.FullPath))
	}
	if !known {
		return false
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
	return false
}
