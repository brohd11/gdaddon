package components

import (
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// DialogScreen is the shared y/n confirm/summary box. It serves two shapes behind the
// Overlay flag, which is the only behavioral switch:
//
//   - Overlay false (a confirm): full-screen, rendered in the body via core.WithTitle,
//     with its hints in the chrome help bar. Reused by the install/archive/new-plugin
//     confirms.
//   - Overlay true (a popup): the router draws it as a centered modal over the screen
//     below it (core.Overlayer), rendered via core.PopupBox with its hints inside the
//     box; the background screen's help bar stays.
//
// Either way it is context-agnostic: it snapshots a breadcrumb and renders its body
// via a closure, and the OnYes/OnKey closures (supplied by the caller) decide what
// happens — it names no domain type. OnYes runs on confirm (y/enter); No (esc/n) pops
// it; any other key is handed to OnKey when set.
type DialogScreen struct {
	Title      string // in-body title bar (confirm) / accent line (overlay); omitted ⇒ none
	Crumb      string // breadcrumb segment (CrumbLabel); omitted ⇒ contributes none
	CrumbShort string // optional short breadcrumb-bar segment; defaults to Crumb/Title
	Render     func(*core.Shared) string
	OnYes      func(*core.Shared) core.Action
	OnKey      func(*core.Shared, string) core.Action // handles keys other than the reserved confirm/cancel keys
	Help       []key.Binding
	Overlay    bool // draw as a centered modal over the screen below (core.Overlayer)
	Width      int  // overlay inner content width; 0 ⇒ size to content (overlay only)
}

type ConfirmSimple struct {
	Text  string
	OnYes core.Action
	// optional
	Title      string // optional in-body title bar; omitted ⇒ no bar
	Crumb      string
	CrumbShort string
	Render     func(*core.Shared) string // this overides text if not nil
	OnYesLamda func(*core.Shared) core.Action
	OnKey      func(*core.Shared, string) core.Action
	Help       []key.Binding
}

var _ core.Crumber = (*DialogScreen)(nil)
var _ core.Overlayer = (*DialogScreen)(nil)

func (s *DialogScreen) Init(*core.Shared) tea.Cmd { return nil }

// IsOverlay reports whether the router should draw this dialog as a centered modal
// over the screen below it (Overlay) rather than full-screen.
func (s *DialogScreen) IsOverlay() bool { return s.Overlay }

// CrumbLabel contributes the dialog's breadcrumb segment. A confirm uses its Crumb
// (default "Conf"); a popup uses its Title (no fallback), since a popup sets Title
// rather than Crumb.
func (s *DialogScreen) CrumbLabel(short bool) string {
	if s.Overlay {
		return crumbSeg(short, s.CrumbShort, s.Title, "")
	}
	return crumbSeg(short, s.CrumbShort, s.Crumb, "Conf")
}

func (s *DialogScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
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

func (s *DialogScreen) View(sh *core.Shared) string {
	if s.Overlay {
		// A popup renders its hints inside the box (the router keeps the background
		// screen's help bar), and composites as a centered modal.
		body := s.Render(sh)
		if hint := sh.BindingHelp(s.Help); hint != "" {
			body = body + "\n\n" + hint
		}
		return core.PopupBox(s.Title, body, s.Width)
	}
	return core.WithTitle(s.Title, s.Render(sh))
}

// HelpView is the chrome help bar for a confirm; empty for a popup (which renders its
// hints inside the box).
func (s *DialogScreen) HelpView(sh *core.Shared) string {
	if s.Overlay {
		return ""
	}
	return sh.BindingHelp(s.Help)
}

func (s *DialogScreen) SetSize(*core.Shared, int, int) {}

var DefaultHelpKeys = []key.Binding{
	core.Hint("confirm", core.Keys.Yes),
	core.Hint("cancel", core.Keys.No),
}

// DefaultPopupHelp is the standard single-key "done" hint for an acknowledgement popup.
var DefaultPopupHelp = []key.Binding{core.Hint("done", core.Keys.Yes)}

// CreateConfirmScreen builds a full-screen confirm (Overlay false) from the simplified
// ConfirmSimple config.
func CreateConfirmScreen(cs ConfirmSimple) *DialogScreen {
	if cs.Help == nil {
		cs.Help = DefaultHelpKeys
	}
	render := func(sh *core.Shared) string { return sh.Box(cs.Text) }
	if cs.Render != nil {
		render = cs.Render
	}
	onYes := func(sh *core.Shared) core.Action { return cs.OnYes }
	if cs.OnYesLamda != nil {
		onYes = cs.OnYesLamda
	}

	return &DialogScreen{
		Title:      cs.Title,
		Crumb:      cs.Crumb,
		CrumbShort: cs.CrumbShort,
		Render:     render,
		OnYes:      onYes,
		OnKey:      cs.OnKey,
		Help:       cs.Help,
	}
}

// CreatePopup builds a text-only acknowledgement popup (title + body, Overlay true):
// OnYes is the action taken when dismissed with y/enter (defaults to a plain Pop when
// nil), and Help defaults to the "done" hint.
func CreatePopup(title, body string, onYes core.Action, help ...key.Binding) *DialogScreen {
	if help == nil {
		help = DefaultPopupHelp
	}
	return &DialogScreen{
		Title:   title,
		Render:  func(*core.Shared) string { return body },
		OnYes:   func(*core.Shared) core.Action { return onYes },
		Help:    help,
		Overlay: true,
	}
}
