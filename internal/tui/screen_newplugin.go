package tui

import (
	"strings"

	"gdaddon/internal/addon"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// newPluginForm is the single-page Add Plugin form: url/name/path text fields and
// the Project/Global target toggle. On enter it pushes the confirm screen.
type newPluginForm struct {
	inputs    []textinput.Model
	formFocus int
	addTarget int
}

var _ filterer = (*newPluginForm)(nil)

func newNewPluginForm() *newPluginForm {
	mk := func(placeholder string) textinput.Model {
		ti := textinput.New()
		ti.Placeholder = placeholder
		ti.Prompt = "" // labels are rendered separately in the form view
		return ti
	}
	// Order matches the fld* indices: url, name, path.
	return &newPluginForm{
		inputs: []textinput.Model{
			mk("https://github.com/owner/repo"),
			mk("(optional — derived from url)"),
			mk("(optional — derived on install)"),
		},
		addTarget: targetProject,
		formFocus: fldURL,
	}
}

func (s *newPluginForm) Init(*shared) tea.Cmd { return s.syncFormFocus() }

// filtering: the URL text input captures keys, so the global tab/c shortcuts must
// not steal characters typed into it.
func (s *newPluginForm) filtering() bool { return true }

func (s *newPluginForm) Update(sh *shared, msg tea.Msg) (screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch key.String() {
	case "esc":
		return s, pop()
	case "up", "shift+tab":
		s.formFocus = (s.formFocus - 1 + fldCount) % fldCount
		return s, s.syncFormFocus()
	case "down", "tab":
		s.formFocus = (s.formFocus + 1) % fldCount
		return s, s.syncFormFocus()
	case "left", "right", "h", "l":
		// On the target row these toggle Project↔Global; on text rows they fall
		// through to the input (cursor movement / literal characters).
		if s.formFocus == fldTarget {
			s.addTarget = otherTarget(s.addTarget)
			return s, nil
		}
	case "enter":
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
		return s, push(newNewPluginConfirm(name, addon.NormalizeRepoURL(url), path, s.addTarget))
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
func (s *newPluginForm) syncFormFocus() tea.Cmd {
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

func (s *newPluginForm) View(sh *shared) string {
	label := lipgloss.NewStyle().Foreground(mutedColor)
	marker := func(focused bool) string {
		if focused {
			return lipgloss.NewStyle().Foreground(focusedColor).Render("▸ ")
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
		renderTitleBar("New Plugin"),
		boxStyle.Width(sh.confirmWidth()).Render(body))
}

func (s *newPluginForm) HelpView(sh *shared) string {
	// return sh.bindingHelp(newPluginInputHelp)
	var newPluginInputHelp = []key.Binding{
		key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑/↓", "field")),
		key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "target")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "next")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
	return sh.bindingHelp(newPluginInputHelp)
}

func (s *newPluginForm) SetSize(sh *shared, width, bodyHeight int) {
	w := sh.confirmWidth() - 12 // box room minus the label column
	if w < 10 {
		w = 10
	}
	for i := range s.inputs {
		s.inputs[i].Width = w
	}
}
