package addon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDependencyList(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []Dependency
	}{
		{"empty", "", nil},
		{"empty brackets", "[]", nil},
		{
			"owner/repo defaults host, quoted + unquoted",
			`["TokisanGames/Terrain3D@v1.0.1", brohd11/godot-plugin-devtools@v0.2.0]`,
			[]Dependency{
				{Host: "github.com", Owner: "TokisanGames", Repo: "Terrain3D", Tag: "v1.0.1", RepoURL: "https://github.com/TokisanGames/Terrain3D", RepoID: "github.com/tokisangames/terrain3d"},
				{Host: "github.com", Owner: "brohd11", Repo: "godot-plugin-devtools", Tag: "v0.2.0", RepoURL: "https://github.com/brohd11/godot-plugin-devtools", RepoID: "github.com/brohd11/godot-plugin-devtools"},
			},
		},
		{
			"explicit host",
			`["codeberg.org/u/Foo@1.2.3"]`,
			[]Dependency{
				{Host: "codeberg.org", Owner: "u", Repo: "Foo", Tag: "1.2.3", RepoURL: "https://codeberg.org/u/Foo", RepoID: "codeberg.org/u/foo"},
			},
		},
		{
			"tagless owner/repo is added version-less",
			`["u/NoTag", "u/Good@v1.0.0"]`,
			[]Dependency{
				{Host: "github.com", Owner: "u", Repo: "NoTag", Tag: "", RepoURL: "https://github.com/u/NoTag", RepoID: "github.com/u/notag"},
				{Host: "github.com", Owner: "u", Repo: "Good", Tag: "v1.0.0", RepoURL: "https://github.com/u/Good", RepoID: "github.com/u/good"},
			},
		},
		{
			"malformed items are skipped",
			`["single-segment", "@v1.0.0", "a/b/c/d@v1"]`,
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseDependencyList(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d deps, want %d: %+v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("dep %d:\n got %+v\nwant %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestDependenciesFromCfg(t *testing.T) {
	dir := t.TempDir()
	cfg := "[plugin]\nname=\"Demo\"\nversion=\"1.0.0\"\ndeps=[\"u/Dep@v1.2.0\"]\n"
	if err := os.WriteFile(filepath.Join(dir, "plugin.cfg"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	deps, err := Dependencies(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0].RepoID != "github.com/u/dep" || deps[0].Tag != "v1.2.0" {
		t.Errorf("unexpected deps: %+v", deps)
	}

	// No plugin.cfg → no deps, no error.
	if deps, err := Dependencies(t.TempDir()); err != nil || deps != nil {
		t.Errorf("expected nil deps for a dir without plugin.cfg; got %+v err=%v", deps, err)
	}
}

func TestMissingDeps(t *testing.T) {
	root := t.TempDir()
	addonDir := filepath.Join(root, "addons", "a")
	if err := os.MkdirAll(addonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `[plugin]
name="A"
deps=["u/Present@v1.0.0", "u/Absent@v1.0.0", "u/Stale@v2.0.0", "u/Tagless"]
`
	if err := os.WriteFile(filepath.Join(addonDir, "plugin.cfg"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	a := Addon{Name: "A", URL: "https://github.com/u/A", Path: "addons/a"}
	manifest := []Addon{
		a,
		{Name: "Present", URL: "https://github.com/u/Present", Tag: "v1.1.0"}, // satisfies >=1.0.0
		{Name: "Stale", URL: "https://github.com/u/Stale", Tag: "v1.0.0"},     // < v2.0.0 → missing
		{Name: "Tagless", URL: "https://github.com/u/Tagless"},                // present → satisfies tagless
		// u/Absent has no entry → missing
	}

	missing, err := MissingDeps(a, root, manifest)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, d := range missing {
		got[d.RepoID] = true
	}
	if len(got) != 2 || !got["github.com/u/absent"] || !got["github.com/u/stale"] {
		t.Errorf("expected Absent + Stale missing, got %v", got)
	}

	// An addon with no recorded path declares nothing (not installed).
	if missing, err := MissingDeps(Addon{Name: "A", URL: a.URL}, root, manifest); err != nil || missing != nil {
		t.Errorf("expected nil for an addon with no path; got %v err=%v", missing, err)
	}
}

func TestSemverGE(t *testing.T) {
	cases := []struct {
		a, b           string
		wantGE, wantOK bool
	}{
		{"v1.3.0", "v1.2.0", true, true},
		{"v1.0.0", "v1.2.0", false, true},
		{"1.2.0", "v1.2.0", true, true}, // equal, mixed v-prefix
		{"v1.2", "v1.2.0", true, true},  // shorter == longer when trailing zeros
		{"v2.0.0-rc1", "v1.0.0", true, true},
		{"2024-01-02", "v1.0.0", false, false}, // non-numeric → not comparable
		{"head", "v1.0.0", false, false},
	}
	for _, tc := range cases {
		ge, ok := semverGE(tc.a, tc.b)
		if ge != tc.wantGE || ok != tc.wantOK {
			t.Errorf("semverGE(%q,%q) = (%v,%v), want (%v,%v)", tc.a, tc.b, ge, ok, tc.wantGE, tc.wantOK)
		}
	}
}

func TestSatisfiedByTag(t *testing.T) {
	d := Dependency{Tag: "v1.2.0"}
	if sat, verified := d.SatisfiedByTag("v1.3.0"); !sat || !verified {
		t.Errorf("v1.3.0 should satisfy >=v1.2.0 (verified)")
	}
	if sat, verified := d.SatisfiedByTag("v1.0.0"); sat || !verified {
		t.Errorf("v1.0.0 should not satisfy >=v1.2.0 (verified)")
	}
	if sat, verified := d.SatisfiedByTag(""); sat || verified {
		t.Errorf("a tag-less (HEAD) install should be unverified, not satisfied")
	}
}
