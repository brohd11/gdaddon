package project

import (
	"context"
	"maps"
	"time"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/core"

	tea "charm.land/bubbletea/v2"
)

// updateChecksReady carries update-check results to the Project root as a broadcast, so the
// markers appear even from another tab.
type updateChecksReady struct {
	gen      int64
	checks   map[string]addon.UpdateInfo
	stampErr error
}

// updateCheckTimeout caps the whole batch of release-listing fetches so a slow or
// unreachable host can't leave the check pending forever.
const updateCheckTimeout = 30 * time.Second

// checkUpdatesCmd checks each installed addon for a newer release off the UI thread and
// broadcasts the results, saving them for later sessions. When the daily limit skips the
// check and this session holds no results yet, it shows the saved ones instead,
// re-checking only entries whose pin changed since they were saved. A check superseded
// before it starts (RefreshAll asks twice in one tick) skips its network work.
func checkUpdatesCmd(sh *core.Shared) tea.Cmd {
	c := appctx.Of(sh)
	manifestPath, projectRoot := c.ManifestPath, c.ProjectRoot
	// Later skipped checks keep the session's results (and leave pin changes to
	// recheckChangedCmd, so nothing is checked twice).
	fromCache := c.UpdateChecks == nil
	gen := c.UpdateGen.Add(1)
	return func() tea.Msg {
		if c.UpdateGen.Load() != gen || manifestPath == "" {
			return nil
		}
		begin, stampErr := addon.BeginUpdateCheck(projectRoot, time.Now(), false)
		if !begin {
			if !fromCache {
				return nil
			}
			return cachedUpdatesMsg(gen, manifestPath, projectRoot)
		}
		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			if stampErr != nil {
				return core.PropagateAll(updateChecksReady{gen: gen, stampErr: stampErr})
			}
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()

		checks := addon.CheckUpdates(ctx, statuses)
		addons := make([]addon.Addon, 0, len(statuses))
		for _, s := range statuses {
			addons = append(addons, s.Addon)
		}
		if err := addon.SaveUpdateCache(projectRoot, addons, checks); err != nil && stampErr == nil {
			stampErr = err
		}
		return core.PropagateAll(updateChecksReady{gen: gen, checks: checks, stampErr: stampErr})
	}
}

// cachedUpdatesMsg loads the saved results for the manifest's entries, re-checking (and
// re-saving) those whose pin changed since. The checks map is never nil, so the session
// counts as holding results afterwards.
func cachedUpdatesMsg(gen int64, manifestPath, projectRoot string) tea.Msg {
	m, err := addon.LoadManifest(manifestPath)
	if err != nil {
		return nil
	}
	checks, stale := addon.CachedUpdates(projectRoot, m.Addons)
	var saveErr error
	if len(stale) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()
		fresh := addon.CheckUpdatesOf(ctx, stale)
		maps.Copy(checks, fresh)
		saveErr = addon.MergeUpdateCache(projectRoot, stale, fresh)
	}
	return core.PropagateAll(updateChecksReady{gen: gen, checks: checks, stampErr: saveErr})
}

// updateRecheckReady carries a targeted re-check's results: root and the checked pin keys
// let the receiver drop results that a root change or a further edit made stale.
type updateRecheckReady struct {
	root   string
	keys   map[string]string
	checks map[string]addon.UpdateInfo
}

// recheckChangedCmd re-checks only the entries whose pin changed in the reload that just
// began (nil when none did). The user changed them, so it skips the daily stamp: an
// update or reinstall clears its marker without waiting a day.
func recheckChangedCmd(sh *core.Shared) tea.Cmd {
	c := appctx.Of(sh)
	changed := c.TakePinChanged()
	if len(changed) == 0 {
		return nil
	}
	root := c.ProjectRoot
	keys := make(map[string]string, len(changed))
	for _, a := range changed {
		keys[a.Name] = addon.UpdatePinKey(a)
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()
		checks := addon.CheckUpdatesOf(ctx, changed)
		// Best effort: an unsaved result only costs the next session a re-check.
		_ = addon.MergeUpdateCache(root, changed, checks)
		return core.PropagateAll(updateRecheckReady{root: root, keys: keys, checks: checks})
	}
}
