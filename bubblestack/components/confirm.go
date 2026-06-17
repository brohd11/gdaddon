package components

import (
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ConfirmScreen is the shared y/n confirm/summary box, reused by the install
// confirm, the archive confirm, and the New Plugin confirm. It is context-agnostic:
// it snapshots a breadcrumb and renders its body via a closure, and the OnYes/OnKey
// closures (supplied by the calling tab) decide what happens — it names no domain
// type. OnYes runs on confirm (y/enter); any non-reserved key is handed to OnKey
// (when set), so a caller can add custom interactions like a Project/Global toggle.
type ConfirmScreen struct {
	Crumb  string
	Render func(*core.Shared) string
	OnYes  func(*core.Shared) (tea.Msg, tea.Cmd)
	OnKey  func(*core.Shared, string) (tea.Msg, tea.Cmd) // handles keys other than the reserved confirm/cancel keys
	Help   []key.Binding
}

func (s *ConfirmScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *ConfirmScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Msg, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil, nil
	}
	k := key.String()
	switch {
	case core.MatchKey(k, core.Keys.Yes):
		m, c := s.OnYes(sh)
		return s, m, c
	case core.MatchKey(k, core.Keys.No):
		return s, core.Pop(), nil
	}
	if s.OnKey != nil {
		m, c := s.OnKey(sh, k)
		return s, m, c
	}
	return s, nil, nil
}

func (s *ConfirmScreen) View(sh *core.Shared) string {
	return lipgloss.JoinVertical(lipgloss.Left, s.Crumb, s.Render(sh))
}

func (s *ConfirmScreen) HelpView(sh *core.Shared) string { return sh.BindingHelp(s.Help) }
func (s *ConfirmScreen) SetSize(*core.Shared, int, int)  {}
