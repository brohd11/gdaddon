package project

import (
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/core"

	"github.com/brohd11/gdaddon/internal/addon"
)

// pinInstall writes the installed entry's url, path, version and tag (and kind and commit)
// from the InstallResult, returning a status line. A failed manifest write is an error
// even though the install succeeded.
func pinInstall(manifestPath string, selected addon.Addon, pick versionItem, res addon.InstallResult) (string, error) {
	name, url := selected.Name, pick.asset.URL
	path, instVersion := res.Path, res.Version
	// Installing from the local archive must not pin the machine-specific archive
	// path as the manifest url — keep the entry's canonical repo url instead.
	if pick.archived {
		url = ""
	}
	// A commit-pinned package (a branch install, or an archived copy of one) records only its
	// sha.
	commit := ""
	if pick.asset.Commit != "" && !pick.clone {
		commit = pick.asset.Commit
	}
	pinned := commit != ""

	version := instVersion
	// Use the picked tag as the version only for release installs; clones and branch packages
	// carry a branch name there.
	if version == "" && !pick.clone && !pick.branch && !pinned {
		version = addon.TagVersion(pick.tag)
	}
	// Branch and commit-pinned installs record no tag. Clones keep the branch as tag and
	// record the canonical .git url.
	tag := pick.tag
	if (pick.branch || pinned) && !pick.clone {
		tag = ""
	}
	if pick.clone {
		url = addon.CloneURL(pick.repoID)
	}

	if err := addon.UpdateEntry(manifestPath, name, url, path, version, tag); err != nil {
		return "", err
	}
	// Always write the kind so a package install over a former clone clears the
	// stale kind line (SetKind removes it for KindPackage), not just clone installs.
	kind := addon.KindPackage
	if pick.clone {
		kind = addon.KindClone
	}
	if err := addon.SetKind(manifestPath, name, kind); err != nil {
		return "", err
	}
	// Record the pinned HEAD commit (computed above), clearing any stale pin on every
	// other install kind so a re-install off a release/branch drops it.
	if err := addon.SetCommit(manifestPath, name, commit); err != nil {
		return "", err
	}
	if err := addon.AdoptName(manifestPath, selected, res); err != nil {
		return "", err
	}

	label := selected.Label()
	if pick.clone {
		return "cloned " + label + " (" + pick.tag + ")", nil
	}
	if commit != "" {
		return "pinned " + label + " @ " + shortSHA(commit), nil
	}
	return "updated " + label + " → " + version, nil
}

// commitRemove removes per mode: "local" deletes the files, "project" the entry,
// "project + local" both. It broadcasts ProjectDirty.
func commitRemove(sh *core.Shared, st addon.Status, mode int) core.Action {
	c := appctx.Of(sh)
	if mode == removeLocal || mode == removeProjectLocal {
		if err := addon.Uninstall(st.Addon, c.ProjectRoot); err != nil {
			return core.SeqErr(err, core.ResetToRoot())
		}
	}
	if mode != removeLocal {
		if err := addon.RemoveEntry(c.ManifestPath, st.Addon.Name); err != nil {
			return core.SeqErr(err, core.ResetToRoot())
		}
	}
	msg := "removed " + st.Addon.Label()
	if mode == removeLocal {
		msg = "deleted files for " + st.Addon.Label()
	}
	return core.Seq(
		core.SetStatus(msg),
		core.PropagateAll(appctx.ProjectDirty{}),
		core.ShowTab(appctx.TitleProject),
	)
}
