package components

import (
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// ConfirmScreen is the shared y/n confirm/summary box, reused by the install
// confirm, the archive confirm, and the New Plugin confirm. It is context-agnostic:
// it snapshots a breadcrumb and renders its body via a closure, and the OnYes/OnKey
// closures (supplied by the calling tab) decide what happens — it names no domain
// type. OnYes runs on confirm (y/enter); any non-reserved key is handed to OnKey
// (when set), so a caller can add custom interactions like a Project/Global toggle.
type ConfirmScreen struct {
	Title      string // optional in-body title bar (core.WithTitle); omitted ⇒ no bar
	Crumb      string // breadcrumb segment (CrumbLabel); omitted ⇒ contributes none
	CrumbShort string // optional short breadcrumb-bar segment; defaults to Crumb
	Render     func(*core.Shared) string
	OnYes      func(*core.Shared) core.Action
	OnKey      func(*core.Shared, string) core.Action // handles keys other than the reserved confirm/cancel keys
	Help       []key.Binding
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

var _ core.Crumber = (*ConfirmScreen)(nil)

func (s *ConfirmScreen) Init(*core.Shared) tea.Cmd { return nil }

// CrumbLabel contributes the confirm screen's breadcrumb segment, defaulting to
// "Confirm" when no Crumb is declared.
func (s *ConfirmScreen) CrumbLabel(short bool) string {
	return crumbSeg(short, s.CrumbShort, s.Crumb, "Conf")
}

func (s *ConfirmScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, core.Action{}
	}
	k := key.String()
	switch {
	case core.MatchKey(k, core.Keys.Yes):
		return s, s.OnYes(sh)
	case core.MatchKey(k, core.Keys.No):
		return s, core.Pop()
	}
	if s.OnKey != nil {
		return s, s.OnKey(sh, k)
	}
	return s, core.Action{}
}

func (s *ConfirmScreen) View(sh *core.Shared) string {
	return core.WithTitle(s.Title, s.Render(sh))
}

func (s *ConfirmScreen) HelpView(sh *core.Shared) string { return sh.BindingHelp(s.Help) }
func (s *ConfirmScreen) SetSize(*core.Shared, int, int)  {}

var DefaultHelpKeys = []key.Binding{
	core.Hint("confirm", core.Keys.Yes),
	core.Hint("cancel", core.Keys.No),
}

func CreateConfirmScreen(cs ConfirmSimple) *ConfirmScreen {
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

	return &ConfirmScreen{
		Title:      cs.Title,
		Crumb:      cs.Crumb,
		CrumbShort: cs.CrumbShort,
		Render:     render,
		OnYes:      onYes,
		OnKey:      cs.OnKey,
		Help:       cs.Help,
	}
}
