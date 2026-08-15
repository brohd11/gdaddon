package addon

import (
	"os"
	"path/filepath"
	"testing"
)

// declaring writes an installed addon dir under root whose plugin.cfg declares deps, and
// returns the manifest entry for it. It is PlanDeps' input shape: the entry says where the
// plugin.cfg is (Path) and what the user has suppressed.
func declaring(t *testing.T, root, name, url, relPath, deps string, suppress ...string) Addon {
	t.Helper()
	dir := filepath.Join(root, relPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[plugin]\nname=\"" + name + "\"\nversion=\"1.0.0\"\n"
	if deps != "" {
		cfg += "deps=" + deps + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.cfg"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return Addon{Name: name, URL: url, Path: relPath, SuppressDeps: suppress}
}

func repoIDs(deps []Dependency) []string {
	out := make([]string, len(deps))
	for i, d := range deps {
		out[i] = d.RepoID
	}
	return out
}

func staleIDs(stale []StaleDep) []string {
	out := make([]string, len(stale))
	for i, s := range stale {
		out[i] = s.Dep.RepoID
	}
	return out
}

func sameIDs(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s = %v, want %v", label, got, want)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s = %v, want %v", label, got, want)
			return
		}
	}
}

// TestPlanDeps covers the three-way classification: absent deps are addable, a satisfying
// (or unverifiable, or tagless-and-present) entry is satisfied, a verifiably older entry
// is stale, and a suppressed dep appears in no bucket at all.
func TestPlanDeps(t *testing.T) {
	root := t.TempDir()
	a := declaring(t, root, "A", "https://github.com/u/A", "addons/a",
		`["u/Absent", "u/OK@v1.0.0", "u/Old@v2.0.0", "u/Tagless", "u/Weird@v3.0.0", "u/Sup@v1.0.0"]`,
		"github.com/u/sup")

	manifest := []Addon{
		a,
		{Name: "OK", URL: "https://github.com/u/OK", Tag: "v1.5.0"},
		{Name: "Old", URL: "https://github.com/u/Old", Tag: "v1.0.0"},
		{Name: "Tagless", URL: "https://github.com/u/Tagless"},
		// A branch checkout records no comparable tag: unverifiable ⇒ trusted, not stale.
		{Name: "Weird", URL: "https://github.com/u/Weird", Tag: "nightly-2026-01-01"},
		{Name: "Sup", URL: "https://github.com/u/Sup", Tag: "v0.1.0"},
	}

	plan, err := PlanDeps(a, root, manifest)
	if err != nil {
		t.Fatal(err)
	}
	sameIDs(t, "Add", repoIDs(plan.Add), []string{"github.com/u/absent"})
	sameIDs(t, "Satisfied", repoIDs(plan.Satisfied),
		[]string{"github.com/u/ok", "github.com/u/tagless", "github.com/u/weird"})
	sameIDs(t, "Stale", staleIDs(plan.Stale), []string{"github.com/u/old"})

	if got := plan.Stale[0].Recorded; got != "v1.0.0" {
		t.Errorf("Stale[0].Recorded = %q, want the manifest's v1.0.0", got)
	}

	// MissingDeps is the "needs recording" view of the same classification, in declaration
	// order — which is why it walks the deps itself rather than concatenating the buckets.
	missing, err := MissingDeps(a, root, manifest)
	if err != nil {
		t.Fatal(err)
	}
	sameIDs(t, "MissingDeps", repoIDs(missing), []string{"github.com/u/absent", "github.com/u/old"})
}

// TestPlanDepsRenamedUpstream is the regression test for the upstream-rename fallback: a
// repo renamed upstream serves its assets under the NEW name, so the manifest entry's url
// parses to an id the declared dep spec — still naming the old one — never matches. The
// entry must still be found, by the name the dep would be recorded under.
//
// Without the fallback this dep classifies as Add, and committing that plan fails with
// "already added from <new-id>". The TUI's dep resolver had exactly that hole before it was
// moved onto PlanDeps; this test fails against a depIndex that only matches by repo id.
func TestPlanDepsRenamedUpstream(t *testing.T) {
	root := t.TempDir()
	// A declares the dependency under the repo's OLD name.
	a := declaring(t, root, "A", "https://github.com/u/A", "addons/a", `["brohd11/Godot-TreeSitter-Wrapper"]`)

	// The manifest recorded it after the rename: the url (and so the repo id) is the new
	// one, but the entry name is what DeriveName yields for the declared spec.
	manifest := []Addon{
		a,
		{Name: "Godot-TreeSitter-Wrapper", URL: "https://github.com/brohd11/godot-tree-sitter-gd"},
	}

	plan, err := PlanDeps(a, root, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Add) != 0 {
		t.Errorf("a renamed-upstream dep already in the manifest planned as an add: %v", repoIDs(plan.Add))
	}
	if len(plan.Satisfied) != 1 {
		t.Fatalf("Satisfied = %v, want the recorded entry to satisfy the dep", repoIDs(plan.Satisfied))
	}

	if missing, err := MissingDeps(a, root, manifest); err != nil || len(missing) != 0 {
		t.Errorf("MissingDeps = %v (err %v), want none — the entry is present under its recorded name", repoIDs(missing), err)
	}
}

// TestPlanDepsNotInstalled covers the guard every dependency reader needs: an entry with no
// recorded path has no plugin.cfg on disk, so it declares nothing.
func TestPlanDepsNotInstalled(t *testing.T) {
	plan, err := PlanDeps(Addon{Name: "A", URL: "https://github.com/u/A"}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Add)+len(plan.Satisfied)+len(plan.Stale) != 0 {
		t.Errorf("an addon with no path declared dependencies: %+v", plan)
	}
}

// TestDepIndexFind pins the lookup order itself: repo identity wins, the recorded name is
// the fallback, and an unmatched dep reports -1 rather than a zero position.
func TestDepIndexFind(t *testing.T) {
	entries := []Addon{
		{Name: "byname", URL: "https://github.com/u/renamed"},
		{Name: "other", URL: "https://github.com/u/other"},
	}
	ix := newDepIndex(entries)

	byRepo, ok := ParseRepoSpec("u/other")
	if !ok {
		t.Fatal("could not parse u/other")
	}
	if got := ix.find(byRepo); got != 1 {
		t.Errorf("find by repo id = %d, want 1", got)
	}

	byName, ok := ParseRepoSpec("u/byname")
	if !ok {
		t.Fatal("could not parse u/byname")
	}
	if got := ix.find(byName); got != 0 {
		t.Errorf("find by name fallback = %d, want 0", got)
	}

	absent, ok := ParseRepoSpec("u/nothere")
	if !ok {
		t.Fatal("could not parse u/nothere")
	}
	if got := ix.find(absent); got != -1 {
		t.Errorf("find of an absent dep = %d, want -1", got)
	}
}
