package editmanifest_test

import (
	"path/filepath"
	"testing"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gdaddon/internal/tui/flows/editmanifest"
	globaltab "github.com/brohd11/gdaddon/internal/tui/tabs/global"
	projecttab "github.com/brohd11/gdaddon/internal/tui/tabs/project"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"

	tea "charm.land/bubbletea/v2"
)

// staleSubmenu stands in for the per-entry command hub beneath Edit Manifest. Those
// menus capture the Addon used to construct their row closures, so a successful edit
// must remove this screen as well as the form after refreshing the owning root.
type staleSubmenu struct{}

func (staleSubmenu) Init(*core.Shared) tea.Cmd { return nil }
func (staleSubmenu) Update(*core.Shared, tea.Msg) (core.Screen, core.Action) {
	return staleSubmenu{}, core.Action{}
}
func (staleSubmenu) View(*core.Shared) string       { return "" }
func (staleSubmenu) HelpView(*core.Shared) string   { return "" }
func (staleSubmenu) SetSize(*core.Shared, int, int) {}

func submitEdit(t *testing.T, r core.Router, form *components.FormScreen, newURL string) core.Router {
	t.Helper()
	var tm tea.Model = r
	tm, _ = tm.Update(core.Push(staleSubmenu{}))
	tm, _ = tm.Update(core.Push(form))
	form.SetValue("url", newURL)
	tm, _ = tm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return tm.(core.Router)
}

func TestProjectEditRefreshesMemoryAndDropsStaleSubmenu(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	projectRoot := t.TempDir()
	manifestPath := filepath.Join(projectRoot, "addon_manifest.yml")
	if err := addon.AddEntry(manifestPath, "demo", "https://github.com/owner/old", "addons/demo"); err != nil {
		t.Fatal(err)
	}

	c := appctx.New(projectRoot, "dev")
	sh := core.NewShared(c)
	r := core.NewRouter(sh, []core.TabEntry{{
		Title: appctx.TitleProject,
		New:   func(sh *core.Shared) core.Screen { return projecttab.NewProjectScreen(sh) },
	}})
	root := r.Top()
	r = submitEdit(t, r, editmanifest.New(manifestPath, c.ProjectAddons[0], appctx.ProjectDirty{}, false), "https://github.com/owner/new")

	if r.Top() != root {
		t.Fatalf("successful edit should return to refreshed project root, got %T", r.Top())
	}
	if got := c.ProjectAddons[0].URL; got != "https://github.com/owner/new" {
		t.Fatalf("project cache was not refreshed after edit: got %q", got)
	}
}

func TestGlobalEditRefreshesMemoryAndDropsStaleSubmenu(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	globalPath, err := addon.GlobalListPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := addon.AddEntry(globalPath, "demo", "https://github.com/owner/old", "addons/demo"); err != nil {
		t.Fatal(err)
	}

	c := appctx.New(t.TempDir(), "dev")
	sh := core.NewShared(c)
	r := core.NewRouter(sh, []core.TabEntry{{
		Title: appctx.TitleGlobal,
		New:   func(sh *core.Shared) core.Screen { return globaltab.NewGlobalScreen(sh) },
	}})
	root := r.Top()
	r = submitEdit(t, r, editmanifest.New(globalPath, c.GlobalAddons[0], appctx.GlobalDirty{}, true), "https://github.com/owner/new")

	if r.Top() != root {
		t.Fatalf("successful edit should return to refreshed global root, got %T", r.Top())
	}
	if got := c.GlobalAddons[0].URL; got != "https://github.com/owner/new" {
		t.Fatalf("global cache was not refreshed after edit: got %q", got)
	}
}
