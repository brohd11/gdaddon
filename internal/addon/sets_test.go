package addon

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Sets resolve under ~/.gdaddon/sets via os.UserHomeDir(), so each test overrides
// $HOME to a temp dir (the pattern config_test.go uses) to isolate the real home.

func TestCreateSetEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path, err := CreateSet("foo")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".gdaddon", "sets", "foo.yml")
	if path != want {
		t.Errorf("CreateSet path = %q, want %q", path, want)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("set file not created: %v", err)
	}
	addons, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(addons) != 0 {
		t.Errorf("empty set should parse to 0 entries, got %d", len(addons))
	}
	if sp, _ := SetPath("foo"); sp != want {
		t.Errorf("SetPath = %q, want %q", sp, want)
	}
	names, err := ListSets()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"foo"}) {
		t.Errorf("ListSets = %v, want [foo]", names)
	}
}

// TestCreateSetFromSeedPreservesEntries is the Project→Set seed path: CreateSetFrom
// is a verbatim copy of the source manifest, so every field of every entry (incl. a
// clone kind) survives both byte-for-byte and through a Parse round-trip.
func TestCreateSetFromSeedPreservesEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projPath := filepath.Join(home, "proj", "addon_manifest.yml")
	if err := os.MkdirAll(filepath.Dir(projPath), 0o755); err != nil {
		t.Fatal(err)
	}
	const project = `Alpha:
    url: https://github.com/u/Alpha.git
    path: addons/alpha
    version: "2.0.0"
    tag: "v2.0.0"
    kind: clone

Beta:
    url: https://github.com/u/Beta/releases/download/v3.1.0/beta-3.1.0.zip
    path: addons/beta
    version: "3.1.0"
`
	if err := os.WriteFile(projPath, []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}

	setFile, err := CreateSetFrom("seeded", projPath)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(setFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(seed) != project {
		t.Errorf("set is not a verbatim copy of the project manifest:\n%s", seed)
	}

	addons, err := Parse(setFile)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Addon{}
	for _, a := range addons {
		byName[a.Name] = a
	}
	alpha := byName["Alpha"]
	if alpha.URL != "https://github.com/u/Alpha.git" || alpha.Path != "addons/alpha" ||
		alpha.Version != "2.0.0" || alpha.Tag != "v2.0.0" || alpha.Kind != KindClone {
		t.Errorf("Alpha fields lost in seed: %+v", alpha)
	}
}

func TestCreateSetRefusesOverwrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projPath := filepath.Join(home, "addon_manifest.yml")
	if err := os.WriteFile(projPath, []byte("Alpha:\n    url: https://github.com/u/Alpha.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateSetFrom("dup", projPath); err != nil {
		t.Fatal(err)
	}

	// A second create with the same name from a different source must error and
	// leave the existing set untouched.
	other := filepath.Join(home, "other.yml")
	if err := os.WriteFile(other, []byte("Beta:\n    url: https://github.com/u/Beta.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateSetFrom("dup", other); err == nil {
		t.Fatal("expected the duplicate-name create to be refused")
	}
	sp, _ := SetPath("dup")
	got, _ := os.ReadFile(sp)
	if !strings.Contains(string(got), "Alpha:") || strings.Contains(string(got), "Beta:") {
		t.Errorf("set should be untouched on a refused overwrite; got:\n%s", got)
	}
}

func TestListSetsAndDelete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// A missing sets dir reads as an empty list.
	names, err := ListSets()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 0 {
		t.Errorf("missing sets dir should be empty, got %v", names)
	}

	for _, n := range []string{"charlie", "alpha", "bravo"} {
		if _, err := CreateSet(n); err != nil {
			t.Fatal(err)
		}
	}
	// A non-.yml file and a directory (even one named like a set) must be ignored.
	dir, _ := SetsDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub.yml"), 0o755); err != nil {
		t.Fatal(err)
	}

	names, err = ListSets()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"alpha", "bravo", "charlie"}; !reflect.DeepEqual(names, want) {
		t.Errorf("ListSets = %v, want %v (sorted, filtered)", names, want)
	}

	if err := DeleteSet("bravo"); err != nil {
		t.Fatal(err)
	}
	names, _ = ListSets()
	if want := []string{"alpha", "charlie"}; !reflect.DeepEqual(names, want) {
		t.Errorf("after delete ListSets = %v, want %v", names, want)
	}
}
