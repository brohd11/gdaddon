// Package newplugin is the shared "Add Plugin" flow: the url/name/path form, its
// confirm screen, and the commit that writes the entry to the project manifest or
// the global list. It lives outside any single tab because more than one tab opens
// it — the Actions tab ("New Plugin") and the Search tab (with the URL prefilled
// from a chosen asset). It sits in the flows layer between components and tabs
// (core ← components ← flows ← tabs ← tui), so tabs compose it without importing
// each other.
package newplugin

import (
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// add targets for the target toggle.
const (
	targetProject = iota
	targetGlobal
)

// rows of the form (url/name/path text fields + the target toggle). URL is first
// because it's the only mandatory field.
const (
	fldURL = iota
	fldName
	fldPath
	fldTarget
	fldCount
)

// NewPluginForm is the single-page Add Plugin form: url/name/path text fields and
// the Project/Global target toggle. On enter it pushes the confirm screen.
type NewPluginForm struct {
	inputs    []textinput.Model
	formFocus int
	addTarget int
}

var _ core.Filterer = (*NewPluginForm)(nil)

// NewNewPluginForm builds an empty Add Plugin form (focus on the URL field).
func NewNewPluginForm() *NewPluginForm { return NewWithURL("") }

// NewWithURL builds the Add Plugin form with the URL prefilled (focus jumps to the
// Name field, since the URL is already known). An empty url behaves like
// NewNewPluginForm. The Search tab uses this to hand off a chosen asset's repo URL.
func NewWithURL(url string) *NewPluginForm {
	mk := func(placeholder string) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.Prompt = "" // labels are rendered separately in the form view
		return ti
	}
	// Order matches the fld* indices: url, name, path.
	f := &NewPluginForm{
		inputs: []textinput.Model{
			mk("https://github.com/owner/repo"),
			mk("(optional — derived from url)"),
			mk("(optional — derived on install)"),
		},
		addTarget: targetProject,
		formFocus: fldURL,
	}
	if url != "" {
		f.inputs[fldURL].SetValue(url)
		f.formFocus = fldName
	}
	return f
}

func (s *NewPluginForm) Init(*core.Shared) tea.Cmd { return s.syncFormFocus() }

// filtering: the text inputs capture keys, so the global tab/c shortcuts must not
// steal characters typed into them.
func (s *NewPluginForm) Filtering() bool { return true }

func (s *NewPluginForm) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	k := key.String()
	switch {
	case core.MatchKey(k, core.Keys.Back):
		return s, core.Pop()
	case core.MatchKey(k, core.Keys.PrevField):
		s.formFocus = (s.formFocus - 1 + fldCount) % fldCount
		return s, s.syncFormFocus()
	case core.MatchKey(k, core.Keys.NextField):
		s.formFocus = (s.formFocus + 1) % fldCount
		return s, s.syncFormFocus()
	case core.MatchKey(k, core.Keys.Left), core.MatchKey(k, core.Keys.Right):
		// On the target row these toggle Project↔Global; on text rows they fall
		// through to the input (cursor movement / literal characters).
		if s.formFocus == fldTarget {
			s.addTarget = otherTarget(s.addTarget)
			return s, nil
		}
	case core.MatchKey(k, core.Keys.Select):
		url := strings.TrimSpace(s.inputs[fldURL].Value())
		if url == "" {
			s.formFocus = fldURL
			return s, s.syncFormFocus()
		}
		name := strings.TrimSpace(s.inputs[fldName].Value())
		if name == "" {
			name = addon.DeriveName(url)
		}
		path := strings.TrimSpace(s.inputs[fldPath].Value())
		return s, core.Push(newNewPluginConfirm(name, addon.NormalizeRepoURL(url), path, s.addTarget))
	}
	if s.formFocus == fldTarget {
		return s, nil
	}
	var cmd tea.Cmd
	s.inputs[s.formFocus], cmd = s.inputs[s.formFocus].Update(msg)
	return s, cmd
}

// syncFormFocus focuses the textinput at formFocus and blurs the rest (the target
// row focuses none), returning the cursor-blink command.
func (s *NewPluginForm) syncFormFocus() tea.Cmd {
	var cmd tea.Cmd
	for i := range s.inputs {
		if i == s.formFocus {
			cmd = s.inputs[i].Focus()
		} else {
			s.inputs[i].Blur()
		}
	}
	return cmd
}

