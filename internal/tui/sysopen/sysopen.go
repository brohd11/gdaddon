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

// Terminal opens an OS terminal at path (a directory). On darwin/windows it shells
// out to a known terminal; on linux it probes for a common emulator and reports a
// status if none is found.
func Terminal(path string) core.Action {
	if _, err := os.Stat(path); err != nil {
		return core.SetStatusAndLog("path not found: " + path)
	}
	cmd := terminalCmd(path)
	if cmd == nil {
		return core.SetStatusAndLog("no terminal emulator found")
	}
	return core.Seq(
		core.SetStatus("opening terminal at "+path),
		core.Async(func() tea.Msg {
			_ = cmd.Start()
			return nil
		}),
	)
}

// terminalCmd builds the terminal launch command for the current OS, or returns nil
// when no suitable terminal could be found (linux with no known emulator on PATH).
func terminalCmd(path string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-a", "Terminal", path)
	case "windows":
		return exec.Command("cmd", "/c", "start", "cmd", "/k", "cd /d "+path)
	default:
		for _, t := range []struct {
			bin  string
			args []string
		}{
			{"x-terminal-emulator", []string{"--working-directory=" + path}},
			{"gnome-terminal", []string{"--working-directory=" + path}},
			{"konsole", []string{"--workdir", path}},
			{"xfce4-terminal", []string{"--working-directory=" + path}},
			{"xterm", []string{"-e", "cd " + path + " && exec ${SHELL:-/bin/sh}"}},
		} {
			if _, err := exec.LookPath(t.bin); err == nil {
				return exec.Command(t.bin, t.args...)
			}
		}
		return nil
	}
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
