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
	Crumb      string // raw breadcrumb text; rendered via core.WithCrumb, omitted entirely when ""
	CrumbShort string // optional short breadcrumb-bar segment; defaults to Crumb
	Render     func(*core.Shared) string
	OnYes      func(*core.Shared) core.Action
	OnKey      func(*core.Shared, string) core.Action // handles keys other than the reserved confirm/cancel keys
	Help       []key.Binding
}

type ConfirmSimple struct {
	Crumb      string
	CrumbShort string
	Text       string
	OnYes      core.Action
	// optional
	OnKey func(*core.Shared, string) core.Action
	Help  []key.Binding
}

var _ core.Crumber = (*ConfirmScreen)(nil)

func (s *ConfirmScreen) Init(*core.Shared) tea.Cmd { return nil }

// CrumbLabel contributes the confirm screen's breadcrumb text as its segment (the
// short form when set, else the full crumb).
func (s *ConfirmScreen) CrumbLabel(short bool) string {
	if short && s.CrumbShort != "" {
		return s.CrumbShort
	}
	return s.Crumb
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
	return core.WithCrumb(s.Crumb, s.Render(sh))
}

func (s *ConfirmScreen) HelpView(sh *core.Shared) string { return sh.BindingHelp(s.Help) }
func (s *ConfirmScreen) SetSize(*core.Shared, int, int)  {}

var DefaultHelpKeys = []key.Binding{
	core.Hint("confirm", core.Keys.Yes),
	core.Hint("cancel", core.Keys.No),
}

func CreateConfirmScreen(sh *core.Shared, cs ConfirmSimple) *ConfirmScreen {
	if cs.Help == nil {
		cs.Help = DefaultHelpKeys
	}
	if cs.Crumb == "" {
		cs.Crumb = "Confirm"
	}
	if cs.CrumbShort == "" {
		cs.CrumbShort = "Conf"
	}
	return &ConfirmScreen{
		Crumb:      cs.Crumb,
		CrumbShort: cs.CrumbShort,
		Render:     func(*core.Shared) string { return sh.Box(cs.Text) },
		OnYes:      func(sh *core.Shared) core.Action { return cs.OnYes },
		OnKey:      cs.OnKey,
		Help:       cs.Help,
	}
}
