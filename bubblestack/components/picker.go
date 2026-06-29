package components

import (
	"github.com/brohd11/bubblestack/core"

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
	list       list.Model
	crumb      string // breadcrumb segment; defaults to the list title when ""
	crumbShort string
	OnSelect   func(*core.Shared, list.Item) core.Action
	OnKey      func(*core.Shared, string, list.Item) (core.Action, bool)
	refresh    func(*core.Shared, any) ([]list.Item, bool)
	popStop    bool
}

// pickerOpts configures a pickerScreen. onKey is optional; when it reports
// handled=true the key is consumed (and its command, if any, run), otherwise the
// key falls through to the list.
type PickerOpts struct {
	Title        string
	Crumb        string        // optional breadcrumb segment; defaults to Title
	CrumbShort   string        // optional short breadcrumb segment; defaults to Crumb/Title
	Help         []key.Binding // extra help/hint bindings shown in the list help
	OnSelect     func(*core.Shared, list.Item) core.Action
	OnKey        func(*core.Shared, string, list.Item) (core.Action, bool)
	// Refresh, when set, makes the picker a Receiver: on a PropagateAll broadcast it
	// is called with the payload; returning ok=true rebuilds the rows from items.
	Refresh      func(sh *core.Shared, payload any) (items []list.Item, ok bool)
	PopStop      bool // mark this picker as a PopTo boundary (a command hub)
	InitialIndex int  // cursor starts here; 0 = first item (default)
}

var _ core.Filterer = (*PickerScreen)(nil)
var _ core.PopStopper = (*PickerScreen)(nil)
var _ core.Crumber = (*PickerScreen)(nil)
var _ core.Receiver = (*PickerScreen)(nil)

func NewPicker(items []list.Item, opts PickerOpts) *PickerScreen {
	s := &PickerScreen{
		list:       core.NewSelectList(items, opts.Title, opts.Help...),
		crumb:      opts.Crumb,
		crumbShort: opts.CrumbShort,
		OnSelect:   opts.OnSelect,
		OnKey:      opts.OnKey,
		refresh:    opts.Refresh,
		popStop:    opts.PopStop,
	}
	if opts.InitialIndex > 0 {
		s.list.Select(opts.InitialIndex)
	}
	return s
}

func (s *PickerScreen) PopStop() bool { return s.popStop }

// Receive lets a picker rebuild its rows on a PropagateAll broadcast when a Refresh
// closure is configured; without one it's a no-op (the common case).
func (s *PickerScreen) Receive(sh *core.Shared, payload any) core.Action {
	if s.refresh != nil {
		if items, ok := s.refresh(sh, payload); ok {
			s.list.SetItems(items)
		}
	}
	return core.Action{}
}

// CrumbLabel contributes the picker's breadcrumb segment: the short form when set,
// else the explicit crumb, else the list title (the default — crumb and title agree).
func (s *PickerScreen) CrumbLabel(short bool) string {
	return crumbSeg(short, s.crumbShort, s.crumb, s.list.Title)
}

func (s *PickerScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *PickerScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *PickerScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	if s.Filtering() {
		var cmd tea.Cmd
		s.list, cmd = s.list.Update(msg)
		return s, core.Async(cmd)
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		k := key.String()
		switch {
		case core.MatchKey(k, core.Keys.Back):
			return s, core.Pop()
		case core.MatchKey(k, core.Keys.Select):
			if s.OnSelect != nil {
				return s, s.OnSelect(sh, s.list.SelectedItem())
			}
			// No screen-level handler: let a self-dispatching Item pick itself.
			if it, ok := s.list.SelectedItem().(Item); ok && it.Pick != nil {
				return s, it.Pick(sh)
			}
			return s, core.Action{}
		default:
			if s.OnKey != nil {
				if act, handled := s.OnKey(sh, k, s.list.SelectedItem()); handled {
					return s, act
				}
			} else if it, ok := s.list.SelectedItem().(Item); ok && it.Keys != nil {
				if act, handled := it.Keys(sh, k); handled {
					return s, act
				}
			}
		}
	}
	var cmd tea.Cmd
	s.list, cmd = s.list.Update(msg)
	return s, core.Async(cmd)
}

func (s *PickerScreen) View(*core.Shared) string     { return s.list.View() }
func (s *PickerScreen) HelpView(*core.Shared) string { return core.ShortHelp(s.list, core.HelpMinimal) }

func (s *PickerScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}
