package addon

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestNormalizeCategory(t *testing.T) {
	for in, want := range map[string]string{
		"":                   "",
		"  ":                 "",
		"/":                  "",
		"tools":              "tools",
		"tools/editor":       "tools/editor",
		" tools // editor/ ": "tools/editor",
		"/a/b/c/":            "a/b/c",
		"My Tools/Sub Cat":   "My Tools/Sub Cat",
	} {
		if got := NormalizeCategory(in); got != want {
			t.Errorf("NormalizeCategory(%q) = %q, want %q", in, got, want)
		}
	}
	if got := (Addon{Category: " a//b "}).CategoryPath(); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("CategoryPath = %q", got)
	}
	if got := (Addon{}).CategoryPath(); got != nil {
		t.Errorf("empty CategoryPath = %q, want nil", got)
	}
}

func TestSetCategoryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plugins.yml")
	for _, n := range []string{"github.com/owner/one", "github.com/owner/two"} {
		if err := AddEntry(path, n, "https://"+n, "addons/"+filepath.Base(n)); err != nil {
			t.Fatal(err)
		}
	}
	categoryOf := func(name string) string {
		t.Helper()
		entries, err := Parse(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.Name == name {
				return e.Category
			}
		}
		t.Fatalf("%s missing", name)
		return ""
	}

	for _, tc := range []struct{ in, want string }{
		{"tools/editor", "tools/editor"},
		{` odd "quoted"/ # name `, `odd "quoted"/# name`},
		{"//", ""}, // normalizes to empty: clears
	} {
		if err := SetCategory(path, "github.com/owner/one", tc.in); err != nil {
			t.Fatal(err)
		}
		if got := categoryOf("github.com/owner/one"); got != tc.want {
			t.Fatalf("SetCategory(%q) read back %q, want %q", tc.in, got, tc.want)
		}
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "category") {
		t.Fatalf("a cleared category should remove its line:\n%s", data)
	}
	if got := categoryOf("github.com/owner/two"); got != "" {
		t.Fatalf("other entry gained a category %q", got)
	}
}

func TestAddEntryFullWritesCategory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "addon_manifest.yml")
	if err := AddEntryFull(path, Addon{Name: "demo", URL: "https://github.com/o/demo", Path: "addons/demo", Category: " tools//editor "}); err != nil {
		t.Fatal(err)
	}
	if err := AddEntryFull(path, Addon{Name: "plain", URL: "https://github.com/o/plain", Path: "addons/plain"}); err != nil {
		t.Fatal(err)
	}
	entries, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range entries {
		got[e.Name] = e.Category
	}
	if got["demo"] != "tools/editor" || got["plain"] != "" {
		t.Fatalf("categories after AddEntryFull = %v", got)
	}
}
