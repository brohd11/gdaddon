package addon

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeCfg(t *testing.T, dir, name, version string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[plugin]\nname=\"" + name + "\"\nversion=\"" + version + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, "plugin.cfg"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanInstalled(t *testing.T) {
	root := t.TempDir()
	writeCfg(t, filepath.Join(root, "addons", "alpha"), "Alpha", "1.0.0")
	// version.cfg-only library.
	libDir := filepath.Join(root, "addons", "beta")
	os.MkdirAll(libDir, 0o755)
	os.WriteFile(filepath.Join(libDir, "version.cfg"), []byte("[plugin]\nversion=\"2.1\"\n"), 0o644)
	// Nested sub-addon must be pruned (parent already found).
	writeCfg(t, filepath.Join(root, "addons", "alpha", "sub"), "Sub", "9.9")
	// Dotfolder must be skipped.
	writeCfg(t, filepath.Join(root, ".godot", "cache_plugin"), "Cache", "0.1")

	got, err := ScanInstalled(root)
	if err != nil {
		t.Fatalf("ScanInstalled: %v", err)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Path < got[j].Path })

	if len(got) != 2 {
		t.Fatalf("found %d plugins, want 2: %+v", len(got), got)
	}
	if got[0].Path != "addons/alpha" || got[0].Name != "Alpha" || got[0].Version != "1.0.0" {
		t.Errorf("alpha = %+v", got[0])
	}
	// beta has no name key → falls back to folder basename.
	if got[1].Path != "addons/beta" || got[1].Name != "beta" || got[1].Version != "2.1" {
		t.Errorf("beta = %+v", got[1])
	}
}

func TestUntrackedInstalls(t *testing.T) {
	root := t.TempDir()
	writeCfg(t, filepath.Join(root, "addons", "alpha"), "alpha", "1.0.0") // tracked by path
	writeCfg(t, filepath.Join(root, "addons", "cogito"), "Cogito", "3.0") // tracked by url, no path
	writeCfg(t, filepath.Join(root, "addons", "extra"), "Extra", "0.5")   // untracked

	manifest := filepath.Join(root, "addon_manifest.yml")
	body := "" +
		"alpha:\n  url: https://github.com/x/alpha.git\n  path: addons/alpha\n" +
		"cogito:\n  url: https://github.com/Maaack/Cogito.git\n"
	if err := os.WriteFile(manifest, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := UntrackedInstalls(manifest, root)
	if err != nil {
		t.Fatalf("UntrackedInstalls: %v", err)
	}
	sort.Slice(got, func(i, j int) bool { return got[i].Path < got[j].Path })

	if len(got) != 2 {
		t.Fatalf("untracked = %d, want 2 (cogito, extra): %+v", len(got), got)
	}
	// cogito: pathless entry matched by folder basename → SuggestedURL prefilled.
	if got[0].Path != "addons/cogito" || got[0].SuggestedURL != "https://github.com/Maaack/Cogito.git" {
		t.Errorf("cogito = %+v", got[0])
	}
	// extra: no matching entry → no suggestion.
	if got[1].Path != "addons/extra" || got[1].SuggestedURL != "" {
		t.Errorf("extra = %+v", got[1])
	}
}
