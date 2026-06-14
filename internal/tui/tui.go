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
	modeFetchingBranches
	modeSubmenu
	modeConfirm
	modeInstalling
	modeInstallingAll
)

// layout constants. headerHeight is the persistent context box above the list;
// maxOutputLines caps how tall the output pane grows before it starts scrolling.
const (
	headerHeight   = 5 // border (2) + 3 content lines
	maxOutputLines = 8
	footerSpacer   = 1 // blank line below the help bar so the layout bottom is static
)

// focusArea tracks which pane receives navigation keys.
type focusArea int

const (
	focusList focusArea = iota
	focusOutput
)

// ---------- list items ----------

// installAllItem is the entry pinned to the top of the browse list; selecting it
// runs the manifest install (the equivalent of the addon_install CLI).
type installAllItem struct{}

func (installAllItem) Title() string       { return "↧ Install / update all" }
func (installAllItem) FilterValue() string { return "install all" }
func (installAllItem) Description() string { return "download everything per the manifest" }

type item struct{ status addon.Status }

func (i item) Title() string       { return i.status.Addon.Name }
func (i item) FilterValue() string { return i.status.Addon.Name }

func (i item) Description() string {
	s := i.status
	switch s.State {
	case addon.StateInvalid:
		return "✗ invalid — missing url or path"
	case addon.StateMissing:
		if s.Addon.Version != "" {
			return "• not installed — target v" + s.Addon.Version
		}
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

// headItem is the top-of-list entry that opens the branch (refs/heads) submenu.
type headItem struct{}

func (headItem) Title() string       { return "HEAD" }
func (headItem) FilterValue() string { return "HEAD" }
func (headItem) Description() string { return "track a branch (refs/heads)" }

// releaseItem is one release in the top-level versions list. Selecting it either
// drops straight into confirm (single asset) or opens the asset submenu.
type releaseItem struct{ rel source.Release }

func (r releaseItem) Title() string       { return r.rel.Tag }
func (r releaseItem) FilterValue() string { return r.rel.Tag }

func (r releaseItem) Description() string {
	d := fmt.Sprintf("%d asset(s)", len(r.rel.Assets))
	if r.rel.Prerelease {
		d += " · prerelease"
	}
	return d
}

// versionItem is a leaf choice (a branch or a release asset) shown in a submenu
// and carried in m.pick through confirm/install.
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
	return v.asset.Name
}

func (v versionItem) Description() string {
	if v.branch {
		return "latest commit · " + v.asset.Name
	}
	d := v.tag
	if v.prerelease {
		d += " · prerelease"
	}
	return d
}

func (v versionItem) FilterValue() string { return v.tag + " " + v.asset.Name }

// versionTopItems builds the top-level versions list: HEAD first, then one entry
// per release (newest first).
func versionTopItems(l *source.Listing) []list.Item {
	items := []list.Item{headItem{}}
	for _, r := range l.Releases {
		items = append(items, releaseItem{rel: r})
	}
	return items
}

// assetItems builds the per-release asset submenu.
func assetItems(r source.Release) []list.Item {
	items := make([]list.Item, 0, len(r.Assets))
	for _, a := range r.Assets {
		items = append(items, versionItem{tag: r.Tag, asset: a, prerelease: r.Prerelease})
	}
	return items
}

// branchItems builds the HEAD/branch submenu.
func branchItems(branches []source.Asset) []list.Item {
	items := make([]list.Item, 0, len(branches))
	for _, b := range branches {
		items = append(items, versionItem{tag: b.Name, asset: b, branch: true})
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

type branchesMsg struct {
	branches []source.Asset
	err      error
}

type installedAllMsg struct {
	statuses []addon.Status
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
	submenu  list.Model
	spinner  spinner.Model
	output   viewport.Model
	focus    focusArea

	listing       *source.Listing
	selected      addon.Addon
	selectedLocal string // installed version of selected addon, for the versions title
	pick          versionItem

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
	// installAllItem is pinned first; the addons follow (offset by 1 — see
	// applyStatuses which writes back into the same layout).
	items := make([]list.Item, 0, len(statuses)+1)
	items = append(items, installAllItem{})
	for _, s := range statuses {
		items = append(items, item{status: s})
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
			key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "install all")),
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

// newSelectList builds a list styled like the others (no status bar, help drawn
// separately, esc/enter hints) for the versions and submenu screens.
func (m model) newSelectList(items []list.Item, title string) list.Model {
	l := list.New(items, list.NewDefaultDelegate(), m.width, m.listHeight())
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	keys := func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		}
	}
	l.AdditionalShortHelpKeys = keys
	l.AdditionalFullHelpKeys = keys
	return l
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	nm, cmd := m.update(msg)
	// Re-lay-out after every message: it's cheap and avoids chasing every spot
	// that changes content height (help expansion, log growth, mode switches).
	nm.refreshSizes()
	return nm, cmd
}

func (m model) update(msg tea.Msg) (model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case releasesMsg:
		if msg.err != nil {
			m.statusMsg = "error: " + msg.err.Error()
			m.mode = modeBrowse
			return m, nil
		}
		m.listing = msg.listing
		m.versions = m.newSelectList(versionTopItems(msg.listing), m.versionsTitle())
		m.mode = modeVersions
		return m, nil

	case branchesMsg:
		if msg.err != nil {
			m.statusMsg = "error: " + msg.err.Error()
			m.mode = modeBrowse
			return m, nil
		}
		if len(msg.branches) == 0 {
			m.statusMsg = "no branches found"
			m.mode = modeVersions
			return m, nil
		}
		m.submenu = m.newSelectList(branchItems(msg.branches), "Branches · "+m.selected.Name)
		m.mode = modeSubmenu
		return m, nil

	case installEvent:
		if !msg.done {
			m.appendLog(msg.line)
			return m, waitForEvent(m.events)
		}
		if m.mode == modeInstallingAll {
			return m, m.finishInstallAll()
		}
		if msg.err != nil {
			m.appendLog(fmt.Sprintf("[%s] error: %v", m.selected.Name, msg.err))
			m.statusMsg = "install failed"
			m.mode = modeBrowse
			return m, nil
		}
		m.appendLog(fmt.Sprintf("[%s] installed", m.selected.Name))
		return m, m.finishInstall()

	case installedAllMsg:
		m.applyStatuses(msg.statuses)
		m.statusMsg = "install complete"
		m.mode = modeBrowse
		return m, nil

	case refreshMsg:
		m.applyStatuses(msg.statuses)
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
	case modeSubmenu:
		m.submenu, cmd = m.submenu.Update(msg)
	}
	return m, cmd
}

