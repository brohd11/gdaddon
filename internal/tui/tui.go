// Package tui implements the interactive bubbletea front-end for browsing
// addons, picking a remote version, and installing/updating. It renders state
// from the addon package and turns install progress into bubbletea messages.
package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
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
	modeActions
	modeFetching
	modeVersions
	modeFetchingBranches
	modeSubmenu
	modeConfirm
	modeNewPluginInput
	modeNewPluginConfirm
	modeImport
	modeInstalling
	modeInstallingAll
)

// add targets for the New Plugin target toggle.
const (
	targetProject = iota
	targetGlobal
)

// rows of the New Plugin form (url/name/path text fields + the target toggle).
// URL is first because it's the only mandatory field.
const (
	fldURL = iota
	fldName
	fldPath
	fldTarget
	fldCount
)

// headerHeight is the persistent context box above the list.
const headerHeight = 5 // border (2) + 3 content lines

// focusArea tracks which pane receives navigation keys.
type focusArea int

const (
	focusList focusArea = iota
	focusOutput
)

// ---------- list items ----------

// menuItem is the entry pinned to the top of the browse list; selecting it opens
// the Actions submenu (install all, add plugin, future config).
type menuItem struct{}

func (menuItem) Title() string       { return "☰ Actions" }
func (menuItem) FilterValue() string { return "actions menu" }
func (menuItem) Description() string { return "install all · new / import plugin" }

// actionKind identifies a row in the Actions submenu.
type actionKind int

const (
	actInstallAll actionKind = iota
	actNewPlugin
	actImportPlugin
)

// actionItem is one row in the Actions submenu.
type actionItem struct {
	title string
	desc  string
	kind  actionKind
}

func (a actionItem) Title() string       { return a.title }
func (a actionItem) FilterValue() string { return a.title }
func (a actionItem) Description() string { return a.desc }

// actionItems builds the Actions submenu rows.
func actionItems() []list.Item {
	return []list.Item{
		actionItem{title: "↧ Install / update all", desc: "download everything per the manifest", kind: actInstallAll},
		actionItem{title: "+ New Plugin", desc: "add a plugin to the project or your global list", kind: actNewPlugin},
		actionItem{title: "⬇ Import Plugin", desc: "add a plugin from your global list", kind: actImportPlugin},
	}
}

// importItem is one row in the Import Plugin picker (an entry from the global
// list); selecting it copies the entry into the project manifest.
type importItem struct {
	name string
	url  string
	path string
}

func (i importItem) Title() string       { return i.name }
func (i importItem) FilterValue() string { return i.name }
func (i importItem) Description() string { return i.url }

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
	line    string
	done    bool
	err     error
	path    string // resolved install path (single-install done event)
	version string // version read from the installed plugin.cfg
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

	mode      mode
	addons    list.Model
	actions   list.Model
	versions  list.Model
	submenu   list.Model
	imports   list.Model
	spinner   spinner.Model
	output    viewport.Model
	inputs    []textinput.Model // New Plugin form fields: name, url, path
	formFocus int               // focused row: fldName/fldURL/fldPath/fldTarget
	focus     focusArea

	listing       *source.Listing
	selected      addon.Addon
	selectedLocal string // installed version of selected addon, for the versions title
	pick          versionItem

	installedPath    string // resolved path from the last single install, for finishInstall
	installedVersion string // version from the last single install's plugin.cfg

	pendingName string // New Plugin: name (entered or derived) awaiting confirm
	pendingURL  string // New Plugin: normalized url awaiting confirm
	pendingPath string // New Plugin: optional explicit path awaiting confirm
	addTarget   int    // New Plugin target toggle: targetProject / targetGlobal

	events    chan installEvent
	logs      []string
	statusMsg string
}

var (
	// mutedColor is the secondary/muted gray (borders, labels, help, list
	// descriptions); logColor is brighter, near-white, for the output log text.
	mutedColor   = lipgloss.Color("247")
	logColor     = lipgloss.Color("252")
	borderColor  = lipgloss.Color("245")
	focusedColor = lipgloss.Color("212")

	statusStyle = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(focusedColor)
	logStyle    = lipgloss.NewStyle().Foreground(logColor)
	boxStyle    = lipgloss.NewStyle().Margin(1, 2).Padding(1, 2).Border(lipgloss.RoundedBorder())
	headerStyle = lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder()).BorderForeground(borderColor)
	labelStyle  = lipgloss.NewStyle().Foreground(mutedColor)
)

