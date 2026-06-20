package actions

import (
	"gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// actionsScreen is the Actions tab (reached with [ / ]): create manifest (when none
// is loaded), install all, new plugin, theme. As a tab root it quits on q rather than
// popping.
type ActionsScreen struct {
	list list.Model
}

var _ core.Filterer = (*ActionsScreen)(nil)
var _ core.Receiver = (*ActionsScreen)(nil)
var _ core.Crumber = (*ActionsScreen)(nil)

// CrumbLabel anchors the breadcrumb at the Actions root.
func (s *ActionsScreen) CrumbLabel(bool) string { return "Tab" } // s.list.Title }

func NewActionsScreen(sh *core.Shared) *ActionsScreen {
	return &ActionsScreen{list: core.NewSelectList(actionItems(sh), "Actions")}
}

func (s *ActionsScreen) Init(*core.Shared) tea.Cmd { return nil }

// Receive rebuilds the menu on a PathRefresh so the Create-manifest row appears or
// disappears with the manifest's presence. It never grabs focus (PathRefresh's focus
// belongs to the Project tab).
func (s *ActionsScreen) Receive(sh *core.Shared, payload any) core.Action {
	if _, ok := payload.(appctx.PathRefresh); ok {
		s.list.SetItems(actionItems(sh))
	}
	return core.Action{}
}

func (s *ActionsScreen) Filtering() bool { return s.list.FilterState() == list.Filtering }

func (s *ActionsScreen) Update(sh *core.Shared, msg tea.Msg) (core.Screen, core.Action) {
	return s, components.RootUpdate(sh, &s.list, msg)
}

func (s *ActionsScreen) View(*core.Shared) string     { return s.list.View() }
func (s *ActionsScreen) HelpView(*core.Shared) string { return core.ShortHelp(s.list, core.HelpTabbed) }

func (s *ActionsScreen) SetSize(sh *core.Shared, width, bodyHeight int) {
	s.list.SetSize(width, bodyHeight)
}
