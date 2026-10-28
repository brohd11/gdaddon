package editversion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/bubblestack/tuitest"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
	"github.com/charmbracelet/x/ansi"
)

func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

type versionRoot struct {
	*components.PickerScreen
	dirty int
}

func (r *versionRoot) Receive(_ *core.Shared, payload any) core.Action {
	if _, ok := payload.(appctx.ProjectDirty); ok {
		r.dirty++
	}
	return core.Action{}
}

func versionRouter(f *components.FormScreen) (core.Router, *versionRoot, *core.Shared) {
	sh := core.NewShared(nil)
	sh.Chrome = &core.Chrome{Status: components.NewStatusLine()}
	root := &versionRoot{PickerScreen: components.NewPicker(nil, components.PickerOpts{})}
	r := core.NewRouter(sh, []core.TabEntry{{Title: "Test", New: func(*core.Shared) core.Screen { return root }}})
	m, _ := r.Update(core.Push(components.NewPicker(nil, components.PickerOpts{})))
	m, _ = m.Update(core.Push(f))
	return m.(core.Router), root, sh
}

func TestVersionFormPrefill(t *testing.T) {
	for _, tc := range []struct{ current, next, label string }{
		{"0.1.0", "0.1.1", "0.1.0"},
		{"v1.2.9", "v1.2.10", "v1.2.9"},
		{"nightly", "nightly", "nightly"},
		{"1.0.0-rc.1", "1.0.0-rc.1", "1.0.0-rc.1"},
		{"", "", "(no version)"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, filepath.Join(dir, "addon.cfg"), "[addon]\nversion=\""+tc.current+"\"\n")
			f, err := New(dir, "Example")
			if err != nil {
				t.Fatal(err)
			}
			if f.Value("version") != tc.next || f.FocusedKey() != "version" {
				t.Fatalf("value = %q, focus = %q", f.Value("version"), f.FocusedKey())
			}
			sh := core.NewShared(nil)
			f.Init(sh)
			f.SetSize(sh, 80, 20)
			view := ansi.Strip(f.View(sh))
			if !strings.Contains(view, tc.label+" → ") || !strings.Contains(view, "Example · addon.cfg") {
				t.Fatalf("missing current value or config heading:\n%s", view)
			}
			if f.CrumbLabel(false) != "Edit Version/Tag" {
				t.Fatal("wrong form breadcrumb")
			}
		})
	}
}

func TestVersionFormSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.cfg")
	body := "[plugin]\nversion=\"0.1.0\"\nname=\"Example\"\n"
	writeConfig(t, path, body)
	f, err := New(dir, "Example")
	if err != nil {
		t.Fatal(err)
	}
	r, root, _ := versionRouter(f)
	m, _ := r.Update(tuitest.KeyMsg("enter"))
	if m.(core.Router).Top() != root || root.dirty != 1 {
		t.Fatal("save must broadcast ProjectDirty and close the form and submenu")
	}
	got, _ := os.ReadFile(path)
	if string(got) != strings.Replace(body, "0.1.0", "0.1.1", 1) {
		t.Fatalf("unexpected saved config: %q", got)
	}
}

func TestVersionFormCancelAndErrors(t *testing.T) {
	for _, mode := range []string{"cancel", "blank", "stale", "deleted", "custom"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "plugin.cfg")
			body := "[plugin]\nversion=\"0.1.0\"\n"
			writeConfig(t, path, body)
			f, err := New(dir, "Example")
			if err != nil {
				t.Fatal(err)
			}
			r, root, sh := versionRouter(f)
			switch mode {
			case "blank":
				f.SetValue("version", "")
			case "stale":
				body = strings.Replace(body, "0.1.0", "0.2.0", 1)
				writeConfig(t, path, body)
			case "deleted":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "custom":
				f.SetValue("version", "release-2026-10-28")
			}
			key := "enter"
			if mode == "cancel" {
				key = "esc"
			}
			m, _ := r.Update(tuitest.KeyMsg(key))
			if mode == "custom" {
				got, _ := os.ReadFile(path)
				if !strings.Contains(string(got), "release-2026-10-28") || root.dirty != 1 {
					t.Fatal("custom versions must save")
				}
				return
			}
			if root.dirty != 0 {
				t.Fatal("cancel or failed save must not broadcast a change")
			}
			if mode == "cancel" {
				if m.(core.Router).Top() == f || m.(core.Router).Top() == root {
					t.Fatal("cancel must return to the submenu")
				}
			} else if m.(core.Router).Top() != f || !strings.Contains(sh.Chrome.Status.View(), "error:") {
				t.Fatal("failed save must keep the form open and report the error")
			}
			got, err := os.ReadFile(path)
			if mode == "deleted" {
				if !os.IsNotExist(err) {
					t.Fatal("save recreated a deleted config")
				}
			} else if err != nil || string(got) != body {
				t.Fatalf("cancel or failed save changed config: %q, %v", got, err)
			}
		})
	}
}
