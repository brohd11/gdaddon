// Package sysopen adapts bubblestack/sysopen for gdaddon: URL reduces an asset url to its
// repo before opening.
package sysopen

import (
	"github.com/brohd11/gdaddon/internal/source"
	"path"

	"github.com/brohd11/bubblestack/core"
	bsysopen "github.com/brohd11/bubblestack/sysopen"
)

// Path opens path in the OS file manager. When reveal is set (used for a file like the
// manifest), the file is highlighted within its containing folder.
func Path(p string, reveal bool) core.Action {
	return bsysopen.Path(p, reveal)
}

// Terminal opens a detached OS terminal window at dir (a directory).
func Terminal(dir string) core.Action {
	return bsysopen.Terminal(dir)
}

// TerminalInline hands gdaddon's own terminal to a shell at dir: the TUI suspends and is
// restored when the shell exits, so no window is left behind.
func TerminalInline(dir string) core.Action {
	return bsysopen.TerminalInlineFor("gdaddon", dir)
}

// URL opens target in the browser, reducing a file url (release asset, archive) to its
// repo first.
func URL(target string) core.Action {
	if target == "" {
		return core.SetStatusAndLog("no source url")
	}
	if path.Ext(target) != "" {
		host, err := source.RepoURL(target)
		if err != nil {
			return core.SetStatusAndLog("could not get host of url: " + target)
		}
		target = host
	}
	return bsysopen.URL(target)
}
