package addon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEntryKey covers the identity a new entry is keyed by, across the url shapes the
// manifest actually holds: a repo url, a clone url, a release asset, an Asset Store
// package, and something with no identity at all (which must keep recording what it
// always did).
func TestEntryKey(t *testing.T) {
	cases := []struct{ name, url, want string }{
		{"repo url", "https://github.com/Owner/My-Plugin", "github.com/owner/my-plugin"},
		{"clone url", "https://github.com/Owner/My-Plugin.git", "github.com/owner/my-plugin"},
		{"release asset", "https://github.com/Owner/My-Plugin/releases/download/v1.2.3/my-plugin.zip", "github.com/owner/my-plugin"},
		{"source archive", "https://github.com/Owner/My-Plugin/archive/refs/tags/v1.2.3.zip", "github.com/owner/my-plugin"},
		{"asset store", "https://store.godotengine.org/publisher/some-asset", "store.godotengine.org/publisher/some-asset"},
		{"no identity", "/tmp/local/my_addon.zip", "my_addon"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EntryKey(c.url); got != c.want {
				t.Errorf("EntryKey(%q) = %q, want %q", c.url, got, c.want)
			}
		})
	}
}

// TestLabelAndSlug pins the split the whole change turns on: the key is identity, Label
// is what a human reads, and Slug is what a folder may be named. A legacy key must come
// through all three unchanged.
func TestLabelAndSlug(t *testing.T) {
	cases := []struct {
		name      string
		a         Addon
		wantLabel string
		wantSlug  string
	}{
		{"legacy key", Addon{Name: "my_addon"}, "my_addon", "my_addon"},
		{"legacy key with a name", Addon{Name: "my_addon", Display: "My Addon"}, "My Addon", "my_addon"},
		{"identity key", Addon{Name: "github.com/owner/my-plugin"}, "my-plugin", "my-plugin"},
		{"identity key with a name", Addon{Name: "github.com/owner/my-plugin", Display: "My Plugin"}, "My Plugin", "my-plugin"},
		{"store key", Addon{Name: "store.godotengine.org/pub/slug"}, "slug", "slug"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.a.Label(); got != c.wantLabel {
				t.Errorf("Label() = %q, want %q", got, c.wantLabel)
			}
			if got := c.a.Slug(); got != c.wantSlug {
				t.Errorf("Slug() = %q, want %q", got, c.wantSlug)
			}
		})
	}
}

// TestSetDisplayName covers the one manifest field whose value is the addon author's
// free text: it must set, update, and clear like the other scalar writers, and it must
// survive characters that would otherwise end the YAML scalar early.
func TestSetDisplayName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "addon_manifest.yml")
	if err := AddEntry(path, "github.com/owner/repo", "https://github.com/owner/repo.git", "addons/repo"); err != nil {
		t.Fatal(err)
	}

	nameOf := func(t *testing.T) string {
		t.Helper()
		entries, err := Parse(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("Parse returned %d entries, want 1", len(entries))
		}
		return entries[0].Display
	}

	for _, want := range []string{
		"My Plugin",
		`Weird: "quoted" # name\with slashes`,
		"Renamed",
	} {
		if err := SetDisplayName(path, "github.com/owner/repo", want); err != nil {
			t.Fatalf("SetDisplayName(%q): %v", want, err)
		}
		if got := nameOf(t); got != want {
			t.Errorf("after setting %q, Parse read back %q", want, got)
		}
	}

	if err := SetDisplayName(path, "github.com/owner/repo", ""); err != nil {
		t.Fatal(err)
	}
	if got := nameOf(t); got != "" {
		t.Errorf("blank should remove the line; Parse read back %q", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "name:") {
		t.Errorf("cleared name left a line behind:\n%s", data)
	}
	// Everything else the entry recorded is untouched by all that.
	entries, _ := Parse(path)
	if entries[0].Name != "github.com/owner/repo" || entries[0].Path != "addons/repo" {
		t.Errorf("SetDisplayName disturbed the entry: %+v", entries[0])
	}
}

