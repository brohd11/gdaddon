package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/bubblestack/tuitest"

	tea "charm.land/bubbletea/v2"
)

// treeRows lists the tree's visible row titles, two spaces of indent per depth.
func treeRows(s *components.RootTreeScreen) string {
	var out []string
	for _, item := range s.Tree().List().VisibleItems() {
		row := item.(interface {
			Title() string
			PrefixText() string
		})
		prefix := []rune(row.PrefixText())
		out = append(out, string(prefix[:len(prefix)-2])+row.Title()) // drop the fold marker
	}
	return strings.Join(out, ",")
}

// viewFixture seeds a global list with categories and a project whose entries match some of
// them by repo, in a different url form.
func viewFixture(t *testing.T) (*appctx.Ctx, string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	gp, err := addon.GlobalListPath()
	if err != nil {
		t.Fatal(err)
	}
	for name, cat := range map[string]string{"hammer": "tools", "deep": "tools/editor", "brush": ""} {
		if err := addon.AddEntry(gp, name, "https://github.com/owner/"+name, "addons/"+name); err != nil {
			t.Fatal(err)
		}
		if err := addon.SetCategory(gp, name, cat); err != nil {
			t.Fatal(err)
		}
	}
	projectDir := t.TempDir()
	manifest := filepath.Join(projectDir, "addon_manifest.yml")
	for name, url := range map[string]string{
		"hammer": "https://github.com/Owner/hammer.git", // same repo, other url form
		"deep":   "https://www.github.com/owner/deep",
		"brush":  "https://github.com/owner/brush",
		"local":  "https://github.com/someone/local", // not in the global list
	} {
		if err := addon.AddEntry(manifest, name, url, "addons/"+name); err != nil {
			t.Fatal(err)
		}
	}
	return appctx.New(projectDir, "dev"), gp
}

func TestProjectTreeFilesAddonsByGlobalCategory(t *testing.T) {
	c, gp := viewFixture(t)
	sh := core.NewShared(c)
	tree := NewProjectTreeScreen(sh)
	tree.SetSize(sh, 100, 30)

	if got := treeRows(tree); got != "tools,brush,local" {
		t.Fatalf("matched addons go in collapsed folders, the rest at the top level; got %s", got)
	}
	tree.Update(sh, tuitest.KeyMsg("right"))
	if got := treeRows(tree); got != "tools,  editor,  hammer,brush,local" {
		t.Fatalf("opening tools: got %s", got)
	}

	// A category set in the Global tab moves the addon here on the same broadcast.
	if err := addon.SetCategory(gp, "brush", "art"); err != nil {
		t.Fatal(err)
	}
	tree.Receive(sh, appctx.GlobalDirty{})
	if got := treeRows(tree); got != "art,tools,  editor,  hammer,local" {
		t.Fatalf("GlobalDirty should refile brush and keep folds; got %s", got)
	}
}

func TestProjectViewToggleKeepsSortAndPicks(t *testing.T) {
	c, _ := viewFixture(t)
	sh := core.NewShared(c)
	var tm tea.Model = core.NewRouter(sh, []core.TabEntry{{
		Title: appctx.TitleProject,
		New:   func(sh *core.Shared) core.Screen { return NewProjectRoot(sh) },
	}})
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if _, ok := tm.(core.Router).Top().(*components.RootListScreen); !ok {
		t.Fatalf("the Project tab opens as a list, got %T", tm.(core.Router).Top())
	}

	sortKey := tuitest.KeyMsg(appctx.AppKeys.Sort.Keys()[0])
	viewKey := tuitest.KeyMsg(appctx.AppKeys.View.Keys()[0])
	tm = tuitest.Pump(tm, sortKey) // Z→A
	tm = tuitest.Pump(tm, viewKey)
	tree, ok := tm.(core.Router).Top().(*components.RootTreeScreen)
	if !ok {
		t.Fatalf("b should switch to the tree, got %T", tm.(core.Router).Top())
	}
	if title := tree.Tree().List().Title; !strings.Contains(title, "Z→A") {
		t.Fatalf("the sort mode should survive the switch, title %q", title)
	}
	if got := treeRows(tree); got != "tools,local,brush" {
		t.Fatalf("Z→A in the tree: got %s", got)
	}

	// Enter on a leaf opens its submenu (Update, not Pump: the submenu's Init may go online).
	tm, _ = tm.Update(tuitest.KeyMsg("down"))
	tm, _ = tm.Update(tuitest.KeyMsg("enter"))
	if _, ok := tm.(core.Router).Top().(*components.PickerScreen); !ok {
		t.Fatalf("enter on an addon leaf should push its submenu, got %T", tm.(core.Router).Top())
	}
	tm, _ = tm.Update(tuitest.KeyMsg("esc"))

	tm = tuitest.Pump(tm, viewKey)
	list, ok := tm.(core.Router).Top().(*components.RootListScreen)
	if !ok {
		t.Fatalf("b again should return to the list, got %T", tm.(core.Router).Top())
	}
	if !strings.Contains(list.List().Title, "Z→A") || c.ProjectTree {
		t.Fatalf("back in the list with the sort kept, title %q", list.List().Title)
	}
}