// addonListItems builds the browse list contents: the pinned Actions menu first,
// then one row per addon (so addon index i lives at list index i+1 — see
// applyStatuses).
func addonListItems(statuses []addon.Status) []list.Item {
	items := make([]list.Item, 0, len(statuses)+1)
	items = append(items, menuItem{})
	for _, s := range statuses {
		items = append(items, item{status: s})
	}
	return items
}

func newModel(manifestPath, projectRoot string, statuses []addon.Status) model {
	l := list.New(addonListItems(statuses), newDelegate(), 0, 0)
	l.Title = "Godot Addons"
	styleList(&l)
	// The browse short help is rendered custom (see browseHelpView) to stay
	// uncluttered; these extras only show in the full (?) help.
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "output")),
			key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear log")),
		}
	}

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
	l := list.New(items, newDelegate(), m.width, m.listHeight())
	l.Title = title
	styleList(&l)
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

// newDelegate is the shared list delegate with brightened description text.
func newDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(mutedColor)
	d.Styles.DimmedDesc = d.Styles.DimmedDesc.Foreground(mutedColor)
	return d
}

// styleList applies the shared list config: hide the built-in status bar and
// help (help is drawn manually at the bottom), and brighten the help colors.
func styleList(l *list.Model) {
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Help.Styles.ShortKey = l.Help.Styles.ShortKey.Foreground(mutedColor)
	l.Help.Styles.ShortDesc = l.Help.Styles.ShortDesc.Foreground(mutedColor)
	l.Help.Styles.ShortSeparator = l.Help.Styles.ShortSeparator.Foreground(mutedColor)
	l.Help.Styles.FullKey = l.Help.Styles.FullKey.Foreground(mutedColor)
	l.Help.Styles.FullDesc = l.Help.Styles.FullDesc.Foreground(mutedColor)
	l.Help.Styles.FullSeparator = l.Help.Styles.FullSeparator.Foreground(mutedColor)
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
		m.versions = m.newSelectList(versionTopItems(msg.listing), m.headerTitle("Versions"))
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
		m.submenu = m.newSelectList(branchItems(msg.branches), m.headerTitle("Branches"))
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
		m.installedPath = msg.path
		m.installedVersion = msg.version
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
	case modeImport:
		m.imports, cmd = m.imports.Update(msg)
	case modeNewPluginInput:
		if m.formFocus != fldTarget {
			m.inputs[m.formFocus], cmd = m.inputs[m.formFocus].Update(msg)
		}
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

// headerTitle is the shared header for the selected addon's screens, e.g.
// "MyAddon - Current:v1.0.0 - Versions" (section being Versions/Branches/Assets…).
// An empty section yields just "MyAddon - Current:v1.0.0".
func (m model) headerTitle(section string) string {
	cur := "none"
	if m.selectedLocal != "" {
		cur = "v" + m.selectedLocal
	}
	base := fmt.Sprintf("%s - Current:%s", m.selected.Name, cur)
	if section == "" {
		return base
	}
	return base + " - " + section
}

// crumb renders the addon breadcrumb as a list-title-styled bar, so screens
// without their own list title (fetching, confirm) keep a consistent header.
func (m model) crumb(section string) string {
	return m.addons.Styles.TitleBar.Render(m.addons.Styles.Title.Render(m.headerTitle(section)))
}

// titleBar renders a plain list-title-styled bar for screens that aren't tied to
// a selected addon (New Plugin / Import), keeping a consistent header.
func (m model) titleBar(text string) string {
	return m.addons.Styles.TitleBar.Render(m.addons.Styles.Title.Render(text))
}

// pickSection describes the chosen asset for the confirm breadcrumb, e.g.
// "Assets v1.0.0 - addon.zip" or "Branches - main".
func (m model) pickSection() string {
	if m.pick.branch {
		return "Branches - " + m.pick.tag
	}
	return fmt.Sprintf("Assets %s - %s", m.pick.tag, m.pick.asset.Name)
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
		case "enter":
			switch sel := m.addons.SelectedItem().(type) {
			case menuItem:
				m.actions = m.newSelectList(actionItems(), "Actions")
				m.mode = modeActions
				return m, nil
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

	case modeActions:
		if m.actions.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.actions, cmd = m.actions.Update(msg)
			return m, cmd
		}
		switch k {
		case "esc", "q":
			m.mode = modeBrowse
			return m, nil
		case "enter":
			a, ok := m.actions.SelectedItem().(actionItem)
			if !ok {
				return m, nil
			}
			switch a.kind {
			case actInstallAll:
				return m.startInstallAll()
			case actNewPlugin:
				return m.startNewPlugin()
			case actImportPlugin:
				return m.startImport()
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.actions, cmd = m.actions.Update(msg)
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

	case modeNewPluginInput:
		switch k {
		case "esc":
			m.mode = modeActions
			return m, nil
		case "up", "shift+tab":
			m.formFocus = (m.formFocus - 1 + fldCount) % fldCount
			return m, m.syncFormFocus()
		case "down", "tab":
			m.formFocus = (m.formFocus + 1) % fldCount
			return m, m.syncFormFocus()
		case "left", "right", "h", "l":
			// On the target row these toggle Project↔Global; on text rows they fall
			// through to the input (cursor movement / literal characters).
			if m.formFocus == fldTarget {
				m.addTarget = (m.addTarget + 1) % 2
				return m, nil
			}
		case "enter":
			url := strings.TrimSpace(m.inputs[fldURL].Value())
			if url == "" {
				m.formFocus = fldURL
				return m, m.syncFormFocus()
			}
			m.pendingURL = addon.NormalizeRepoURL(url)
			if name := strings.TrimSpace(m.inputs[fldName].Value()); name != "" {
				m.pendingName = name
			} else {
				m.pendingName = addon.DeriveName(url)
			}
			m.pendingPath = strings.TrimSpace(m.inputs[fldPath].Value())
			m.mode = modeNewPluginConfirm
			return m, nil
		}
		if m.formFocus == fldTarget {
			return m, nil
		}
		var cmd tea.Cmd
		m.inputs[m.formFocus], cmd = m.inputs[m.formFocus].Update(msg)
		return m, cmd

	case modeNewPluginConfirm:
		switch k {
		case "left", "h", "right", "l":
			if m.addTarget == targetProject {
				m.addTarget = targetGlobal
			} else {
				m.addTarget = targetProject
			}
			return m, nil
		case "esc":
			m.mode = modeNewPluginInput
			return m, nil
		case "enter":
			return m.commitNewPlugin()
		}
		return m, nil

	case modeImport:
		if m.imports.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.imports, cmd = m.imports.Update(msg)
			return m, cmd
		}
		switch k {
		case "esc", "q":
			m.mode = modeBrowse
			return m, nil
		case "enter":
			return m.commitImport()
		}
		var cmd tea.Cmd
		m.imports, cmd = m.imports.Update(msg)
		return m, cmd

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
		m.submenu = m.newSelectList(assetItems(sel.rel), m.headerTitle("Assets "+sel.rel.Tag))
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

// browseHelpView renders a decluttered short help for the browse screen
// (navigation + select + quit + more); filter, output, and clear-log live only
// in the full (?) help. The full help is unchanged.
func (m model) browseHelpView() string {
	l := m.addons
	if l.Help.ShowAll {
		return l.Styles.HelpStyle.Render(l.Help.FullHelpView(l.FullHelp()))
	}
	short := []key.Binding{
		l.KeyMap.CursorUp,
		l.KeyMap.CursorDown,
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		l.KeyMap.Quit,
		l.KeyMap.ShowFullHelp,
	}
	return l.Styles.HelpStyle.Render(l.Help.ShortHelpView(short))
}

// helpBar is the always-visible bottom bar. Interactive list screens show their
// real key help; confirm and non-interactive screens render in the same help
// format (key/desc styling, • separator) so the bar — and the layout — stays put.
func (m model) helpBar() string {
	switch m.mode {
	case modeBrowse:
		return m.browseHelpView()
	case modeActions:
		return helpView(m.actions)
	case modeVersions:
		return helpView(m.versions)
	case modeSubmenu:
		return helpView(m.submenu)
	case modeImport:
		return helpView(m.imports)
	case modeConfirm:
		return m.staticHelp(m.addons.Help.ShortHelpView([]key.Binding{
			key.NewBinding(key.WithKeys("y", "enter"), key.WithHelp("y/enter", "confirm")),
			key.NewBinding(key.WithKeys("n", "esc"), key.WithHelp("n/esc", "cancel")),
		}))
	case modeNewPluginInput:
		return m.staticHelp(m.addons.Help.ShortHelpView([]key.Binding{
			key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "field")),
			key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "target")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "next")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}))
	case modeNewPluginConfirm:
		return m.staticHelp(m.addons.Help.ShortHelpView([]key.Binding{
			key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "target")),
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "add")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		}))
	default: // fetching / installing — non-interactive
		return m.staticHelp(m.addons.Help.Styles.ShortDesc.Render("non-interactive · working…"))
	}
}