// TestAdoptName is the rule every install path relies on: record what the package calls
// itself, but only into a name that isn't already spoken for, and never touch the key.
func TestAdoptName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	setup := func(t *testing.T, block string) (string, Addon) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "addon_manifest.yml")
		if err := os.WriteFile(path, []byte(block), 0o644); err != nil {
			t.Fatal(err)
		}
		entries, err := Parse(path)
		if err != nil {
			t.Fatal(err)
		}
		return path, entries[0]
	}
	const unnamed = "github.com/owner/repo:\n    url: https://github.com/owner/repo.git\n    path: addons/repo\n"
	const named = "github.com/owner/repo:\n    name: \"Chosen\"\n    url: https://github.com/owner/repo.git\n    path: addons/repo\n"

	t.Run("adopts when absent", func(t *testing.T) {
		path, a := setup(t, unnamed)
		if err := AdoptName(path, a, InstallResult{Path: "addons/repo", Name: "My Plugin"}); err != nil {
			t.Fatal(err)
		}
		entries, _ := Parse(path)
		if entries[0].Display != "My Plugin" {
			t.Errorf("Display = %q, want My Plugin", entries[0].Display)
		}
		if entries[0].Name != "github.com/owner/repo" {
			t.Errorf("the key moved: %q", entries[0].Name)
		}
	})

	t.Run("never overwrites a recorded name", func(t *testing.T) {
		path, a := setup(t, named)
		if err := AdoptName(path, a, InstallResult{Path: "addons/repo", Name: "My Plugin"}); err != nil {
			t.Fatal(err)
		}
		entries, _ := Parse(path)
		if entries[0].Display != "Chosen" {
			t.Errorf("Display = %q, want the recorded Chosen", entries[0].Display)
		}
	})

	t.Run("no-op when the package declares nothing", func(t *testing.T) {
		path, a := setup(t, unnamed)
		before, _ := os.ReadFile(path)
		if err := AdoptName(path, a, InstallResult{Path: "addons/repo"}); err != nil {
			t.Fatal(err)
		}
		after, _ := os.ReadFile(path)
		if string(before) != string(after) {
			t.Errorf("manifest changed for a package that declares no name:\n%s", after)
		}
	})

	t.Run("rejects an unusable name", func(t *testing.T) {
		path, a := setup(t, unnamed)
		if err := AdoptName(path, a, InstallResult{Path: "addons/repo", Name: "bad\nname"}); err != nil {
			t.Fatal(err)
		}
		entries, _ := Parse(path)
		if entries[0].Display != "" {
			t.Errorf("a name with a newline should be ignored; got %q", entries[0].Display)
		}
	})
}

// TestAdoptNameBackfillsGlobal covers the global half: the same repo listed globally
// picks the name up too, matched by repo identity across a differing url and a
// differing key, and an already-named global entry is left alone.
func TestAdoptNameBackfillsGlobal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	globalPath, err := GlobalListPath()
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately unlike the project entry: a legacy key and the canonical repo url,
	// where the project entry is identity-keyed and pinned to a release asset.
	if err := AddEntry(globalPath, "repo", "https://github.com/owner/repo", ""); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "addon_manifest.yml")
	block := "github.com/owner/repo:\n    url: https://github.com/owner/repo/releases/download/v1.0.0/repo.zip\n    path: addons/repo\n"
	if err := os.WriteFile(path, []byte(block), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := AdoptName(path, entries[0], InstallResult{Path: "addons/repo", Name: "My Plugin"}); err != nil {
		t.Fatal(err)
	}
	globals, err := Parse(globalPath)
	if err != nil {
		t.Fatal(err)
	}
	if globals[0].Display != "My Plugin" {
		t.Fatalf("global entry Display = %q, want My Plugin", globals[0].Display)
	}
	if globals[0].Name != "repo" {
		t.Errorf("the global key moved: %q", globals[0].Name)
	}

	// A second install declaring something else must not rewrite what is now recorded.
	if err := AdoptName(path, entries[0], InstallResult{Path: "addons/repo", Name: "Renamed Upstream"}); err != nil {
		t.Fatal(err)
	}
	globals, _ = Parse(globalPath)
	if globals[0].Display != "My Plugin" {
		t.Errorf("global entry was overwritten: %q", globals[0].Display)
	}
}

