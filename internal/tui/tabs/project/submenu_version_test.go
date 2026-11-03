package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
)

func TestVersionEditorMenu(t *testing.T) {
	for _, tc := range []struct {
		name, file string
		kind       addon.Kind
		state      addon.State
		want       bool
	}{
		{"plugin", "plugin.cfg", addon.KindPackage, addon.StateInstalled, true},
		{"package", "package.cfg", addon.KindPackage, addon.StateInstalled, true},
		{"library", "addon.cfg", addon.KindPackage, addon.StateUnversioned, true},
		{"stamped", "version.cfg", addon.KindPackage, addon.StateMismatch, true},
		{"clone", "plugin.cfg", addon.KindClone, addon.StateInstalled, true},
		{"submodule", "plugin.cfg", addon.KindSubmodule, addon.StateInstalled, true},
		{"missing install", "plugin.cfg", addon.KindPackage, addon.StateMissing, false},
		{"no config", "", addon.KindClone, addon.StateInstalled, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.file != "" {
				if err := os.WriteFile(filepath.Join(dir, tc.file), []byte("[plugin]\nversion=\"1.2.3\"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			sh := core.NewShared(&appctx.Ctx{})
			st := addon.Status{Addon: addon.Addon{Name: "example", Kind: tc.kind}, State: tc.state, FullPath: dir, LocalVersion: "stale"}
			menu := newSubmenuScreen(st, sh)
			var edit *components.Item
			for _, item := range menu.List().Items() {
				if row, ok := item.(components.Item); ok && row.Name == "✎ Edit Version/Tag" {
					edit = &row
				}
			}
			if (edit != nil) != tc.want {
				t.Fatalf("editor offered = %v; want %v", edit != nil, tc.want)
			}
			if edit != nil {
				r := core.NewRouter(sh, []core.TabEntry{{Title: "Test", New: func(*core.Shared) core.Screen { return menu }}})
				m, _ := r.Update(edit.Pick(sh))
				f, ok := m.(core.Router).Top().(*components.FormScreen)
				if !ok || f.Value("version") != "1.2.4" {
					t.Fatal("menu must open the form using the current config, not the cached status")
				}
			}
		})
	}
}
