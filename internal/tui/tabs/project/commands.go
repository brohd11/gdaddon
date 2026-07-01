package project

import (
	"gdaddon/internal/tui/appctx"
	"strings"

	"github.com/brohd11/bubblestack/core"

	"gdaddon/internal/addon"
)

// pinInstall writes the freshly installed entry's url/path/version/tag (+clone
// flag) into the manifest and returns a human status line. path is passed in
// explicitly so the post-install location form can pin a corrected path; the
// url/version/tag derivation is shared with the silent finish path.
func pinInstall(manifestPath string, selected addon.Addon, pick versionItem, path, instVersion string) string {
	name, url := selected.Name, pick.asset.URL
	// Installing from the local archive must not pin the machine-specific archive
	// path as the manifest url — keep the entry's canonical repo url instead.
	if pick.archived {
		url = ""
	}
	version := instVersion
	// Fall back to the picked tag as the version only for release installs; a clone
	// tracks a branch and a branch package carries the branch name in pick.tag (not a
	// version), so leave version empty for both rather than recording the branch name.
	if version == "" && !pick.clone && !pick.branch {
		version = strings.TrimPrefix(pick.tag, "v")
	}
	// Branch-HEAD installs carry the branch name in pick.tag but have no release
	// tag; pick.branch marks them so we don't record a bogus tag. A clone install
	// is the exception: it keeps the branch as tag and records the canonical .git
	// url so a re-clone targets the right branch.
	tag := pick.tag
	if pick.branch && !pick.clone {
		tag = ""
	}
	if pick.clone {
		url = "https://" + pick.repoID + ".git"
	}

	_ = addon.UpdateEntry(manifestPath, name, url, path, version, tag)
	// Always write the kind so a package install over a former clone clears the
	// stale kind line (SetKind removes it for KindPackage), not just clone installs.
	kind := addon.KindPackage
	if pick.clone {
		kind = addon.KindClone
	}
	_ = addon.SetKind(manifestPath, name, kind)
	// A branch package pins the resolved HEAD commit; record it (and clear any stale
	// pin on every other install kind, so a re-install off a release/branch drops it).
	commit := ""
	if pick.branch && !pick.clone {
		commit = pick.asset.Commit
	}
	_ = addon.SetCommit(manifestPath, name, commit)

	if pick.clone {
		return "cloned " + name + " (" + pick.tag + ")"
	}
	if commit != "" {
		return "pinned " + name + " @ " + shortSHA(commit)
	}
	return "updated " + name + " → " + version
}

// commitRemove removes the addon from the project according to the chosen mode:
// "local" deletes the installed files but keeps the manifest entry, "project"
// removes the manifest entry only, "project + local" does both. On success it
// broadcasts ProjectDirty, which reloads the browse list from the manifest and focuses it.
func commitRemove(sh *core.Shared, st addon.Status, mode int) core.Action {
	c := appctx.Of(sh)
	if mode == removeLocal || mode == removeProjectLocal {
		if err := addon.Uninstall(st.Addon, c.ProjectRoot); err != nil {
			return core.Seq(
				core.SetStatusAndLog("error: "+err.Error()),
				core.ResetToRoot(),
			)
		}
	}
	if mode != removeLocal {
		if err := addon.RemoveEntry(c.ManifestPath, st.Addon.Name); err != nil {
			return core.Seq(
				core.SetStatusAndLog("error: "+err.Error()),
				core.ResetToRoot(),
			)
		}
	}
	msg := "removed " + st.Addon.Name
	if mode == removeLocal {
		msg = "deleted files for " + st.Addon.Name
	}
	return core.Seq(
		core.SetStatus(msg),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.ShowTab(appctx.TitleProject),
	)
}