// TestAdoptNameNoGlobalList is the ordinary case for a user who has never used the
// global library: there is nothing to back-fill and nothing may fail.
func TestAdoptNameNoGlobalList(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	path := filepath.Join(t.TempDir(), "addon_manifest.yml")
	block := "github.com/owner/repo:\n    url: https://github.com/owner/repo.git\n"
	if err := os.WriteFile(path, []byte(block), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, _ := Parse(path)
	if err := AdoptName(path, entries[0], InstallResult{Name: "My Plugin"}); err != nil {
		t.Fatalf("a missing global list must not fail an adopt: %v", err)
	}
}

// TestDepIndexMatchesIdentityKeyedEntry guards the upstream-rename fallback across the
// key change: a dep spec names a repo, so an identity-keyed entry must still be found by
// the repo name — and by the name the addon declares for itself.
func TestDepIndexMatchesIdentityKeyedEntry(t *testing.T) {
	// An entry whose url no longer parses to the declared id (the renamed-upstream
	// case), so only the name fallback can match it.
	entries := []Addon{{
		Name:    "github.com/brohd11/godot-tree-sitter-gd",
		Display: "Tree Sitter GD",
		URL:     "https://github.com/brohd11/godot-tree-sitter-gd/releases/download/v1.0.0/x.zip",
	}}
	ix := newDepIndex(entries)

	for _, spec := range []string{
		"https://github.com/brohd11/godot-tree-sitter-gd", // by the repo name
		"https://github.com/someone/Tree Sitter GD",       // by the declared name
	} {
		d := Dependency{RepoID: "github.com/nobody/unrelated", RepoURL: spec}
		if got := ix.find(d); got != 0 {
			t.Errorf("find(%q) = %d, want 0", spec, got)
		}
	}
}

// TestIdentityKeyRoundTripsThroughWriters checks the flat-manifest writers against a key
// carrying dots and slashes: it must be found, updated, and removed without touching the
// entry beside it.
func TestIdentityKeyRoundTripsThroughWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "addon_manifest.yml")
	if err := AddEntry(path, "github.com/owner/repo", "https://github.com/owner/repo.git", ""); err != nil {
		t.Fatal(err)
	}
	if err := AddEntry(path, "github.com/owner/other", "https://github.com/owner/other.git", "addons/other"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateEntry(path, "github.com/owner/repo", "", "addons/repo", "1.0.0", "v1.0.0"); err != nil {
		t.Fatal(err)
	}
	entries, err := Parse(path)
	if err != nil || len(entries) != 2 {
		t.Fatalf("Parse = %v, %v; want two entries", entries, err)
	}
	byKey := IndexByName(entries)
	if got := byKey["github.com/owner/repo"]; got.Path != "addons/repo" || got.Version != "1.0.0" {
		t.Fatalf("identity-keyed entry did not update: %+v", got)
	}
	if err := RemoveEntry(path, "github.com/owner/repo"); err != nil {
		t.Fatal(err)
	}
	entries, _ = Parse(path)
	if len(entries) != 1 || entries[0].Name != "github.com/owner/other" {
		t.Fatalf("removing one identity-keyed entry disturbed the other: %+v", entries)
	}
}