// applyStatuses writes refreshed statuses back into the addons list, offset by 1
// to skip the pinned installAllItem.
func (m *model) applyStatuses(statuses []addon.Status) {
	for i, s := range statuses {
		idx := i + 1
		if idx < len(m.addons.Items()) {
			m.addons.SetItem(idx, item{status: s})
		}
	}
}

// versionsTitle is the header for the versions screen, e.g.
// "MyAddon - Current:v1.0.0 - Versions".
func (m model) versionsTitle() string {
	cur := "none"
	if m.selectedLocal != "" {
		cur = "v" + m.selectedLocal
	}
	return fmt.Sprintf("%s - Current:%s - Versions", m.selected.Name, cur)
}

func (m model) handleKey(msg tea.KeyMsg) (model, tea.Cmd) {
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
			return m, cmd
		}
		switch k {
		case "q":
			return m, tea.Quit
		case "i":
			return m.startInstallAll()
		case "enter":
			switch sel := m.addons.SelectedItem().(type) {
			case installAllItem:
				return m.startInstallAll()
			case item:
				if !sel.status.Installable() {
					return m, nil
				}
				m.selected = sel.status.Addon
				m.selectedLocal = sel.status.LocalVersion
				m.statusMsg = ""
				m.mode = modeFetching
				return m, tea.Batch(m.spinner.Tick, fetchReleases(m.selected.URL))
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.addons, cmd = m.addons.Update(msg)
		return m, cmd

	case modeFetching, modeFetchingBranches:
		return m, nil

	case modeVersions:
		if m.versions.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.versions, cmd = m.versions.Update(msg)
			return m, cmd
		}
		switch k {
		case "esc", "q":
			m.mode = modeBrowse
			return m, nil
		case "enter":
			return m.selectVersion()
		}
		var cmd tea.Cmd
		m.versions, cmd = m.versions.Update(msg)
		return m, cmd

	case modeSubmenu:
		if m.submenu.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.submenu, cmd = m.submenu.Update(msg)
			return m, cmd
		}
		switch k {
		case "esc", "q":
			m.mode = modeVersions
			return m, nil
		case "enter":
			v, ok := m.submenu.SelectedItem().(versionItem)
			if !ok {
				return m, nil
			}
			m.pick = v
			m.mode = modeConfirm
			return m, nil
		}
		var cmd tea.Cmd
		m.submenu, cmd = m.submenu.Update(msg)
		return m, cmd

	case modeConfirm:
		switch k {
		case "y", "Y", "enter":
			m.mode = modeInstalling
			return m.startInstall()
		case "n", "N", "esc":
			// Return to wherever the pick came from: branch picks and multi-asset
			// releases came via the submenu; single-asset releases came straight
			// from the versions list.
			if m.cameFromSubmenu() {
				m.mode = modeSubmenu
			} else {
				m.mode = modeVersions
			}
			return m, nil
		}
		return m, nil

	case modeInstalling, modeInstallingAll:
		return m, nil
	}
	return m, nil
}

