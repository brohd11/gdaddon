// Package tui implements the interactive bubbletea front-end for browsing
// addons, picking a remote version, and installing/updating. It renders state
// from the addon package and turns install progress into bubbletea messages.
package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"gdutil/internal/addon"
	"gdutil/internal/source"

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

// ---------- modes ----------

type mode int

const (
	modeBrowse mode = iota
	modeFetching
	modeVersions
	modeConfirm
	modeInstalling
)

// rows reserved below a list for footer/status/log.
const reserve = 8

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
		return fmt.Sprintf("⚠ manifest pins %s, installed %s", s.Addon.Version, local)
	}
	return ""
}

type versionItem struct {
	tag        string
	asset      source.Asset
	prerelease bool
	branch     bool
}

func (v versionItem) Title() string {
	if v.branch {
		return "branch: " + v.tag
	}
	return v.tag
}

func (v versionItem) Description() string {
	d := v.asset.Name
	if v.branch {
		d = "latest commit · " + d
	}
	if v.prerelease {
		d += " · prerelease"
	}
	return d
}

func (v versionItem) FilterValue() string { return v.tag + " " + v.asset.Name }

func versionItems(l *source.Listing) []list.Item {
	var items []list.Item
	if l.Branch != nil {
		for _, a := range l.Branch.Assets {
			items = append(items, versionItem{tag: l.Branch.Tag, asset: a, branch: true})
		}
	}
	for _, r := range l.Releases {
		for _, a := range r.Assets {
			items = append(items, versionItem{tag: r.Tag, asset: a, prerelease: r.Prerelease})
		}
	}
	return items
}

// ---------- messages ----------

type releasesMsg struct {
	listing *source.Listing
	err     error
}

type installEvent struct {
	line string
	done bool
	err  error
}

type refreshMsg struct {
	statuses []addon.Status
	version  string
}

// ---------- model ----------

type model struct {
	manifestPath string
	projectRoot  string
	width        int
	height       int

	mode     mode
	addons   list.Model
	versions list.Model
	spinner  spinner.Model

	selected addon.Addon
	pick     versionItem

	events    chan installEvent
	logs      []string
	statusMsg string
}

var (
	helpStyle   = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("241"))
	statusStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(lipgloss.Color("212"))
	logStyle    = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("245"))
	boxStyle    = lipgloss.NewStyle().Margin(1, 2).Padding(1, 2).Border(lipgloss.RoundedBorder())
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
		addons:       l,
		spinner:      sp,
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.addons.SetSize(msg.Width, msg.Height-reserve)
		// m.versions is a zero-value list until a version list is built in
		// releasesMsg; SetSize on it would nil-panic. It's created with the
		// current size there, so we only need to resize it while it's in use.
		if m.mode == modeVersions {
			m.versions.SetSize(msg.Width, msg.Height-reserve)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case releasesMsg:
		if msg.err != nil {
			m.statusMsg = "error: " + msg.err.Error()
			m.mode = modeBrowse
			return m, nil
		}
		items := versionItems(msg.listing)
		if len(items) == 0 {
			m.statusMsg = "no downloadable .zip versions found"
			m.mode = modeBrowse
			return m, nil
		}
		vl := list.New(items, list.NewDefaultDelegate(), m.width, m.height-reserve)
		vl.Title = "Versions · " + m.selected.Name
		vl.SetShowStatusBar(false)
		m.versions = vl
		m.mode = modeVersions
		return m, nil

	case installEvent:
		if !msg.done {
			m.logs = append(m.logs, msg.line)
			return m, waitForEvent(m.events)
		}
		if msg.err != nil {
			m.logs = append(m.logs, fmt.Sprintf("[%s] error: %v", m.selected.Name, msg.err))
			m.statusMsg = "install failed"
			m.mode = modeBrowse
			return m, nil
		}
		m.logs = append(m.logs, fmt.Sprintf("[%s] installed", m.selected.Name))
		return m, m.finishInstall()

	case refreshMsg:
		if msg.statuses != nil {
			for i, s := range msg.statuses {
				if i < len(m.addons.Items()) {
					m.addons.SetItem(i, item{status: s})
				}
			}
		}
		m.statusMsg = fmt.Sprintf("updated %s → %s", m.selected.Name, msg.version)
		m.mode = modeBrowse
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	// Route anything else to the active list.
	var cmd tea.Cmd
	switch m.mode {
	case modeBrowse:
		m.addons, cmd = m.addons.Update(msg)
	case modeVersions:
		m.versions, cmd = m.versions.Update(msg)
	}
	return m, cmd
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.mode {
	case modeBrowse:
		switch k {
		case "q":
			return m, tea.Quit
		case "enter":
			it, ok := m.addons.SelectedItem().(item)
			if !ok || !it.status.Installable() {
				return m, nil
			}
			m.selected = it.status.Addon
			m.statusMsg = ""
			m.mode = modeFetching
			return m, tea.Batch(m.spinner.Tick, fetchReleases(m.selected.URL))
		}
		var cmd tea.Cmd
		m.addons, cmd = m.addons.Update(msg)
		return m, cmd

	case modeFetching:
		return m, nil

	case modeVersions:
		switch k {
		case "esc", "q":
			m.mode = modeBrowse
			return m, nil
		case "enter":
			v, ok := m.versions.SelectedItem().(versionItem)
			if !ok {
				return m, nil
			}
			m.pick = v
			m.mode = modeConfirm
			return m, nil
		}
		var cmd tea.Cmd
		m.versions, cmd = m.versions.Update(msg)
		return m, cmd

	case modeConfirm:
		switch k {
		case "y", "Y", "enter":
			m.mode = modeInstalling
			m.logs = nil
			return m.startInstall()
		case "n", "N", "esc":
			m.mode = modeVersions
			return m, nil
		}
		return m, nil

	case modeInstalling:
		return m, nil
	}
	return m, nil
}

