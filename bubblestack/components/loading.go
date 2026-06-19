package components

import (
	"fmt"
	"github.com/brohd11/bubblestack/core"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// loadingScreen is the non-interactive spinner shown while an upstream fetch is in
// flight. It is context-agnostic: the caller supplies the title, the fetch command,
// and an onResult closure that turns the fetch result (releasesMsg / branchesMsg /
// …) into the next navigation command. loadingScreen itself names no domain type.
type LoadingScreen struct {
	Title      string
	CrumbShort string // optional short breadcrumb segment; defaults to Title
	label      string
	cmd        tea.Cmd                                 // the fetch command, run on Init
	onResult   func(*core.Shared, tea.Msg) core.Action // caller's result handler; the zero Action ⇒ ignore msg
}

var _ core.Crumber = (*LoadingScreen)(nil)

func NewLoadingScreen(Title, label string, cmd tea.Cmd, onResult func(*core.Shared, tea.Msg) core.Action) *LoadingScreen {
	return &LoadingScreen{Title: Title, label: label, cmd: cmd, onResult: onResult}
}

// CrumbLabel contributes the loading screen's title as its breadcrumb segment.
func (s *LoadingScreen) CrumbLabel(short bool) string {
	if short && s.CrumbShort != "" {
		return s.CrumbShort
	}
	return s.Title
}

func (s *LoadingScreen) Init(sh *core.Shared) tea.Cmd {
	return tea.Batch(sh.Spinner.Tick, s.cmd)
}

func (s *LoadingScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	return s, s.onResult(sh, msg)
}

func (s *LoadingScreen) View(sh *core.Shared) string {
	return lipgloss.JoinVertical(lipgloss.Left,
		core.RenderTitleBar(s.Title),
		fmt.Sprintf("  %s %s", sh.Spinner.View(), s.label))
}

func (s *LoadingScreen) HelpView(sh *core.Shared) string {
	return sh.NoteHelp("non-interactive · working…")
}

func (s *LoadingScreen) SetSize(*core.Shared, int, int) {}
