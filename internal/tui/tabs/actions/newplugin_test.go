package actions

import (
	"strings"
	"testing"

	"gdaddon/internal/tui/components"
	"gdaddon/internal/tui/core"

	tea "github.com/charmbracelet/bubbletea"
)

func sized(tm tea.Model) tea.Model {
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return tm
}

func pump(tm tea.Model, msg tea.Msg) tea.Model {
	tm, cmd := tm.Update(msg)
	for i := 0; i < 8 && cmd != nil; i++ {
		out := cmd()
		if out == nil {
			break
		}
		if _, isBatch := out.(tea.BatchMsg); isBatch {
			break
		}
		tm, cmd = tm.Update(out)
	}
	return tm
}

func newTestRouter() core.Router {
	sh := core.NewShared("/tmp/gdaddon-test/addon_manifest.yml", "/tmp/gdaddon-test")
	return core.NewRouter(sh, []core.TabEntry{{Title: "Actions", Root: NewActionsScreen()}})
}

// TestNewPluginFormToConfirm checks the form validates the URL (empty stays put)
// and a filled URL pushes the confirm screen.
func TestNewPluginFormToConfirm(t *testing.T) {
	tm := sized(newTestRouter())
	tm, _ = tm.Update(core.Push(NewNewPluginForm())())
	form, ok := tm.(core.Router).Top().(*NewPluginForm)
	if !ok {
		t.Fatalf("want *NewPluginForm, got %T", tm.(core.Router).Top())
	}

	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tm.(core.Router).Top().(*NewPluginForm); !ok {
		t.Fatalf("empty URL should keep the form, got %T", tm.(core.Router).Top())
	}

	form.inputs[fldURL].SetValue("https://github.com/owner/repo")
	tm = pump(tm, tea.KeyMsg{Type: tea.KeyEnter})
	if _, ok := tm.(core.Router).Top().(*components.ConfirmScreen); !ok {
		t.Fatalf("filled URL should push confirm, got %T", tm.(core.Router).Top())
	}
	if !strings.Contains(tm.View(), "owner/repo") {
		t.Fatal("confirm view should show the entered url")
	}
}