// staticHelp wraps a pre-rendered help string in the list's HelpStyle so it
// aligns with the real help bars.
func (m model) staticHelp(s string) string {
	return m.addons.Styles.HelpStyle.Render(s)
}

// helpHeight is the row count of the (always-visible) help bar.
func (m model) helpHeight() int {
	return lipgloss.Height(m.helpBar())
}

// filtering reports whether the active list's filter input is capturing text,
// in which case global single-key shortcuts must not steal keystrokes.
func (m model) filtering() bool {
	switch m.mode {
	case modeBrowse:
		return m.addons.FilterState() == list.Filtering
	case modeActions:
		return m.actions.FilterState() == list.Filtering
	case modeVersions:
		return m.versions.FilterState() == list.Filtering
	case modeSubmenu:
		return m.submenu.FilterState() == list.Filtering
	case modeImport:
		return m.imports.FilterState() == list.Filtering
	case modeNewPluginInput:
		// The URL text input captures keys, so the global tab/c shortcuts must
		// not steal characters typed into it.
		return true
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
	// actions/versions/submenu are zero-value lists until built; only size them
	// while they're the active screen.
	if m.mode == modeActions {
		m.actions.SetSize(m.width, lh)
	}
	if m.mode == modeVersions {
		m.versions.SetSize(m.width, lh)
	}
	if m.mode == modeSubmenu {
		m.submenu.SetSize(m.width, lh)
	}
	if m.mode == modeImport {
		m.imports.SetSize(m.width, lh)
	}
	if m.mode == modeNewPluginInput {
		w := m.confirmWidth() - 12 // box room minus the label column
		if w < 10 {
			w = 10
		}
		for i := range m.inputs {
			m.inputs[i].Width = w
		}
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
	body := m.bodyView()
	// Pad the body so the always-visible help bar sits at the very bottom.
	if pad := (m.height - headerHeight - m.helpHeight()) - lipgloss.Height(body); pad > 0 {
		body = lipgloss.JoinVertical(lipgloss.Left, body, blanks(pad))
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.headerView(), body, m.helpBar())
}

// bodyView renders the mode-specific content between the header and the help
// bar (which View appends).
func (m model) bodyView() string {
	switch m.mode {
	case modeActions:
		return m.actions.View()

	case modeFetching:
		return lipgloss.JoinVertical(lipgloss.Left, m.crumb(""),
			fmt.Sprintf("  %s fetching versions…", m.spinner.View()))

	case modeFetchingBranches:
		return lipgloss.JoinVertical(lipgloss.Left, m.crumb(""),
			fmt.Sprintf("  %s fetching branches…", m.spinner.View()))

	case modeVersions:
		return m.versions.View()

	case modeSubmenu:
		return m.submenu.View()

	case modeConfirm:
		return lipgloss.JoinVertical(lipgloss.Left, m.crumb(m.pickSection()), m.confirmView())

	case modeImport:
		return m.imports.View()

	case modeNewPluginInput:
		return lipgloss.JoinVertical(lipgloss.Left, m.titleBar("New Plugin"), m.newPluginFormView())

	case modeNewPluginConfirm:
		return lipgloss.JoinVertical(lipgloss.Left, m.titleBar("New Plugin"), m.newPluginConfirmView())

	case modeInstalling, modeInstallingAll:
		label := "installing all addons…"
		if m.mode == modeInstalling {
			label = "installing " + m.selected.Name + "…"
		}
		progress := fmt.Sprintf("\n  %s %s", m.spinner.View(), label)
		if !m.outputVisible() {
			return progress
		}
		// Push the output box to the bottom (just above the help bar) with a
		// flexible filler, so it sits cleanly at the bottom and grows upward as
		// lines stream in.
		out := m.outputView()
		filler := (m.height - headerHeight - m.helpHeight()) - lipgloss.Height(progress) - lipgloss.Height(out)
		if filler < 1 {
			filler = 1
		}
		return lipgloss.JoinVertical(lipgloss.Left, progress, blanks(filler), out)

	default: // modeBrowse
		// Order bottom-up: list, then status, then output.
		body := m.addons.View()
		if m.statusMsg != "" {
			body = lipgloss.JoinVertical(lipgloss.Left, body, statusStyle.Render(m.statusMsg))
		}
		if len(m.logs) > 0 {
			body = lipgloss.JoinVertical(lipgloss.Left, body, m.outputView())
		}
		return body
	}
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

// confirmWidth is the inner width of the boxed confirm/input screens, sized to
// the terminal with a sane floor.
func (m model) confirmWidth() int {
	inner := m.width - 10
	if inner < 24 {
		inner = 24
	}
	return inner
}

func (m model) confirmView() string {
	v := m.pick

	// Size the box to the screen and hard-wrap the (space-less) URL to fit.
	inner := m.confirmWidth()
	urlBlock := indentLines(hardWrap(v.asset.URL, inner-4), "    ")

	body := fmt.Sprintf(
		"Install %s\n\n  version:  %s\n  asset:    %s\n  path:     %s\n  url:\n%s",
		m.selected.Name, v.tag, v.asset.Name, m.selected.Path, urlBlock)
	return boxStyle.Width(inner).Render(body)
}

// targetToggle renders the Project◄ ►Global switch with the active side
// highlighted.
func (m model) targetToggle() string {
	active := lipgloss.NewStyle().Foreground(focusedColor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(mutedColor)
	project, global := dim.Render("Project"), dim.Render("Global")
	if m.addTarget == targetProject {
		project = active.Render("Project")
	} else {
		global = active.Render("Global")
	}
	return fmt.Sprintf("%s  ◄ ►  %s", project, global)
}

// newPluginFormView renders the single-page New Plugin form: name/url/path text
// fields and the target toggle, with a ▸ marker on the focused row.
func (m model) newPluginFormView() string {
	inner := m.confirmWidth()
	label := lipgloss.NewStyle().Foreground(mutedColor)
	marker := func(focused bool) string {
		if focused {
			return lipgloss.NewStyle().Foreground(focusedColor).Render("▸ ")
		}
		return "  "
	}
	field := func(row int, lbl string) string {
		return marker(m.formFocus == row) + label.Render(lbl) + m.inputs[row].View()
	}

	body := strings.Join([]string{
		"Add plugin",
		"",
		field(fldURL, "URL:     "),
		field(fldName, "Name:    "),
		field(fldPath, "Path:    "),
		"",
		marker(m.formFocus == fldTarget) + label.Render("Add to:  ") + m.targetToggle(),
	}, "\n")
	return boxStyle.Width(inner).Render(body)
}

// newPluginConfirmView reviews the entered values and the target before writing.
func (m model) newPluginConfirmView() string {
	inner := m.confirmWidth()
	urlBlock := indentLines(hardWrap(m.pendingURL, inner-4), "    ")

	path := m.pendingPath
	if path == "" {
		path = "(derived on install)"
	}
	body := fmt.Sprintf(
		"Add plugin\n\n  name:     %s\n  url:\n%s\n  path:     %s\n\n  add to:   %s",
		m.pendingName, urlBlock, path, m.targetToggle())
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

// blanks returns an n-line block of empty lines (height n) for use as a flexible
// filler/spacer in JoinVertical stacks.
func blanks(n int) string {
	if n < 1 {
		return ""
	}
	return strings.Repeat("\n", n-1)
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

// outputContentHeight is the viewport height for the log: a fixed ~25% of the
// terminal height, the same in every mode, so the log stretches to fill a stable
// region (and scrolls past it) instead of growing line by line.
func (m model) outputContentHeight() int {
	n := m.height / 4
	if n < 3 {
		n = 3
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
			_ = addon.InstallAll(manifestPath, statuses, projectRoot, report)
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
		res, err := addon.Install(target, m.projectRoot, report)
		events <- installEvent{done: true, err: err, path: res.Path, version: res.Version}
	}(m.events)

	return m, tea.Batch(m.spinner.Tick, waitForEvent(m.events))
}

func waitForEvent(events chan installEvent) tea.Cmd {
	return func() tea.Msg {
		return <-events
	}
}

// finishInstall pins the freshly installed url, resolved path, and version into
// the manifest and re-inspects so the list reflects the new state.
func (m model) finishInstall() tea.Cmd {
	manifestPath, projectRoot := m.manifestPath, m.projectRoot
	name, url := m.selected.Name, m.pick.asset.URL
	path := m.installedPath
	version := m.installedVersion
	if version == "" {
		version = strings.TrimPrefix(m.pick.tag, "v")
	}

	return func() tea.Msg {
		// Record the concrete chosen url alongside the resolved path + version, so
		// a later Install-all (or the CLI) reinstalls exactly what was selected.
		_ = addon.UpdateEntry(manifestPath, name, url, path, version)

		statuses, err := addon.Inspect(manifestPath, projectRoot)
		if err != nil {
			return refreshMsg{version: version}
		}
		return refreshMsg{statuses: statuses, version: version}
	}
}

// startNewPlugin opens the New Plugin form (name/url/path + target toggle).
func (m model) startNewPlugin() (model, tea.Cmd) {
	mk := func(placeholder string) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.Prompt = "" // labels are rendered separately in the form view
		return ti
	}
	// Order matches the fld* indices: url, name, path.
	m.inputs = []textinput.Model{
		mk("https://github.com/owner/repo"),
		mk("(optional — derived from url)"),
		mk("(optional — derived on install)"),
	}
	m.addTarget = targetProject
	m.formFocus = fldURL
	cmd := m.syncFormFocus()
	m.statusMsg = ""
	m.mode = modeNewPluginInput
	return m, cmd
}

// syncFormFocus focuses the textinput at formFocus and blurs the rest (the
// target row focuses none), returning the cursor-blink command.
func (m *model) syncFormFocus() tea.Cmd {
	var cmd tea.Cmd
	for i := range m.inputs {
		if i == m.formFocus {
			cmd = m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
	return cmd
}

// commitNewPlugin writes the pending entry to the project manifest or the global
// list (per addTarget), refreshes the browse list when it touched the project,
// and returns to browse with a status line. Both are quick local file writes.
func (m model) commitNewPlugin() (model, tea.Cmd) {
	name, url, path := m.pendingName, m.pendingURL, m.pendingPath
	if m.addTarget == targetGlobal {
		globalPath, err := addon.GlobalListPath()
		if err == nil {
			err = addon.AddEntry(globalPath, name, url, path)
		}
		if err != nil {
			m.statusMsg = "error: " + err.Error()
		} else {
			m.statusMsg = fmt.Sprintf("added %s to global list", name)
		}
		m.mode = modeBrowse
		return m, nil
	}

	if err := addon.AddEntry(m.manifestPath, name, url, path); err != nil {
		m.statusMsg = "error: " + err.Error()
		m.mode = modeBrowse
		return m, nil
	}
	m.reloadAddons()
	m.statusMsg = "added " + name
	m.mode = modeBrowse
	return m, nil
}

// startImport loads the global plugin list and opens the picker, or reports an
// empty/missing list and returns to browse.
func (m model) startImport() (model, tea.Cmd) {
	path, err := addon.GlobalListPath()
	var addons []addon.Addon
	if err == nil {
		addons, err = addon.Parse(path)
	}
	if err != nil || len(addons) == 0 {
		m.statusMsg = "no global plugins yet — add one via New Plugin → Global"
		m.mode = modeBrowse
		return m, nil
	}
	items := make([]list.Item, 0, len(addons))
	for _, a := range addons {
		items = append(items, importItem{name: a.Name, url: a.URL, path: a.Path})
	}
	m.imports = m.newSelectList(items, "Import Plugin")
	m.statusMsg = ""
	m.mode = modeImport
	return m, nil
}

// commitImport copies the selected global entry into the project manifest
// (deriving the install path), refreshes the list, and returns to browse.
func (m model) commitImport() (model, tea.Cmd) {
	sel, ok := m.imports.SelectedItem().(importItem)
	if !ok {
		return m, nil
	}
	if err := addon.AddEntry(m.manifestPath, sel.name, sel.url, sel.path); err != nil {
		m.statusMsg = "error: " + err.Error()
		m.mode = modeBrowse
		return m, nil
	}
	m.reloadAddons()
	m.statusMsg = "imported " + sel.name
	m.mode = modeBrowse
	return m, nil
}

// reloadAddons re-inspects the manifest and rebuilds the browse list, so newly
// added rows appear (SetItems handles the row-count change, unlike applyStatuses).
func (m *model) reloadAddons() {
	statuses, err := addon.Inspect(m.manifestPath, m.projectRoot)
	if err != nil {
		return
	}
	m.addons.SetItems(addonListItems(statuses))
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
