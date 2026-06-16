package components

import (
	"gdaddon/internal/tui/core"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// pickerScreen is the reusable list picker: a styled list that pops on esc/q,
// runs onSelect on enter, and optionally handles extra keys (e.g. archive on
// 'a'). It backs the asset/branch submenu and is the building block for any new
// flow that needs "pick one of these, then do X".
//
// Configure it with newPicker. The closures return the navigation command to run
// (push/pop/…); the picker stays on screen itself, so they never need a reference
// back to it.
type PickerScreen struct {
	list     list.Model
	OnSelect func(*core.Shared, list.Item) tea.Cmd
	OnKey    func(*core.Shared, string, list.Item) (tea.Cmd, bool)
}

// pickerOpts configures a pickerScreen. onKey is optional; when it reports
// handled=true the key is consumed (and its command, if any, run), otherwise the
// key falls through to the list.
type PickerOpts struct {
	Title    string
	Help     []key.Binding // extra help/hint bindings shown in the list help
	OnSelect func(*core.Shared, list.Item) tea.Cmd
	OnKey    func(*core.Shared, string, list.Item) (tea.Cmd, bool)
}

var _ core.Filterer = (*PickerScreen)(nil)

func NewPicker(items []list.Item, opts PickerOpts) *PickerScreen {
	return &PickerScreen{
		list:     core.NewSelectList(items, opts.Title, opts.Help...),
		OnSelect: opts.OnSelect,
		OnKey:    opts.OnKey,
	}
}

func (s *PickerScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *PickerScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *PickerScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Cmd) {
	if s.Filtering() {
		var cmd tea.Cmd
		s.list, cmd = s.list.Update(msg)
		return s, cmd
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch k := key.String(); k {
		case "esc", "q":
			return s, core.Pop()
		case "enter":
			if s.OnSelect != nil {
				return s, s.OnSelect(sh, s.list.SelectedItem())
			}
			return s, nil
		default:
			if s.OnKey != nil {
				if cmd, handled := s.OnKey(sh, k, s.list.SelectedItem()); handled {
					return s, cmd
				}
			}
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *PickerScreen) View(*core.Shared) string     { return s.list.View() }
func (s *PickerScreen) HelpView(*core.Shared) string { return core.HelpView(s.list) }

func (s *PickerScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}
