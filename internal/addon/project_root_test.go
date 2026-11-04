package addon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestProjectRoot(t *testing.T) {
	for _, tc := range []struct {
		name, field, rel string
		invalid          bool
	}{
		{name: "omitted", rel: "."},
		{name: "empty", field: `project_root: ""`, rel: "."},
		{name: "dot", field: "project_root: .", rel: "."},
		{name: "child", field: "project_root: godot", rel: "godot"},
		{name: "parent", field: "project_root: ..", rel: ".."},
		{name: "clean relative", field: "project_root: ../project/./godot", rel: "../project/godot"},
		{name: "absolute", field: "project_root: /tmp/project", invalid: true},
		{name: "number", field: "project_root: 42", invalid: true},
		{name: "mapping", field: "project_root: {url: old-addon}", invalid: true},
		{name: "sequence", field: "project_root: [godot]", invalid: true},
		{name: "duplicate", field: "project_root: godot\nproject_root: ..", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "addon_manifest.yml")
			if err := os.WriteFile(path, []byte(tc.field+"\n"+sampleManifest), 0o644); err != nil {
				t.Fatal(err)
			}
			m, err := LoadManifest(path)
			if tc.invalid {
				if err == nil || !strings.Contains(err.Error(), "project_root") {
					t.Fatalf("expected project_root diagnostic, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if m.ProjectRoot != filepath.Clean(filepath.Join(dir, tc.rel)) || len(m.Addons) != 2 {
				t.Fatalf("manifest = %+v", m)
			}
			entries, err := Parse(path)
			if err != nil || len(entries) != 2 || entries[0].Name != "PluginDevTools" {
				t.Fatalf("Parse = %+v, %v", entries, err)
			}
		})
	}
}

func TestProjectRootSurvivesEntryWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "addon_manifest.yml")
	const settings = "# project settings\nproject_root: \"godot\" # relative directory\n\n"
	if err := os.WriteFile(path, []byte(settings+sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func() error{
		func() error { return AddEntry(path, "new", "https://github.com/u/new", "") },
		func() error { return EditEntry(path, "new", "https://github.com/u/new", "addons/new", "1", "v1") },
		func() error { return SetLock(path, "new", true) },
		func() error { return RemoveEntry(path, "new") },
	} {
		if err := edit(); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil || !strings.HasPrefix(string(data), settings) {
			t.Fatalf("settings changed: %s, %v", data, err)
		}
	}
	before, _ := os.ReadFile(path)
	for _, edit := range []func() error{
		func() error { return AddEntry(path, "project_root", "https://github.com/u/other", "") },
		func() error { return RemoveEntry(path, "project_root") },
		func() error { return SetLock(path, "project_root", true) },
	} {
		if err := edit(); err == nil {
			t.Fatal("metadata was accepted as an addon entry")
		}
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("reserved-key writes changed the manifest")
	}
}

func TestCreateProjectManifest(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{".", "tools", "tools/nested"} {
		path := filepath.Join(root, dir, "addon_manifest.yml")
		if err := CreateProjectManifest(path, root); err != nil {
			t.Fatal(err)
		}
		m, err := LoadManifest(path)
		if err != nil || m.ProjectRoot != root || len(m.Addons) != 0 {
			t.Fatalf("created manifest = %+v, %v", m, err)
		}
		if err := CreateProjectManifest(path, root); err == nil {
			t.Fatal("overwrote an existing manifest")
		}
	}
}

func TestSetSeedOmitsProjectRoot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	path := filepath.Join(t.TempDir(), "addon_manifest.yml")
	const entries = "# addon comment\n" + sampleManifest
	if err := os.WriteFile(path, []byte("project_root: godot\n\n"+entries), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := CreateSetFrom("reusable", path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(set)
	if err != nil || string(data) != entries {
		t.Fatalf("seed should contain only unchanged entries: %s, %v", data, err)
	}
	// Importing entries keeps the destination project's root.
	dest := filepath.Join(t.TempDir(), "addon_manifest.yml")
	if err := os.WriteFile(dest, []byte("project_root: other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	addons, err := Parse(set)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range addons {
		if err := AddEntryFull(dest, a); err != nil {
			t.Fatal(err)
		}
	}
	m, err := LoadManifest(dest)
	if err != nil || m.ProjectRoot != filepath.Join(filepath.Dir(dest), "other") || len(m.Addons) != 2 {
		t.Fatalf("import changed project settings: %+v, %v", m, err)
	}
}
