// Package docs is the in-TUI manual: a set of markdown pages compiled into the binary,
// browsed from Actions ▸ Docs, and offered to first-run users by a welcome popup.
//
// It's a flow rather than a tab because two layers reach it: the Actions tab (the menu
// row) and tui.Run (the first-run popup, via WelcomeCmd).
//
// Adding a page is dropping a numbered .md file into pages/ — no code change. The
// filename orders it (embed.FS reads sorted), the first "# " heading is its title, and
// the first line after that heading is its one-line description in the index.
package docs

import (
	"embed"
	"strings"
	"sync"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

//go:embed pages/*.md
var pagesFS embed.FS

type page struct {
	Title string
	Desc  string
	Body  string // everything after the title line (the description is its first paragraph)
}

var (
	once   sync.Once
	parsed []page
)

// Pages returns the embedded pages in filename order, parsed once.
func Pages() []page {
	once.Do(func() {
		entries, err := pagesFS.ReadDir("pages")
		if err != nil {
			return
		}
		for _, e := range entries {
			data, err := pagesFS.ReadFile("pages/" + e.Name())
			if err != nil {
				continue
			}
			parsed = append(parsed, parse(string(data)))
		}
	})
	return parsed
}

// parse pulls the title (the first "# " heading) and description (the first non-empty
// line under it) out of a page, leaving the body to the renderer. A page missing its
// heading still reads — it just falls back to its first line as the title.
func parse(src string) page {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	p := page{Body: src}
	for i, line := range lines {
		if !strings.HasPrefix(line, "# ") {
			continue
		}
		p.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
		p.Body = strings.Join(lines[i+1:], "\n")
		for _, rest := range lines[i+1:] {
			if strings.TrimSpace(rest) != "" {
				p.Desc = plain(strings.TrimSpace(rest))
				break
			}
		}
		break
	}
	if p.Title == "" && len(lines) > 0 {
		p.Title = strings.TrimSpace(lines[0])
	}
	return p
}

// Index is the docs menu: one self-dispatching row per page, each pushing its own
// DocScreen.
func Index() *components.PickerScreen {
	var items []list.Item
	for _, p := range Pages() {
		p := p
		items = append(items, components.Item{
			Name: p.Title,
			Desc: p.Desc,
			Pick: func(*core.Shared) core.Action { return core.Push(newPage(p)) },
		})
	}
	items = components.EnsurePlaceholder(items, "(no pages)", "the docs pages didn't compile into this build")
	return components.NewPicker(items, components.PickerOpts{Title: "Docs", Crumb: "Docs"})
}

// newPage is the scrollable reader for one page; the body is re-rendered at whatever
// width the screen is given, so it re-wraps on resize.
func newPage(p page) *components.DocScreen {
	return components.NewDocScreen(components.DocOpts{
		Title:  p.Title,
		Render: func(width int) string { return render(p.Body, width) },
	})
}

// Welcome is the first-run popup: a modal over whatever tab the TUI opened on, offering
// the docs. Enter opens the index in its place (Replace, so esc from the index lands
// back on the tab rather than re-showing the popup); esc dismisses it.
func Welcome() *components.DialogScreen {
	return components.CreatePopup(
		"Welcome to gdaddon",
		"gdaddon installs and tracks Godot addons from a manifest.\n\n"+
			"Set up ~/.gdaddon for its config and archive.\n\n"+
			"Docs are available any time under Actions ▸ Docs.",
		core.Replace(Index()),
		core.Hint("open docs", core.Keys.Yes),
		core.Hint("dismiss", core.Keys.No),
	)
}

// WelcomeCmd shows the welcome popup once the router is up. It's a cmd (not an Action)
// so it can ride bubblestack.Config.Init alongside the startup update check; the router
// applies the Action it returns as a message.
func WelcomeCmd() tea.Cmd {
	return func() tea.Msg { return core.Push(Welcome()) }
}
