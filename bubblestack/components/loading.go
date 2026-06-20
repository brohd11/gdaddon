package components

import (
	"context"
	"fmt"

	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// loadingScreen is the spinner shown while an upstream fetch is in flight. It is
// context-agnostic: the caller supplies the title, a Run closure that builds the
// fetch command from a cancellable context, and an onResult closure that turns the
// fetch result (releasesMsg / branchesMsg / …) into the next navigation command.
// loadingScreen itself names no domain type.
//
// esc cancels the fetch: Init owns a context.WithCancel handed to Run, and a
// keypress calls cancel and pops back — so a slow/unreachable host can be abandoned
// without waiting it out (a cancellable Run threads ctx into its network call).
type LoadingScreen struct {
	Title      string
	Crumb      string // optional breadcrumb segment; defaults to Title
	CrumbShort string // optional short breadcrumb segment; defaults to Crumb/Title
	Label      string
	Run        func(ctx context.Context) tea.Cmd       // builds the fetch command, run on Init
	OnResult   func(*core.Shared, tea.Msg) core.Action // caller's result handler; the zero Action ⇒ ignore msg
	cancel     context.CancelFunc
}

var _ core.Crumber = (*LoadingScreen)(nil)

func NewLoadingScreen(Title, Label string, Run func(context.Context) tea.Cmd, OnResult func(*core.Shared, tea.Msg) core.Action) *LoadingScreen {
	return &LoadingScreen{Crumb: "Loading", Title: Title, Label: Label, Run: Run, OnResult: OnResult}
}

// CrumbLabel contributes the loading screen's breadcrumb segment: the short form when
// set, else the explicit crumb, else the title.
func (s *LoadingScreen) CrumbLabel(short bool) string {
	return crumbSeg(short, s.CrumbShort, s.Crumb, s.Title)
}

func (s *LoadingScreen) Init(sh *core.Shared) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	return tea.Batch(sh.Spinner.Tick, s.Run(ctx))
}

func (s *LoadingScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	if k, ok := msg.(tea.KeyMsg); ok {
		// esc cancels the in-flight fetch (its ctx unwinds the request) and pops back.
		// The cancelled fetch still returns a result to the screen we pop to, which
		// doesn't recognize it and ignores it.
		if core.MatchKey(k.String(), core.Keys.Back) {
			if s.cancel != nil {
				s.cancel()
			}
			return s, core.Seq(core.SetStatus("cancelled"), core.Pop())
			// return s, core.Pop() // testing
		}
		return s, core.Action{}
	}
	return s, s.OnResult(sh, msg)
}

func (s *LoadingScreen) View(sh *core.Shared) string {
	return lipgloss.JoinVertical(lipgloss.Left,
		core.RenderTitleBar(s.Title),
		fmt.Sprintf("  %s %s", sh.Spinner.View(), s.Label))
}

func (s *LoadingScreen) HelpView(sh *core.Shared) string {
	return sh.BindingHelp([]key.Binding{core.Hint("cancel", core.Keys.Back)})
}

func (s *LoadingScreen) SetSize(*core.Shared, int, int) {}
