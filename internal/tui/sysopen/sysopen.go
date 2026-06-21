// Package sysopen opens filesystem paths and URLs with the OS default handler
// (file manager / web browser). It names no domain type and imports only core, so
// any tab (actions, project, global, …) can reuse it without a cross-tab import.
package sysopen

import (
	"gdaddon/internal/source"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"

	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// Path opens path in the OS file manager. When reveal is set (used for a file like
// the manifest), the file is highlighted within its containing folder; otherwise
// path is opened directly as a directory.
func Path(path string, reveal bool) core.Action {
	if _, err := os.Stat(path); err != nil {
		return core.SetStatusAndLog("path not found: " + path)
	}
	return core.Seq(
		core.SetStatus("opening "+path),
		core.Async(func() tea.Msg {
			_ = pathCmd(path, reveal).Start()
			return nil
		}),
	)
}

// URL opens target in the default web browser.
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
	return core.Seq(
		core.SetStatus("opening "+target),
		core.Async(func() tea.Msg {
			_ = urlCmd(target).Start()
			return nil
		}),
	)
}

func pathCmd(path string, reveal bool) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		if reveal {
			return exec.Command("open", "-R", path)
		}
		return exec.Command("open", path)
	case "windows":
		if reveal {
			return exec.Command("explorer", "/select,"+path)
		}
		return exec.Command("explorer", path)
	default:
		if reveal {
			path = filepath.Dir(path)
		}
		return exec.Command("xdg-open", path)
	}
}

func urlCmd(target string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", target)
	case "windows":
		return exec.Command("cmd", "/c", "start", "", target)
	default:
		return exec.Command("xdg-open", target)
	}
}
