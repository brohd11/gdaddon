package tui

import (
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
type pickerScreen struct {
	list     list.Model
	onSelect func(*shared, list.Item) tea.Cmd
	onKey    func(*shared, string, list.Item) (tea.Cmd, bool)
}

// pickerOpts configures a pickerScreen. onKey is optional; when it reports
// handled=true the key is consumed (and its command, if any, run), otherwise the
// key falls through to the list.
type pickerOpts struct {
	title    string
	help     []key.Binding // extra help/hint bindings shown in the list help
	onSelect func(*shared, list.Item) tea.Cmd
	onKey    func(*shared, string, list.Item) (tea.Cmd, bool)
}

var _ filterer = (*pickerScreen)(nil)

func newPicker(items []list.Item, opts pickerOpts) *pickerScreen {
	return &pickerScreen{
		list:     newSelectList(items, opts.title, opts.help...),
		onSelect: opts.onSelect,
		onKey:    opts.onKey,
	}
}

func (s *pickerScreen) Init(*shared) tea.Cmd { return nil }

func (s *pickerScreen) filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *pickerScreen) Update(sh *shared, msg tea.Msg) (screen, tea.Cmd) {
	if s.filtering() {
		var cmd tea.Cmd
		s.list, cmd = s.list.Update(msg)
		return s, cmd
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch k := key.String(); k {
		case "esc", "q":
			return s, pop()
		case "enter":
			if s.onSelect != nil {
				return s, s.onSelect(sh, s.list.SelectedItem())
			}
			return s, nil
		default:
			if s.onKey != nil {
				if cmd, handled := s.onKey(sh, k, s.list.SelectedItem()); handled {
					return s, cmd
				}
			}
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, cmd
}

func (s *pickerScreen) View(*shared) string     { return s.list.View() }
func (s *pickerScreen) HelpView(*shared) string { return helpView(s.list) }

func (s *pickerScreen) SetSize(sh *shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}
