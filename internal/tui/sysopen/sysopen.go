// Package sysopen opens filesystem paths and URLs with the OS default handler (file
// manager / web browser / terminal). It names no domain type — only core plus the two
// leaf packages it needs (source for url normalizing, config for the `terminal` key) —
// so any tab (actions, project, global, …) can reuse it without a cross-tab import.
package sysopen

import (
	"gdaddon/internal/config"
	"gdaddon/internal/source"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
)

// start runs cmd detached and reports the failure on the status line rather than
// swallowing it — a terminal emulator that rejects an option dies immediately, and
// silence there is indistinguishable from a working launch. The returned message is a
// framework control message; the router applies it when the async cmd's result lands.
func start(cmd *exec.Cmd, what string) tea.Msg {
	if err := cmd.Start(); err != nil {
		return core.SetStatusAndLog("could not open " + what + ": " + err.Error()).Msg
	}
	go cmd.Wait() //nolint:errcheck // reap the child; a terminal that stays open just parks this goroutine
	return nil
}

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
			return start(pathCmd(path, reveal), path)
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
			return start(urlCmd(target), target)
		}),
	)
}

// Terminal opens a terminal at dir. config.yml's `terminal` key wins if set; otherwise
// darwin/windows shell out to the system terminal and linux probes for a common
// emulator, reporting a status if none is found.
func Terminal(dir string) core.Action {
	if _, err := os.Stat(dir); err != nil {
		return core.SetStatusAndLog("path not found: " + dir)
	}
	cmd := terminalCmd(dir)
	if cmd == nil {
		return core.SetStatusAndLog("no terminal emulator found — set `terminal` in ~/.gdaddon/config/config.yml")
	}
	return core.Seq(
		core.SetStatus("opening terminal at "+dir),
		core.Async(func() tea.Msg {
			return start(cmd, "terminal at "+dir)
		}),
	)
}

// terminalCmd builds the terminal launch command for dir, or returns nil when no
// suitable terminal could be found (linux with no known emulator on PATH).
func terminalCmd(dir string) *exec.Cmd {
	cmd := buildTerminalCmd(dir)
	if cmd == nil {
		return nil
	}
	// The launched terminal also *inherits* this as its cwd, which is the load-bearing
	// part on linux: emulators that don't understand the working-directory option (and
	// wrappers like x-terminal-emulator, which silently drop it) then still open in the
	// right place instead of wherever gdaddon happens to be running from.
	cmd.Dir = dir
	return cmd
}

// buildTerminalCmd picks the terminal: the configured command if there is one (on any
// platform — someone on macOS may prefer iTerm to Terminal.app), else the OS default.
func buildTerminalCmd(dir string) *exec.Cmd {
	if cmd := configuredTerminal(dir); cmd != nil {
		return cmd
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-a", "Terminal", dir)
	case "windows":
		return exec.Command("cmd", "/c", "start", "cmd", "/k", "cd /d "+dir)
	default:
		return probeTerminal(dir)
	}
}

// configuredTerminal builds the command from config.yml's `terminal` key, or nil when
// unset (or unreadable — a broken config falls back to probing rather than blocking the
// launch). {dir} is substituted in each argument; a template without it still works,
// since Terminal sets the command's cwd either way.
func configuredTerminal(dir string) *exec.Cmd {
	cfg, err := config.Load()
	if err != nil || cfg == nil || strings.TrimSpace(cfg.Terminal) == "" {
		return nil
	}
	fields := splitCommand(cfg.Terminal)
	if len(fields) == 0 {
		return nil
	}
	for i, f := range fields {
		fields[i] = strings.ReplaceAll(f, "{dir}", dir)
	}
	return exec.Command(fields[0], fields[1:]...)
}

// linuxTerminals is the probe order: the emulator each desktop actually ships first,
// then the rest, each with the option *it* understands for the starting directory.
// Entries with no option rely solely on the command's cwd.
//
// x-terminal-emulator is deliberately last. It's the Debian alternatives symlink, and
// its contract only guarantees the xterm options -T and -e; the gnome-terminal wrapper
// behind it drops --working-directory on the floor, which is how a terminal opened at
// gdaddon's own cwd (the project root) rather than the addon directory.
var linuxTerminals = []struct {
	bin  string
	args []string // {dir} is replaced with the target directory
}{
	{"gnome-terminal", []string{"--working-directory={dir}"}},
	{"ptyxis", []string{"--working-directory={dir}"}}, // GNOME's current default terminal
	{"kgx", []string{"--working-directory={dir}"}},    // GNOME Console
	{"konsole", []string{"--workdir", "{dir}"}},
	{"kitty", []string{"--directory", "{dir}"}},
	{"alacritty", []string{"--working-directory", "{dir}"}},
	{"wezterm", []string{"start", "--cwd", "{dir}"}},
	{"foot", []string{"--working-directory={dir}"}},
	{"tilix", []string{"-w", "{dir}"}},
	{"terminator", []string{"--working-directory={dir}"}},
	{"xfce4-terminal", []string{"--working-directory={dir}"}},
	{"mate-terminal", []string{"--working-directory={dir}"}},
	{"lxterminal", []string{"--working-directory={dir}"}},
	{"urxvt", nil},
	{"st", nil},
	{"xterm", nil},
	{"x-terminal-emulator", nil},
}

// probeTerminal returns the first emulator on PATH, or nil when none is installed.
func probeTerminal(dir string) *exec.Cmd {
	for _, t := range linuxTerminals {
		if _, err := exec.LookPath(t.bin); err != nil {
			continue
		}
		args := make([]string, len(t.args))
		for i, a := range t.args {
			args[i] = strings.ReplaceAll(a, "{dir}", dir)
		}
		return exec.Command(t.bin, args...)
	}
	return nil
}

// splitCommand splits a configured command line into arguments on whitespace, honoring
// single and double quotes so a path with spaces survives. It is deliberately not a
// shell: no expansion, no operators — the emulator is exec'd directly.
func splitCommand(s string) []string {
	var (
		out   []string
		cur   strings.Builder
		quote rune
		have  bool // cur holds a field, even if empty ("" is a real argument)
	)
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			cur.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			have = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if have || cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteRune(r)
		}
	}
	if have || cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
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