// TestInstallRecordsDeclaredName is the end-to-end shape of the feature: installing a
// package whose plugin.cfg names itself records that name beside the path and version,
// and a second install changes nothing.
func TestInstallRecordsDeclaredName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	ds := newDepServer(t)
	project := t.TempDir()

	url := ds.publish(t, "my_plugin", "1.2.3")
	manifest := writeManifest(t, project, "")

	res, err := InstallOne(context.Background(), InstallOneOpts{
		ManifestPath: manifest,
		ProjectRoot:  project,
		Entry:        Addon{Name: EntryKey(url), URL: url, Tag: "v1.2.3"},
		Report:       func(string, ...any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "addons/my_plugin" || res.Version != "1.2.3" {
		t.Fatalf("install landed wrong: %+v", res)
	}
	if res.Label() != "my_plugin" {
		t.Errorf("Label() = %q, want the declared my_plugin", res.Label())
	}

	entries, err := Parse(manifest)
	if err != nil || len(entries) != 1 {
		t.Fatalf("Parse = %v, %v; want one entry", entries, err)
	}
	got := entries[0]
	if got.Display != "my_plugin" {
		t.Errorf("Display = %q, want my_plugin", got.Display)
	}
	if !strings.HasSuffix(got.Name, "/o/my_plugin") {
		t.Errorf("entry key = %q, want the repo identity", got.Name)
	}
	if got.Path != "addons/my_plugin" || got.Version != "1.2.3" {
		t.Errorf("path/version pin regressed: %+v", got)
	}

	before, _ := os.ReadFile(manifest)
	if _, err := InstallOne(context.Background(), InstallOneOpts{
		ManifestPath: manifest,
		ProjectRoot:  project,
		Entry:        Addon{Name: EntryKey(url), URL: url, Tag: "v1.2.3"},
		Report:       func(string, ...any) {},
	}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(manifest)
	if string(before) != string(after) {
		t.Errorf("re-installing rewrote the entry:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestDependencyEntryIsKeyedAndNamed covers the other path that pins an entry: a
// dependency installed on an addon's behalf picks up the name its own config declares,
// exactly like a directly installed one. b is recorded but absent from disk, which is
// the branch that installs what the manifest already holds.
func TestDependencyEntryIsKeyedAndNamed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	ds := newDepServer(t)
	project := t.TempDir()

	bURL := ds.publish(t, "b", "1.0.0")
	aURL := ds.publish(t, "a", "1.0.0", ds.spec("b"))
	bKey := EntryKey(bURL)
	manifest := writeManifest(t, project, entryBlock(bKey, bURL))

	a := installRoot(t, EntryKey(aURL), aURL, project)
	if _, err := InstallDepsFor(context.Background(), manifest, a, project, nil, func(string, ...any) {}); err != nil {
		t.Fatal(err)
	}

	entries, err := Parse(manifest)
	if err != nil {
		t.Fatal(err)
	}
	dep, ok := IndexByName(entries)[bKey]
	if !ok {
		t.Fatalf("no entry keyed %q; entries: %+v", bKey, entries)
	}
	if dep.Display != "b" {
		t.Errorf("dependency Display = %q, want the declared b", dep.Display)
	}
	if dep.Path != "addons/b" {
		t.Errorf("dependency Path = %q, want addons/b", dep.Path)
	}
	if !strings.HasSuffix(dep.Name, "/o/b") {
		t.Errorf("dependency key = %q, want the repo identity", dep.Name)
	}
}

// TestAddEntryFullCarriesName covers the copy paths — exporting to the global list,
// importing a set — which hand a whole entry to AddEntryFull and would otherwise drop
// the one field that isn't url, path, or a pin.
func TestAddEntryFullCarriesName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.yml")
	src := Addon{Name: "github.com/owner/repo", Display: "My Plugin", URL: "https://github.com/owner/repo", Path: "addons/repo"}
	if err := AddEntryFull(path, src); err != nil {
		t.Fatal(err)
	}
	entries, err := Parse(path)
	if err != nil || len(entries) != 1 {
		t.Fatalf("Parse = %v, %v; want one entry", entries, err)
	}
	if entries[0].Display != "My Plugin" {
		t.Errorf("Display = %q, want My Plugin", entries[0].Display)
	}
}
