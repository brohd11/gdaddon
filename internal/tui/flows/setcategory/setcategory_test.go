package setcategory_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/flows/setcategory"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/bubblestack/tuitest"

	tea "charm.land/bubbletea/v2"
)

type dirty struct{}

// recorder is a root that records broadcasts it receives.
type recorder struct {
	*components.PickerScreen
	got *[]any
}

func (r recorder) Receive(_ *core.Shared, payload any) core.Action {
	*r.got = append(*r.got, payload)
	return core.Action{}
}

// save runs the form over a root and a stand-in submenu, submitting value.
func save(t *testing.T, o setcategory.Opts, value string) (top core.Screen, root core.Screen, broadcasts []any) {
	t.Helper()
	sh := core.NewShared(nil)
	var got []any
	rootScreen := recorder{PickerScreen: components.NewPicker(nil, components.PickerOpts{Title: "root"}), got: &got}
	var tm tea.Model = core.NewRouter(sh, []core.TabEntry{{Title: "T", New: func(*core.Shared) core.Screen { return rootScreen }}})
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	tm = tuitest.Pump(tm, core.Push(components.NewPicker(nil, components.PickerOpts{Title: "submenu"})))
	form := setcategory.New(o)
	tm = tuitest.Pump(tm, core.Push(form))
	form.SetValue("category", value)
	tm = tuitest.Pump(tm, tuitest.KeyMsg("enter"))
	return tm.(core.Router).Top(), rootScreen, got
}

func categoryOf(t *testing.T, path string) string {
	t.Helper()
	entries, err := addon.Parse(path)
	if err != nil || len(entries) != 1 {
		t.Fatalf("parse: %v (%d entries)", err, len(entries))
	}
	return entries[0].Category
}

func TestSetCategorySavesClearsAndPops(t *testing.T) {
	path := filepath.Join(t.TempDir(), "addon_manifest.yml")
	if err := addon.AddEntry(path, "demo", "https://github.com/o/demo", "addons/demo"); err != nil {
		t.Fatal(err)
	}
	o := setcategory.Opts{ManifestPath: path, Name: "demo", Label: "Demo", Dirty: dirty{}}

	top, root, got := save(t, o, " tools // editor ")
	if top != root {
		t.Fatalf("saving should pop the form and the submenu, top is %T", top)
	}
	if c := categoryOf(t, path); c != "tools/editor" {
		t.Fatalf("category = %q", c)
	}
	sawDirty := false
	for _, p := range got {
		if _, ok := p.(dirty); ok {
			sawDirty = true
		}
	}
	if !sawDirty {
		t.Fatalf("Dirty should be broadcast, got %v", got)
	}

	o.Current = "tools/editor"
	save(t, o, "")
	if c := categoryOf(t, path); c != "" {
		t.Fatalf("blank should clear, got %q", c)
	}
}

func TestPlaceholders(t *testing.T) {
	if got := setcategory.PlaceholderInherit("tools"); got != "blank uses global: tools" {
		t.Fatalf("PlaceholderInherit = %q", got)
	}
	if got := setcategory.PlaceholderInherit(""); !strings.Contains(got, "global") {
		t.Fatalf("PlaceholderInherit(\"\") = %q", got)
	}
}
