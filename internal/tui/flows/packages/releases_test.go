package packages

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/source"
)

func TestMergedReleasePickerSemanticOrder(t *testing.T) {
	remote := []source.Release{
		{Tag: "v1.9.0", Assets: []source.Asset{{Name: "addon.zip", URL: "https://download/old.zip"}}},
		{Tag: "v1.10.0-beta2", Assets: []source.Asset{{Name: "addon.zip", URL: "https://download/beta.zip"}}},
	}
	archived := []source.Release{
		{Tag: "v1.10.0", Assets: []source.Asset{{Name: "addon.zip (archived)", URL: "/tmp/stable.zip"}}},
		{Tag: "v1.10.0-beta2", Assets: []source.Asset{{Name: "addon.zip (archived)", URL: "/tmp/beta.zip"}}},
	}
	releases := append(append([]source.Release(nil), remote...), archiveOnly(remote, archived)...)
	before := append([]source.Release(nil), releases...)
	var selected []Selection
	opts := BrowseOpts{Source: SourceAll, IncludeHEAD: true, LeadItems: []list.Item{components.Item{Name: "Reinstall"}},
		Endpoint: func(s Selection) core.Screen {
			selected = append(selected, s)
			return components.NewPicker(nil, components.PickerOpts{Title: s.Tag})
		},
	}
	picker := newVersionsPicker("github.com/u/r", "https://github.com/u/r", opts, releases, buildArchivedSet(archived))
	sh := core.NewShared(nil)
	picker.SetSize(sh, 100, 30)
	var items []components.Item
	picker.OnSelect = func(_ *core.Shared, item list.Item) core.Action {
		items = append(items, item.(components.Item))
		return core.Action{}
	}
	want := []string{"Reinstall", "Tags", "HEAD", "v1.10.0", "v1.10.0-beta2", "v1.9.0"}
	for range want {
		picker.Update(sh, tea.KeyPressMsg{Code: tea.KeyEnter})
		picker.Update(sh, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if len(items) != len(want) {
		t.Fatalf("items=%v", items)
	}
	for i, name := range want {
		if items[i].Name != name {
			t.Fatalf("row %d = %q, want %q", i, items[i].Name, name)
		}
	}
	if !strings.Contains(items[3].Desc, opts.marker()) || !strings.Contains(items[4].Desc, opts.marker()) || !strings.Contains(items[4].Desc, "prerelease") {
		t.Fatalf("missing release annotations: %q / %q", items[3].Desc, items[4].Desc)
	}
	items[3].Pick(sh)
	items[4].Pick(sh)
	if len(selected) != 2 || !selected[0].Archived || selected[0].Asset.URL != "/tmp/stable.zip" ||
		!selected[1].Prerelease || selected[1].Asset.URL != "https://download/beta.zip" || selected[1].ArchivedAsset.URL != "/tmp/beta.zip" {
		t.Fatalf("selections=%+v", selected)
	}
	if !reflect.DeepEqual(releases, before) {
		t.Fatal("picker changed the input listing")
	}
}
