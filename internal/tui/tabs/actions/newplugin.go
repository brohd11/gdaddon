package actions

import (
	"gdaddon/internal/tui/core"
	"strings"

	"gdaddon/internal/addon"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// newPluginForm is the single-page Add Plugin form: url/name/path text fields and
// the Project/Global target toggle. On enter it pushes the confirm screen.
type NewPluginForm struct {
	inputs    []textinput.Model
	formFocus int
	addTarget int
}

var _ core.Filterer = (*NewPluginForm)(nil)

func NewNewPluginForm() *NewPluginForm {
	mk := func(placeholder string) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.Prompt = "" // labels are rendered separately in the form view
		return ti
	}
	// Order matches the fld* indices: url, name, path.
	return &NewPluginForm{
		inputs: []textinput.Model{
			mk("https://github.com/owner/repo"),
			mk("(optional — derived from url)"),
			mk("(optional — derived on install)"),
		},
		addTarget: targetProject,
		formFocus: fldURL,
	}
}

func (s *NewPluginForm) Init(*core.Shared) tea.Cmd { return s.syncFormFocus() }

// filtering: the URL text input captures keys, so the global tab/c shortcuts must
// not steal characters typed into it.
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
