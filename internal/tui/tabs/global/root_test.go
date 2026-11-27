package global

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/brohd11/gdaddon/internal/tui/flows/setcategory"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/bubblestack/tuitest"

	tea "charm.land/bubbletea/v2"
)

// seedGlobal writes a global list under a temp HOME: name → category ("" uncategorized).
func seedGlobal(t *testing.T, entries map[string]string) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	gp, err := addon.GlobalListPath()
	if err != nil {
		t.Fatal(err)
	}
	for name, cat := range entries {
		addEntry(t, gp, name, cat)
	}
	return gp
}

func addEntry(t *testing.T, gp, name, cat string) {
	t.Helper()
	if err := addon.AddEntry(gp, name, "https://github.com/owner/"+name, "addons/"+name); err != nil {
		t.Fatal(err)
	}
	if err := addon.SetCategory(gp, name, cat); err != nil {
		t.Fatal(err)
	}
}

// rows lists the visible row titles, indented two spaces per depth as the tree draws.
func rows(s *components.RootTreeScreen) string {
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

func newSizedScreen(sh *core.Shared) *components.RootTreeScreen {
	s := NewGlobalScreen(sh)
	s.SetSize(sh, 80, 24)
	return s
}

func TestGlobalTreeGroupsFoldsAndSorts(t *testing.T) {
	gp := seedGlobal(t, map[string]string{
		"hammer": "tools",
		"deep":   "tools/editor",
		"brush":  "art",
		"loose":  "",
		"apple":  "",
	})
	c := appctx.New(t.TempDir(), "dev")
	sh := core.NewShared(c)
	s := newSizedScreen(sh)

	if got := rows(s); got != "art,tools,apple,loose" {
		t.Fatalf("folders start collapsed and come before uncategorized plugins, got %s", got)
	}
	s.Update(sh, tuitest.KeyMsg("down"))  // tools
	s.Update(sh, tuitest.KeyMsg("right")) // open it
	want := "art,tools,  editor,  hammer,apple,loose"
	if got := rows(s); got != want {
		t.Fatalf("opening tools: got %s, want %s", got, want)
	}

	for _, item := range s.Tree().List().VisibleItems() {
		if row := item.(interface {
			Title() string
			SuffixText() string
		}); row.Title() == "hammer" && row.SuffixText() != "owner/hammer" {
			t.Fatalf("a GitHub leaf's suffix should be owner/repo, got %q", row.SuffixText())
		}
	}

	// A theme change reinstances the root: folds live on the Ctx.
	if got := rows(newSizedScreen(sh)); got != want {
		t.Fatalf("a rebuilt root lost its folds: got %s", got)
	}

	addEntry(t, gp, "anvil", "tools")
	s.Receive(sh, appctx.GlobalDirty{})
	if got := rows(s); got != "art,tools,  editor,  anvil,  hammer,apple,loose" {
		t.Fatalf("refresh should keep tools open and add anvil, got %s", got)
	}

	s.Update(sh, tuitest.KeyMsg(appctx.AppKeys.Sort.Keys()[0]))
	if got := rows(s); got != "tools,  editor,  hammer,  anvil,art,loose,apple" {
		t.Fatalf("Z→A should reverse folders and plugins at every level, got %s", got)
	}
	if !strings.Contains(s.Tree().List().Title, "Z→A") {
		t.Fatalf("title should show the sort mode, got %q", s.Tree().List().Title)
	}
}

func TestGlobalTreeEmptyPlaceholder(t *testing.T) {
	seedGlobal(t, nil)
	sh := core.NewShared(appctx.New(t.TempDir(), "dev"))
	if got := rows(newSizedScreen(sh)); got != "(no global plugins yet)" {
		t.Fatalf("empty list should show the placeholder, got %s", got)
	}
}

func TestSetCategoryMovesPluginAndPopsSubmenu(t *testing.T) {
	gp := seedGlobal(t, map[string]string{"demo": ""})
	c := appctx.New(t.TempDir(), "dev")
	sh := core.NewShared(c)
	var tm tea.Model = core.NewRouter(sh, []core.TabEntry{{
		Title: appctx.TitleGlobal,
		New:   func(sh *core.Shared) core.Screen { return NewGlobalScreen(sh) },
	}})
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	root := tm.(core.Router).Top().(*components.RootTreeScreen)

	a := c.GlobalAddons[0]
	g := globalItem{name: a.Name, display: a.Display, url: a.URL, path: a.Path}
	tm = tuitest.Pump(tm, core.Push(newSubmenuScreen(g, sh)))
	form := setcategory.New(setcategory.Opts{
		ManifestPath: gp, Name: g.name, Label: g.label(), Dirty: appctx.GlobalDirty{},
	})
	tm = tuitest.Pump(tm, core.Push(form))
	form.SetValue("category", "tools/ editor")
	tm = tuitest.Pump(tm, tuitest.KeyMsg("enter"))

	if top := tm.(core.Router).Top(); top != root {
		t.Fatalf("saving should pop the form and the stale submenu, top is %T", top)
	}
	if got := c.GlobalAddons[0].Category; got != "tools/editor" {
		t.Fatalf("category = %q, want tools/editor", got)
	}
	if got := rows(root); got != "tools" {
		t.Fatalf("the plugin should move under a collapsed tools folder, got %s", got)
	}
}

func TestImportToProjectLeavesCategoryToTheGlobalList(t *testing.T) {
	seedGlobal(t, map[string]string{"demo": "tools"})
	projectDir := t.TempDir()
	manifest := filepath.Join(projectDir, "addon_manifest.yml")
	if err := os.WriteFile(manifest, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	c := appctx.New(projectDir, "dev")
	sh := core.NewShared(c)
	a := c.GlobalAddons[0]
	importToProject(sh, globalItem{name: a.Name, url: a.URL, path: a.Path, category: a.Category})

	data, err := os.ReadFile(c.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "url:") || strings.Contains(string(data), "category") {
		t.Fatalf("import should add the entry without a category override:\n%s", data)
	}
}
