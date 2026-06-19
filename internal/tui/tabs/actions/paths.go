package actions

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// newPathsPicker is a quick navigation helper (Actions ▸ Paths): each row reveals a
// project-relevant location in the OS file manager. Rows are built from the live
// context, so missing locations (no manifest yet) are simply omitted. Selecting a row
// fires the open command asynchronously and leaves the picker open, so several spots
// can be opened in a row.
func newPathsPicker(sh *core.Shared) core.Screen {
	c := appctx.Of(sh)
	var items []list.Item

	add := func(name, path string, reveal bool) {
		if path == "" {
			return
		}
		items = append(items, components.Item{
			Name: name,
			Desc: path,
			Pick: func(sh *core.Shared) core.Action { return openInFileManager(path, reveal) },
		})
	}

	add("Project", c.ProjectRoot, false)
	if c.ProjectRoot != "" {
		add("Addons dir", filepath.Join(c.ProjectRoot, "addons"), false)
	}
	add("Manifest", c.ManifestPath, true)
	if home, err := os.UserHomeDir(); err == nil {
		add(".gdaddon", filepath.Join(home, ".gdaddon"), false)
	}

	return components.NewPicker(items, components.PickerOpts{Title: "Paths"})
}

// openInFileManager opens path in the OS file manager. When reveal is set (used for a
// file like the manifest), the file is highlighted within its containing folder;
// otherwise path is opened directly as a directory.
func openInFileManager(path string, reveal bool) core.Action {
	if _, err := os.Stat(path); err != nil {
		return core.SetStatusAndLog("path not found: " + path)
	}
	return core.Seq(
		core.SetStatus("opening "+path),
		core.Async(func() tea.Msg {
			_ = openCmd(path, reveal).Start()
			return nil
		}),
	)
}

func openCmd(path string, reveal bool) *exec.Cmd {
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
