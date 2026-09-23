package newplugin

import (
	"strings"
	"testing"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/bubblestack/tuitest"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	tea "charm.land/bubbletea/v2"
)

// stubRoot is a minimal tab root so the test can build a router to push the form
// onto (the flow itself is tab-agnostic).
type stubRoot struct{}

func (stubRoot) Init(*core.Shared) tea.Cmd { return nil }
func (stubRoot) Update(*core.Shared, tea.Msg) (core.Screen, core.Action) {
	return stubRoot{}, core.Action{}
}
func (stubRoot) View(*core.Shared) string       { return "" }
func (stubRoot) HelpView(*core.Shared) string   { return "" }
func (stubRoot) SetSize(*core.Shared, int, int) {}

func sized(tm tea.Model) tea.Model {
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return tm
}

var pump = tuitest.Pump

func newTestRouter() core.Router {
	sh := core.NewShared(appctx.New("/tmp/gdaddon-test", "dev"))
	return core.NewRouter(sh, []core.TabEntry{{Title: "Test", New: func(*core.Shared) core.Screen { return stubRoot{} }}})
}

// TestNewPluginFormToConfirm checks the form validates the URL (empty stays put)
// and a filled URL pushes the confirm screen.
func TestNewPluginFormToConfirm(t *testing.T) {
	tm := sized(newTestRouter())
	tm, _ = tm.Update(core.Push(NewNewPluginForm()))
	form, ok := tm.(core.Router).Top().(*components.FormScreen)
	if !ok {
		t.Fatalf("want *components.FormScreen, got %T", tm.(core.Router).Top())
	}

	tm = pump(tm, keyMsg("enter"))
	if _, ok := tm.(core.Router).Top().(*components.FormScreen); !ok {
		t.Fatalf("empty URL should keep the form, got %T", tm.(core.Router).Top())
	}

	form.SetValue("url", "https://github.com/owner/repo")
	tm = pump(tm, keyMsg("enter"))
	if _, ok := tm.(core.Router).Top().(*components.DialogScreen); !ok {
		t.Fatalf("filled URL should push confirm, got %T", tm.(core.Router).Top())
	}
	if !strings.Contains(view(tm), "owner/repo") {
		t.Fatal("confirm view should show the entered url")
	}
}

// TestNewWithURL prefills the URL and focuses the Name field.
func TestNewWithURL(t *testing.T) {
	f := NewWithURL("https://github.com/owner/repo")
	if got := f.Value("url"); got != "https://github.com/owner/repo" {
		t.Fatalf("url not prefilled, got %q", got)
	}
	if f.FocusedKey() != "name" {
		t.Fatalf("focus should jump to Name field, got %q", f.FocusedKey())
	}
}

var view = tuitest.View
