// Package sysopen is gdaddon's thin domain adapter over bubblestack/sysopen (the shared
// OS-open helpers: file manager, browser, terminal). Path and Terminal delegate
// directly; URL adds gdaddon's domain normalization (an addon URL with a file extension
// is reduced to its repo host via source.RepoURL) before handing off. It names no other
// domain type, so any tab can reuse it without a cross-tab import.
package sysopen

import (
	"gdaddon/internal/source"
	"path"

	"github.com/brohd11/bubblestack/core"
	bsysopen "github.com/brohd11/bubblestack/sysopen"
)

// Path opens path in the OS file manager. When reveal is set (used for a file like the
// manifest), the file is highlighted within its containing folder.
func Path(p string, reveal bool) core.Action {
	return bsysopen.Path(p, reveal)
}

// Terminal opens an OS terminal at dir (a directory).
func Terminal(dir string) core.Action {
	return bsysopen.Terminal(dir)
}

// URL opens target in the default web browser. An addon url that points at a file (a
// release asset, a source archive) is first reduced to its repo host via source.RepoURL
// — the browser should land on the repo, not download the asset.
func URL(target string) core.Action {
	if target == "" {
		return core.SetStatusAndLog("no source url")
	}
	if path.Ext(target) != "" {
		host, err := source.RepoURL(target) // think this only handles repos, not asset store
		if err != nil {
			return core.SetStatusAndLog("could not get host of url: " + target)
		}
		target = host
	}
	return bsysopen.URL(target)
}
