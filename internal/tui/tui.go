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

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
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

// layout constants. headerHeight is the persistent context box above the list;
// maxOutputLines caps how tall the output pane grows before it starts scrolling.
const (
	headerHeight   = 5 // border (2) + 3 content lines
	maxOutputLines = 8
)

// focusArea tracks which pane receives navigation keys.
type focusArea int

const (
	focusList focusArea = iota
	focusOutput
)

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
	manifestRel  string
	projectName  string
	hasProject   bool
	width        int
	height       int

	mode     mode
	addons   list.Model
	versions list.Model
	spinner  spinner.Model
	output   viewport.Model
	focus    focusArea

	selected addon.Addon
	pick     versionItem

	events    chan installEvent
	logs      []string
	statusMsg string
}

var (
	statusStyle  = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(lipgloss.Color("212"))
	logStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	boxStyle     = lipgloss.NewStyle().Margin(1, 2).Padding(1, 2).Border(lipgloss.RoundedBorder())
	headerStyle  = lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240"))
	labelStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	borderColor  = lipgloss.Color("240")
	focusedColor = lipgloss.Color("212")
)

func newModel(manifestPath, projectRoot string, statuses []addon.Status) model {
	items := make([]list.Item, len(statuses))
	for i, s := range statuses {
		items[i] = item{status: s}
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Godot Addons"
	l.SetShowStatusBar(false)
	// Help is rendered manually below the status/output panes so it stays the
	// bottom-most element; see View.
	l.SetShowHelp(false)
	browseKeys := func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "versions")),
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "output")),
			key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear log")),
		}
	}
	l.AdditionalShortHelpKeys = browseKeys
	l.AdditionalFullHelpKeys = browseKeys

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	rel, err := filepath.Rel(projectRoot, manifestPath)
	if err != nil {
		rel = manifestPath
	}
	name, exists := addon.ProjectName(projectRoot)

	return model{
		manifestPath: manifestPath,
		projectRoot:  projectRoot,
		manifestRel:  rel,
		projectName:  name,
		hasProject:   exists,
		addons:       l,
		spinner:      sp,
		output:       viewport.New(0, 0),
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.refreshSizes()
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
		vl := list.New(items, list.NewDefaultDelegate(), m.width, m.listHeight())
		vl.Title = "Versions · " + m.selected.Name
		vl.SetShowStatusBar(false)
		vl.SetShowHelp(false)
		versionKeys := func() []key.Binding {
			return []key.Binding{
				key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
				key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
			}
		}
		vl.AdditionalShortHelpKeys = versionKeys
		vl.AdditionalFullHelpKeys = versionKeys
		m.versions = vl
		m.mode = modeVersions
		m.refreshSizes()
		return m, nil

	case installEvent:
		if !msg.done {
			m.appendLog(msg.line)
			return m, waitForEvent(m.events)
		}
		if msg.err != nil {
			m.appendLog(fmt.Sprintf("[%s] error: %v", m.selected.Name, msg.err))
			m.statusMsg = "install failed"
			m.mode = modeBrowse
			return m, nil
		}
		m.appendLog(fmt.Sprintf("[%s] installed", m.selected.Name))
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
		m.refreshSizes()
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
		m.refreshSizes()
	case modeVersions:
		m.versions, cmd = m.versions.Update(msg)
		m.refreshSizes()
	}
	return m, cmd
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		return m, tea.Quit
	}

	// When the output pane holds focus, navigation keys scroll it; everything
	// else either toggles back or clears.
	if m.focus == focusOutput {
		switch k {
		case "tab", "esc":
			m.focus = focusList
			return m, nil
		case "c":
			m.clearLogs()
			return m, nil
		case "q":
			return m, tea.Quit
		}
		var cmd tea.Cmd
		m.output, cmd = m.output.Update(msg)
		return m, cmd
	}

	// Global keys, available in any mode unless a filter input is capturing
	// text: tab jumps into the output pane, c clears the log.
	if !m.filtering() {
		switch k {
		case "tab":
			if m.outputVisible() {
				m.focus = focusOutput
				m.output.GotoBottom()
			}
			return m, nil
		case "c":
			m.clearLogs()
			return m, nil
		}
	}

	switch m.mode {
	case modeBrowse:
		// While the filter input is active, let the list consume keys so typing
		// "q" filters instead of quitting and enter applies the filter.
		if m.addons.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.addons, cmd = m.addons.Update(msg)
			m.refreshSizes()
			return m, cmd
		}
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
		m.refreshSizes()
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
		m.refreshSizes()
		return m, cmd

	case modeConfirm:
		switch k {
		case "y", "Y", "enter":
			m.mode = modeInstalling
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

// listHeight is the rows available to a list, leaving room for the header
// above and the status line / output pane below (each only when present).
func (m model) listHeight() int {
	used := headerHeight
	if m.statusMsg != "" {
		used++
	}
	used += m.outputBoxHeight()
	used += m.helpHeight()
	h := m.height - used
	if h < 1 {
		h = 1
	}
	return h
}

// helpView renders a list's help bar on its own, so it can be placed below the
// status and output panes.
func helpView(l list.Model) string {
	return l.Styles.HelpStyle.Render(l.Help.View(l))
}

// helpHeight is the row count of the active list's help bar (0 when no list is
// on screen).
func (m model) helpHeight() int {
	switch m.mode {
	case modeBrowse:
		return lipgloss.Height(helpView(m.addons))
	case modeVersions:
		return lipgloss.Height(helpView(m.versions))
	}
	return 0
}

// filtering reports whether the active list's filter input is capturing text,
// in which case global single-key shortcuts must not steal keystrokes.
func (m model) filtering() bool {
	switch m.mode {
	case modeBrowse:
		return m.addons.FilterState() == list.Filtering
	case modeVersions:
		return m.versions.FilterState() == list.Filtering
	}
	return false
}

// refreshSizes re-lays out the lists and output pane for the current window
// size and log volume. Safe to call before the first WindowSizeMsg.
func (m *model) refreshSizes() {
	if m.width == 0 {
		return
	}
	lh := m.listHeight()
	m.addons.SetSize(m.width, lh)
	// versions is a zero-value list until built in releasesMsg; only size it
	// while it's the active screen.
	if m.mode == modeVersions {
		m.versions.SetSize(m.width, lh)
	}
	m.output.Width = m.outputInnerWidth()
	m.output.Height = m.outputContentHeight()
	m.output.SetContent(m.logContent())
}

// appendLog records a line and keeps the output pane scrolled to the newest
// entry unless the user is actively scrolling it.
func (m *model) appendLog(line string) {
	m.logs = append(m.logs, line)
	m.refreshSizes()
	if m.focus != focusOutput {
		m.output.GotoBottom()
	}
}

// clearLogs empties the output pane and the status line, and returns focus to
// the list.
func (m *model) clearLogs() {
	m.logs = nil
	m.statusMsg = ""
	m.focus = focusList
	m.output.SetContent("")
	m.refreshSizes()
}

func (m model) View() string {
	var body string
	switch m.mode {
	case modeFetching:
		body = fmt.Sprintf("\n  %s fetching versions for %s…\n", m.spinner.View(), m.selected.Name)

	case modeVersions:
		body = m.versions.View() + "\n" + helpView(m.versions)

	case modeConfirm:
		body = m.confirmView()

	case modeInstalling:
		body = fmt.Sprintf("\n  %s installing %s…\n", m.spinner.View(), m.selected.Name)
		if len(m.logs) > 0 {
			body += "\n" + m.outputView()
		}

	default: // modeBrowse
		// Order bottom-up: list, then status, then output, then the help bar
		// as the final (lowest) element.
		body = m.addons.View()
		if m.statusMsg != "" {
			body += "\n" + statusStyle.Render(m.statusMsg)
		}
		if len(m.logs) > 0 {
			body += "\n" + m.outputView()
		}
		body += "\n" + helpView(m.addons)
	}

	return lipgloss.JoinVertical(lipgloss.Left, m.headerView(), body)
}

// headerView renders the persistent context box shown on every screen.
func (m model) headerView() string {
	name := "No Project File"
	if m.hasProject {
		name = m.projectName
		if name == "" {
			name = "(unnamed project)"
		}
	}

	inner := m.width - 4 // minus border (2) and padding (2)
	if inner < 20 {
		inner = 20
	}
	valWidth := inner - 10 // minus the "Manifest: " label

	line := func(label, value string) string {
		return labelStyle.Render(label) + truncLeft(value, valWidth)
	}
	body := strings.Join([]string{
		labelStyle.Render("Project:  ") + name,
		line("Root:     ", m.projectRoot),
		line("Manifest: ", m.manifestRel),
	}, "\n")

	return headerStyle.Width(inner).Render(body)
}

// truncLeft keeps the right (most informative) end of a path, prefixing "…".
func truncLeft(s string, max int) string {
	if max < 4 {
		max = 4
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return "…" + string(r[len(r)-(max-1):])
}

func (m model) confirmView() string {
	v := m.pick

	// Size the box to the screen and hard-wrap the (space-less) URL to fit.
	inner := m.width - 10
	if inner < 24 {
		inner = 24
	}
	urlBlock := indentLines(hardWrap(v.asset.URL, inner-4), "    ")

	body := fmt.Sprintf(
		"Install %s\n\n  version:  %s\n  asset:    %s\n  path:     %s\n  url:\n%s\n\n  (y) confirm    (n) cancel",
		m.selected.Name, v.tag, v.asset.Name, m.selected.Path, urlBlock)
	return boxStyle.Width(inner).Render(body)
}

// hardWrap breaks s into chunks of at most width runes (URLs have no spaces to
// word-wrap on, so we break unconditionally).
func hardWrap(s string, width int) string {
	if width < 8 {
		width = 8
	}
	r := []rune(s)
	var b strings.Builder
	for len(r) > width {
		b.WriteString(string(r[:width]))
		b.WriteByte('\n')
		r = r[width:]
	}
	b.WriteString(string(r))
	return b.String()
}

func indentLines(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

// outputInnerWidth is the text width inside the output box (full width minus
// the side borders and the 1-col padding on each side).
func (m model) outputInnerWidth() int {
	w := m.width - 2 - 2 - 2 // header margin parity, side borders, padding
	if w < 10 {
		w = 10
	}
	return w
}

// outputContentHeight is the number of log lines shown before scrolling kicks in.
func (m model) outputContentHeight() int {
	n := len(m.logs)
	if n > maxOutputLines {
		n = maxOutputLines
	}
	if n < 1 {
		n = 1
	}
	return n
}

// outputVisible reports whether the output pane is currently on screen.
func (m model) outputVisible() bool {
	return len(m.logs) > 0 && (m.mode == modeBrowse || m.mode == modeInstalling)
}

// outputBoxHeight is the total rows the output pane occupies (content + the top
// and bottom border lines), or 0 when there is nothing to show.
func (m model) outputBoxHeight() int {
	if !m.outputVisible() {
		return 0
	}
	return m.outputContentHeight() + 2
}

// logContent renders the log lines for the viewport.
func (m model) logContent() string {
	var b strings.Builder
	for i, l := range m.logs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(logStyle.Render(l))
	}
	return b.String()
}

// outputView draws the scrollable log inside a bordered box whose top edge is
// interrupted by an "Output" legend (and a scroll hint while focused).
func (m model) outputView() string {
	color := borderColor
	label := "Output"
	if m.focus == focusOutput {
		color = focusedColor
		label = "Output · ↑/↓ scroll · tab/esc back"
	}

	inner := m.outputInnerWidth()
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderTop(false).
		BorderForeground(color).
		Padding(0, 1).
		Width(inner + 2) // inner text + the 1-col padding on each side
	content := box.Render(m.output.View())

	// Hand-draw the top border so the legend can sit mid-line. The run between
	// the corners is the same width as the bottom border: inner + 2 (padding).
	legend := "─ " + label + " "
	fill := (inner + 2) - lipgloss.Width(legend)
	if fill < 0 {
		fill = 0
	}
	top := lipgloss.NewStyle().Foreground(color).
		Render("┌" + legend + strings.Repeat("─", fill) + "┐")

	return top + "\n" + content
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