func (s *NewPluginForm) View(sh *core.Shared) string {
	label := lipgloss.NewStyle().Foreground(core.MutedColor)
	marker := func(focused bool) string {
		if focused {
			return lipgloss.NewStyle().Foreground(core.FocusedColor).Render("▸ ")
		}
		return "  "
	}
	field := func(row int, lbl string) string {
		return marker(s.formFocus == row) + label.Render(lbl) + s.inputs[row].View()
	}

	body := strings.Join([]string{
		"Add plugin",
		"",
		field(fldURL, "URL:     "),
		field(fldName, "Name:    "),
		field(fldPath, "Path:    "),
		"",
		marker(s.formFocus == fldTarget) + label.Render("Add to:  ") + targetToggle(s.addTarget),
	}, "\n")
	return lipgloss.JoinVertical(lipgloss.Left,
		core.RenderTitleBar("New Plugin"),
		sh.Box(body))
}

func (s *NewPluginForm) HelpView(sh *core.Shared) string {
	return sh.BindingHelp([]key.Binding{
		core.Hint("field", core.Keys.PrevField, core.Keys.NextField),
		core.Hint("target", core.Keys.Left, core.Keys.Right),
		core.Hint("next", core.Keys.Select),
		core.Hint("cancel", core.Keys.Back),
	})
}

func (s *NewPluginForm) SetSize(sh *core.Shared, width, bodyHeight int) {
	w := sh.ConfirmWidth() - 12 // box room minus the label column
	if w < 10 {
		w = 10
	}
	for i := range s.inputs {
		s.inputs[i].Width = w
	}
}

// ---------- confirm ----------

var newPluginConfirmHelp = []key.Binding{
	core.Hint("target", core.Keys.Left, core.Keys.Right),
	core.Hint("add", core.Keys.Select),
	core.Hint("back", core.Keys.Back),
}

func newNewPluginConfirm(name, url, path string, addTarget int) *components.ConfirmScreen {
	target := addTarget // local copy the toggle mutates
	return &components.ConfirmScreen{
		Crumb:  core.RenderTitleBar("New Plugin"),
		Render: func(sh *core.Shared) string { return sh.Box(newPluginConfirmBody(sh, name, url, path, target)) },
		OnKey: func(sh *core.Shared, k string) tea.Cmd {
			if core.MatchKey(k, core.Keys.Left) || core.MatchKey(k, core.Keys.Right) {
				target = otherTarget(target)
			}
			return nil
		},
		OnYes: func(sh *core.Shared) tea.Cmd { return commitNewPlugin(sh, name, url, path, target) },
		Help:  newPluginConfirmHelp,
	}
}

func newPluginConfirmBody(sh *core.Shared, name, url, path string, addTarget int) string {
	urlBlock := core.IndentLines(core.HardWrap(url, sh.ConfirmWidth()-4), "    ")
	if path == "" {
		path = "(derived on install)"
	}
	return fmt.Sprintf(
		"Add plugin\n\n  name:     %s\n  url:\n%s\n  path:     %s\n\n  add to:   %s",
		name, urlBlock, path, targetToggle(addTarget))
}

// commitNewPlugin writes the pending entry to the project manifest or the global
// list, then unwinds to the root (rebuilding the Browse list for a project add).
func commitNewPlugin(sh *core.Shared, name, url, path string, addTarget int) tea.Cmd {
	if addTarget == targetGlobal {
		globalPath, err := addon.GlobalListPath()
		if err == nil {
			err = addon.AddEntry(globalPath, name, url, path)
		}
		if err != nil {
			sh.StatusMsg = "error: " + err.Error()
			return core.ResetToRoot()
		}
		// Show the Global tab rebuilt with the new entry (parallel to a project add
		// switching to Browse).
		return core.GlobalRefresh(fmt.Sprintf("added %s to global list", name))
	}

	if err := addon.AddEntry(sh.ManifestPath, name, url, path); err != nil {
		sh.StatusMsg = "error: " + err.Error()
		return core.ResetToRoot()
	}
	return tea.Batch(core.ResetToRoot(), core.RootRefresh("added "+name))
}

// targetToggle renders the Project ◄ ► Global switch with the active side
// highlighted.
func targetToggle(addTarget int) string {
	active := lipgloss.NewStyle().Foreground(core.FocusedColor).Bold(true)
	dim := lipgloss.NewStyle().Foreground(core.MutedColor)
	project, global := dim.Render("Project"), dim.Render("Global")
	if addTarget == targetProject {
		project = active.Render("Project")
	} else {
		global = active.Render("Global")
	}
	return fmt.Sprintf("%s  ◄ ►  %s", project, global)
}

func otherTarget(t int) int {
	if t == targetProject {
		return targetGlobal
	}
	return targetProject
}
