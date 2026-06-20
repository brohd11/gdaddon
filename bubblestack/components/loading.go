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
	Crumb      string // optional breadcrumb segment; defaults to Title
	CrumbShort string // optional short breadcrumb segment; defaults to Crumb/Title
	Label      string
	Cmd        tea.Cmd                                 // the fetch command, run on Init
	OnResult   func(*core.Shared, tea.Msg) core.Action // caller's result handler; the zero Action ⇒ ignore msg
}

var _ core.Crumber = (*LoadingScreen)(nil)

func NewLoadingScreen(Title, Label string, Cmd tea.Cmd, OnResult func(*core.Shared, tea.Msg) core.Action) *LoadingScreen {
	return &LoadingScreen{Crumb: "Loading", Title: Title, Label: Label, Cmd: Cmd, OnResult: OnResult}
}

// CrumbLabel contributes the loading screen's breadcrumb segment: the short form when
// set, else the explicit crumb, else the title.
func (s *LoadingScreen) CrumbLabel(short bool) string {
	return crumbSeg(short, s.CrumbShort, s.Crumb, s.Title)
}

func (s *LoadingScreen) Init(sh *core.Shared) tea.Cmd {
	return tea.Batch(sh.Spinner.Tick, s.Cmd)
}

func (s *LoadingScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	return s, s.OnResult(sh, msg)
}

func (s *LoadingScreen) View(sh *core.Shared) string {
	return lipgloss.JoinVertical(lipgloss.Left,
		core.RenderTitleBar(s.Title),
		fmt.Sprintf("  %s %s", sh.Spinner.View(), s.Label))
}

func (s *LoadingScreen) HelpView(sh *core.Shared) string {
	return sh.NoteHelp("non-interactive · working…")
}

func (s *LoadingScreen) SetSize(*core.Shared, int, int) {}
