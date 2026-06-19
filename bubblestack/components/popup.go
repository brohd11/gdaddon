package components

import (
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// PopupScreen is a modal popup: a small bordered box the router draws centered on
// top of the screen below it (the screen stays visible around the box) rather than
// replacing it. It implements core.Overlayer, which is the only difference from
// ConfirmScreen — otherwise it is the same closure-configured, context-agnostic
// dialog: it snapshots a title, renders its body via a closure, and the
// OnYes/OnKey closures (supplied by the caller) decide what happens. OnYes runs on
// confirm (y/enter); No (esc/n) pops it; any other key is handed to OnKey when set.
//
// Because the router keeps showing the background screen's help bar, a popup renders
// its own key hints *inside* the box (from Help) instead of in the chrome help bar.
type PopupScreen struct {
	Title      string                                 // accent title line, omitted when ""
	CrumbShort string                                 // optional short breadcrumb segment; defaults to Title
	Render     func(*core.Shared) string              // body content
	OnYes      func(*core.Shared) core.Action         // y/enter
	OnKey      func(*core.Shared, string) core.Action // handles keys other than the reserved confirm/cancel keys
	Help       []key.Binding                          // hints rendered inside the box (not the chrome help bar)
	Width      int                                    // inner content width; 0 ⇒ size to content
}

var _ core.Crumber = (*PopupScreen)(nil)

func (s *PopupScreen) IsOverlay() bool { return true }

// CrumbLabel contributes the popup's title as its breadcrumb segment (drawn on the
// background screen's chrome, which stays visible around the modal).
func (s *PopupScreen) CrumbLabel(short bool) string {
	if short && s.CrumbShort != "" {
		return s.CrumbShort
	}
	return s.Title
}

func (s *PopupScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *PopupScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, core.Action{}
	}
	k := key.String()
	switch {
	case core.MatchKey(k, core.Keys.Yes):
		if s.OnYes != nil {
			return s, s.OnYes(sh)
		}
		return s, core.Pop()
	case core.MatchKey(k, core.Keys.No):
		return s, core.Pop()
	}
	if s.OnKey != nil {
		return s, s.OnKey(sh, k)
	}
	return s, core.Action{}
}

func (s *PopupScreen) View(sh *core.Shared) string {
	body := s.Render(sh)
	if hint := sh.BindingHelp(s.Help); hint != "" {
		body = body + "\n\n" + hint
	}
	return core.PopupBox(s.Title, body, s.Width)
}

// HelpView is empty: a popup shows its hints inside the box, and the router keeps the
// background screen's help bar.
func (s *PopupScreen) HelpView(*core.Shared) string   { return "" }
func (s *PopupScreen) SetSize(*core.Shared, int, int) {}

// DefaultPopupHelp is the standard single-key "done" hint for an acknowledgement popup.
var DefaultPopupHelp = []key.Binding{core.Hint("done", core.Keys.Yes)}

// CreatePopup builds a text-only acknowledgement popup (title + body), mirroring
// CreateConfirmScreen: OnYes is the action taken when dismissed with y/enter (defaults
// to a plain Pop when nil), and Help defaults to the "done" hint.
func CreatePopup(title, body string, onYes core.Action, help ...key.Binding) *PopupScreen {
	if help == nil {
		help = DefaultPopupHelp
	}
	return &PopupScreen{
		Title:  title,
		Render: func(*core.Shared) string { return body },
		OnYes:  func(*core.Shared) core.Action { return onYes },
		Help:   help,
	}
}