// selectVersion handles an enter press on the top-level versions list: HEAD opens
// the branch submenu, a single-asset release goes straight to confirm, and a
// multi-asset release opens the asset submenu.
func (m model) selectVersion() (model, tea.Cmd) {
	switch sel := m.versions.SelectedItem().(type) {
	case headItem:
		m.mode = modeFetchingBranches
		return m, tea.Batch(m.spinner.Tick, fetchBranches(m.selected.URL))
	case releaseItem:
		if len(sel.rel.Assets) == 1 {
			m.pick = versionItem{tag: sel.rel.Tag, asset: sel.rel.Assets[0], prerelease: sel.rel.Prerelease}
			m.mode = modeConfirm
			return m, nil
		}
		m.submenu = m.newSelectList(assetItems(sel.rel), "Assets · "+sel.rel.Tag)
		m.mode = modeSubmenu
		return m, nil
	}
	return m, nil
}

// cameFromSubmenu reports whether the current pick was made in a submenu (a
// branch, or an asset from a multi-asset release) rather than a single-asset
// release chosen directly from the versions list.
func (m model) cameFromSubmenu() bool {
	if m.pick.branch {
		return true
	}
	if r, ok := m.versions.SelectedItem().(releaseItem); ok {
		return len(r.rel.Assets) > 1
	}
	return false
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
	if m.mode == modeBrowse {
		used += footerSpacer
	}
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
	case modeSubmenu:
		return lipgloss.Height(helpView(m.submenu))
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
	case modeSubmenu:
		return m.submenu.FilterState() == list.Filtering
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
	// versions/submenu are zero-value lists until built; only size them while
	// they're the active screen.
	if m.mode == modeVersions {
		m.versions.SetSize(m.width, lh)
	}
	if m.mode == modeSubmenu {
		m.submenu.SetSize(m.width, lh)
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

	case modeFetchingBranches:
		body = fmt.Sprintf("\n  %s fetching branches for %s…\n", m.spinner.View(), m.selected.Name)

	case modeVersions:
		body = m.versions.View() + "\n" + helpView(m.versions)

	case modeSubmenu:
		body = m.submenu.View() + "\n" + helpView(m.submenu)

	case modeConfirm:
		body = m.confirmView()

	case modeInstalling:
		body = fmt.Sprintf("\n  %s installing %s…\n", m.spinner.View(), m.selected.Name)
		if len(m.logs) > 0 {
			body += "\n" + m.outputView()
		}

	case modeInstallingAll:
		body = fmt.Sprintf("\n  %s installing all addons…\n", m.spinner.View())
		if len(m.logs) > 0 {
			body += "\n" + m.outputView()
		}

	default: // modeBrowse
		// Order bottom-up: list, then status, then output, then the help bar,
		// then a blank spacer so the bottom edge stays put.
		body = m.addons.View()
		if m.statusMsg != "" {
			body += "\n" + statusStyle.Render(m.statusMsg)
		}
		if len(m.logs) > 0 {
			body += "\n" + m.outputView()
		}
		body += "\n" + helpView(m.addons)
		body += strings.Repeat("\n", footerSpacer)
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

// outputContentHeight is the viewport height for the log. In browse it's a fixed
// region the log stretches to fill, so the list/output boundary doesn't shift as
// lines accumulate; while installing it tracks the streaming output instead.
func (m model) outputContentHeight() int {
	if m.mode == modeBrowse {
		return maxOutputLines
	}
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
	return len(m.logs) > 0 && (m.mode == modeBrowse || m.mode == modeInstalling || m.mode == modeInstallingAll)
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

func fetchBranches(url string) tea.Cmd {
	return func() tea.Msg {
		branches, err := source.Branches(context.Background(), url)
		return branchesMsg{branches: branches, err: err}
	}
}

// startInstallAll runs the manifest install (Inspect + InstallAll), the same as
// the addon_install CLI, streaming progress into the output pane.
func (m model) startInstallAll() (model, tea.Cmd) {
	m.mode = modeInstallingAll
	m.events = make(chan installEvent)
	manifestPath, projectRoot := m.manifestPath, m.projectRoot

	go func(events chan installEvent) {
		report := func(format string, args ...any) {
			events <- installEvent{line: fmt.Sprintf(format, args...)}
		}
		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			report("error: %v", err)
		} else {
			_ = addon.InstallAll(statuses, projectRoot, report)
		}
		events <- installEvent{done: true}
	}(m.events)

	return m, tea.Batch(m.spinner.Tick, waitForEvent(m.events))
}

func (m model) startInstall() (model, tea.Cmd) {
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
		// Record the concrete chosen url alongside the pinned version, so a later
		// Install-all (or the CLI) reinstalls exactly what was selected here.
		_ = addon.UpdateEntry(manifestPath, name, url, version)

		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			return refreshMsg{version: version}
		}
		return refreshMsg{statuses: statuses, version: version}
	}
}

// finishInstallAll re-inspects the manifest after a batch install so the list
// reflects the new states.
func (m model) finishInstallAll() tea.Cmd {
	manifestPath, projectRoot := m.manifestPath, m.projectRoot
	return func() tea.Msg {
		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			return installedAllMsg{}
		}
		return installedAllMsg{statuses: statuses}
	}
}
