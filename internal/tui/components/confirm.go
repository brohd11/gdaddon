package components

import (
	"gdaddon/internal/tui/core"

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
	OnYes  func(*core.Shared) tea.Cmd
	OnKey  func(*core.Shared, string) tea.Cmd // handles keys other than the reserved confirm/cancel keys
	Help   []key.Binding
}

func (s *ConfirmScreen) Init(*core.Shared) tea.Cmd { return nil }

func (s *ConfirmScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}
	switch key.String() {
	case "y", "Y", "enter":
		return s, s.OnYes(sh)
	case "n", "N", "esc":
		return s, core.Pop()
	}
	if s.OnKey != nil {
		return s, s.OnKey(sh, key.String())
	}
	return s, nil
}

func (s *ConfirmScreen) View(sh *core.Shared) string {
	return lipgloss.JoinVertical(lipgloss.Left, s.Crumb, s.Render(sh))
}

func (s *ConfirmScreen) HelpView(sh *core.Shared) string { return sh.BindingHelp(s.Help) }
func (s *ConfirmScreen) SetSize(*core.Shared, int, int)  {}