func (m model) View() string {
	switch m.mode {
	case modeFetching:
		return fmt.Sprintf("\n  %s fetching versions for %s…\n", m.spinner.View(), m.selected.Name)

	case modeVersions:
		return m.versions.View() + "\n" + helpStyle.Render("enter select · esc back")

	case modeConfirm:
		return m.confirmView()

	case modeInstalling:
		return fmt.Sprintf("\n  %s installing %s…\n\n", m.spinner.View(), m.selected.Name) + m.logView()

	default: // modeBrowse
		out := m.addons.View() + "\n" + helpStyle.Render("↑/↓ navigate · enter versions · q quit") + "\n"
		if m.statusMsg != "" {
			out += statusStyle.Render(m.statusMsg) + "\n"
		}
		return out + m.logView()
	}
}

func (m model) confirmView() string {
	v := m.pick
	body := fmt.Sprintf("Install %s\n\n  version:  %s\n  asset:    %s\n  url:      %s\n\n  (y) confirm    (n) cancel",
		m.selected.Name, v.tag, v.asset.Name, v.asset.URL)
	return boxStyle.Render(body)
}

func (m model) logView() string {
	logs := m.logs
	if len(logs) > reserve-2 {
		logs = logs[len(logs)-(reserve-2):]
	}
	var b strings.Builder
	for _, l := range logs {
		b.WriteString(logStyle.Render(l) + "\n")
	}
	return b.String()
}

// ---------- commands ----------

func fetchReleases(url string) tea.Cmd {
	return func() tea.Msg {
		listing, err := source.AvailableVersions(context.Background(), url)
		return releasesMsg{listing: listing, err: err}
	}
}

func (m model) startInstall() (tea.Model, tea.Cmd) {
	m.events = make(chan installEvent)
	target := addon.Addon{Name: m.selected.Name, URL: m.pick.asset.URL, Path: m.selected.Path}

	go func(events chan installEvent) {
		report := func(format string, args ...any) {
			events <- installEvent{line: fmt.Sprintf(format, args...)}
		}
		err := addon.Install(target, m.projectRoot, report)
		events <- installEvent{done: true, err: err}
	}(m.events)

	return m, tea.Batch(m.spinner.Tick, waitForEvent(m.events))
}

func waitForEvent(events chan installEvent) tea.Cmd {
	return func() tea.Msg {
		return <-events
	}
}

// finishInstall pins the freshly installed version into the manifest and
// re-inspects so the list reflects the new state.
func (m model) finishInstall() tea.Cmd {
	manifestPath, projectRoot := m.manifestPath, m.projectRoot
	name, path, url := m.selected.Name, m.selected.Path, m.pick.asset.URL
	fallbackTag := strings.TrimPrefix(m.pick.tag, "v")

	return func() tea.Msg {
		fullPath, _ := filepath.Abs(filepath.Join(projectRoot, path))
		version := addon.LocalVersion(fullPath)
		if version == "" {
			version = fallbackTag
		}
		_ = addon.UpdateEntry(manifestPath, name, url, version)

		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			return refreshMsg{version: version}
		}
		return refreshMsg{statuses: statuses, version: version}
	}
}
