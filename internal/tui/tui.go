// Package tui implements the interactive bubbletea front-end for browsing and
// installing addons. It renders state produced by the addon package and turns
// install progress into bubbletea messages.
package tui

import (
	"fmt"

	"gdutil/internal/addon"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Run loads the manifest, builds the program, and blocks until the user quits.
func Run(manifestPath, projectRoot string) error {
	statuses, err := addon.Inspect(manifestPath, projectRoot)
	if err != nil {
		return err
	}

	m := newModel(manifestPath, projectRoot, statuses)
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

// ---------- list items ----------

type item struct{ status addon.Status }

func (i item) Title() string       { return i.status.Addon.Name }
func (i item) FilterValue() string { return i.status.Addon.Name }

func (i item) Description() string {
	s := i.status
	switch s.State {
	case addon.StateInvalid:
		return "✗ invalid — missing url or path"
	case addon.StateMissing:
		return "• not installed"
	case addon.StateInstalled:
		return fmt.Sprintf("✓ installed v%s", s.LocalVersion)
	case addon.StateUnversioned:
		return "✓ installed (no version pinned)"
	case addon.StateMismatch:
		local := s.LocalVersion
		if local == "" {
			local = "unknown"
		}
		return fmt.Sprintf("⚠ update: %s → %s", local, s.Addon.Version)
	}
	return ""
}

// ---------- messages ----------

// installEvent carries one progress line, or (when done) the final result.
type installEvent struct {
	line string
	done bool
	err  error
}

// ---------- model ----------

type model struct {
	manifestPath string
	projectRoot  string

	list    list.Model
	spinner spinner.Model

	installing bool
	current    string // name of addon being installed
	events     chan installEvent
	logs       []string
	err        error
}

var (
	logStyle    = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("245"))
	statusStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true)
)

func newModel(manifestPath, projectRoot string, statuses []addon.Status) model {
	items := make([]list.Item, len(statuses))
	for i, s := range statuses {
		items[i] = item{status: s}
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Godot Addons"
	l.SetShowStatusBar(false)

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return model{
		manifestPath: manifestPath,
		projectRoot:  projectRoot,
		list:         l,
		spinner:      sp,
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Reserve some rows below the list for the progress/log area.
		m.list.SetSize(msg.Width, msg.Height-logHeight)
		return m, nil

	case tea.KeyMsg:
		if m.installing {
			return m, nil // ignore input while an install is running
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "enter":
			it, ok := m.list.SelectedItem().(item)
			if !ok || !it.status.Installable() {
				return m, nil
			}
			return m.startInstall(it.status.Addon)
		}

	case installEvent:
		if msg.done {
			m.installing = false
			m.err = msg.err
			if msg.err != nil {
				m.logs = append(m.logs, fmt.Sprintf("[%s] error: %v", m.current, msg.err))
			} else {
				m.logs = append(m.logs, fmt.Sprintf("[%s] done", m.current))
			}
			m.current = ""
			return m, m.refresh()
		}
		m.logs = append(m.logs, msg.line)
		return m, waitForEvent(m.events)

	case refreshMsg:
		for i, s := range msg.statuses {
			m.list.SetItem(i, item{status: s})
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

const logHeight = 8

func (m model) View() string {
	var footer string
	if m.installing {
		footer = statusStyle.Render(fmt.Sprintf("%s installing %s...", m.spinner.View(), m.current))
	} else {
		footer = statusStyle.Render("↑/↓ navigate · enter install · q quit")
	}

	logs := m.logs
	if len(logs) > logHeight-2 {
		logs = logs[len(logs)-(logHeight-2):]
	}
	logBlock := ""
	for _, l := range logs {
		logBlock += logStyle.Render(l) + "\n"
	}

	return m.list.View() + "\n" + footer + "\n" + logBlock
}

// ---------- commands ----------

func (m model) startInstall(a addon.Addon) (tea.Model, tea.Cmd) {
	m.installing = true
	m.current = a.Name
	m.events = make(chan installEvent)

	go func(events chan installEvent) {
		report := func(format string, args ...any) {
			events <- installEvent{line: fmt.Sprintf(format, args...)}
		}
		err := addon.Install(a, m.projectRoot, report)
		events <- installEvent{done: true, err: err}
	}(m.events)

	return m, tea.Batch(m.spinner.Tick, waitForEvent(m.events))
}

// waitForEvent blocks for the next install event so it can become a tea.Msg.
func waitForEvent(events chan installEvent) tea.Cmd {
	return func() tea.Msg {
		return <-events
	}
}

type refreshMsg struct{ statuses []addon.Status }

// refresh re-inspects the manifest so freshly installed addons show new state.
func (m model) refresh() tea.Cmd {
	return func() tea.Msg {
		statuses, err := addon.Inspect(m.manifestPath, m.projectRoot)
		if err != nil {
			return refreshMsg{}
		}
		return refreshMsg{statuses: statuses}
	}
}
