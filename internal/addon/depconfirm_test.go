package addon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recorder is a DepConfirmer that logs what it was offered and answers from a table
// keyed by the dependency's repo id (default: yes).
type recorder struct {
	answer map[string]bool
	abort  map[string]bool
	seen   []DepRequest
}

func newRecorder() *recorder {
	return &recorder{answer: map[string]bool{}, abort: map[string]bool{}}
}

func (r *recorder) confirm(req DepRequest) (bool, error) {
	r.seen = append(r.seen, req)
	if r.abort[req.Dep.RepoID] {
		return false, ErrDepAborted
	}
	if ok, set := r.answer[req.Dep.RepoID]; set {
		return ok, nil
	}
	return true, nil
}

// offered reports whether a dependency was ever put to the confirmer.
func (r *recorder) offered(repoID string) bool {
	for _, req := range r.seen {
		if req.Dep.RepoID == repoID {
			return true
		}
	}
	return false
}

func (r *recorder) count(repoID string) int {
	n := 0
	for _, req := range r.seen {
		if req.Dep.RepoID == repoID {
			n++
		}
	}
	return n
}

// installRoot installs one addon directly (the "user named this one" step), returning
// it with its resolved path — the shape InstallDepsFor takes as its root.
func installRoot(t *testing.T, name, url, project string) Addon {
	t.Helper()
	a := Addon{Name: name, URL: url}
	res, err := Install(context.Background(), a, project, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	a.Path = res.Path
	return a
}

// TestInstallDepsForDeclineSkipsSubtree is the property that makes confirmation
// meaningful: declining b in a → b → c must not install b, must not record it, and must
// never even *ask* about c — refusing a package refuses everything under it.
func TestInstallDepsForDeclineSkipsSubtree(t *testing.T) {
	ds := newDepServer(t)
	project := t.TempDir()

	ds.publish(t, "c", "1.0.0")
	ds.publish(t, "b", "1.0.0", ds.spec("c"))
	aURL := ds.publish(t, "a", "1.0.0", ds.spec("b"))

	manifest := writeManifest(t, project, "")
	a := installRoot(t, "a", aURL, project)

	rec := newRecorder()
	rec.answer[strings.ToLower(ds.spec("b"))] = false

	outcomes, err := InstallDepsFor(context.Background(), manifest, a, project, rec.confirm, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 0 {
		t.Fatalf("installed %+v, want nothing (b was declined)", outcomes)
	}
	if rec.offered(strings.ToLower(ds.spec("c"))) {
		t.Error("c was offered; declining b must not walk into what b declares")
	}
	for _, name := range []string{"b", "c"} {
		if _, err := os.Stat(filepath.Join(project, "addons", name)); err == nil {
			t.Errorf("%s was installed despite the decline", name)
		}
	}

	// Declining must leave no manifest entry behind — the whole reason the confirm
	// happens before the write rather than before the download.
	entries, err := Parse(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name == "b" || e.Name == "c" {
			t.Errorf("declined dependency %q was recorded in the manifest: %+v", e.Name, e)
		}
	}
}

// TestInstallDepsForAbort checks a confirmer error stops the walk and propagates, while
// leaving the dependencies already installed before it in place.
func TestInstallDepsForAbort(t *testing.T) {
	ds := newDepServer(t)
	project := t.TempDir()

	bURL := ds.publish(t, "b", "1.0.0")
	cURL := ds.publish(t, "c", "1.0.0")
	aURL := ds.publish(t, "a", "1.0.0", ds.spec("b"), ds.spec("c"))

	// Both deps are pre-recorded at their release urls (as in TestInstallDepsForChain):
	// a tagless dep with no entry is added repo-only and cloned, which this server can't
	// serve, and the point here is what happens around a *successful* install.
	manifest := writeManifest(t, project, entryBlock("b", bURL)+entryBlock("c", cURL))
	a := installRoot(t, "a", aURL, project)

	rec := newRecorder()
	rec.abort[strings.ToLower(ds.spec("c"))] = true

	outcomes, err := InstallDepsFor(context.Background(), manifest, a, project, rec.confirm, func(string, ...any) {})
	if !errors.Is(err, ErrDepAborted) {
		t.Fatalf("err = %v, want ErrDepAborted", err)
	}
	if len(outcomes) != 1 || outcomes[0].Name != "b" {
		t.Fatalf("outcomes = %+v, want just b (installed before the abort)", outcomes)
	}
	if _, err := os.Stat(filepath.Join(project, "addons", "b", "plugin.cfg")); err != nil {
		t.Errorf("b should have survived the abort: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "addons", "c")); err == nil {
		t.Error("c was installed despite the abort")
	}
}

// TestInstallDepsForConfirmsReinstall covers the branch that has no manifest write at
// all: an entry that is already recorded but whose files are gone still downloads, so
// it still has to be confirmed.
func TestInstallDepsForConfirmsReinstall(t *testing.T) {
	ds := newDepServer(t)
	project := t.TempDir()

	ds.publish(t, "b", "1.0.0")
	aURL := ds.publish(t, "a", "1.0.0", ds.spec("b"))

	manifest := writeManifest(t, project, "")
	a := installRoot(t, "a", aURL, project)

	// First pass, unattended: records and installs b.
	if _, err := InstallDepsFor(context.Background(), manifest, a, project, nil, func(string, ...any) {}); err != nil {
		t.Fatal(err)
	}

	// Delete the files, keep the entry.
	if err := os.RemoveAll(filepath.Join(project, "addons", "b")); err != nil {
		t.Fatal(err)
	}

	rec := newRecorder()
	rec.answer[strings.ToLower(ds.spec("b"))] = false
	if _, err := InstallDepsFor(context.Background(), manifest, a, project, rec.confirm, func(string, ...any) {}); err != nil {
		t.Fatal(err)
	}

	if len(rec.seen) != 1 {
		t.Fatalf("confirmer saw %d requests, want 1: %+v", len(rec.seen), rec.seen)
	}
	if got := rec.seen[0].Action; got != DepReinstall {
		t.Errorf("Action = %v, want DepReinstall", got)
	}
	if rec.seen[0].DeclaredBy != "a" {
		t.Errorf("DeclaredBy = %q, want %q", rec.seen[0].DeclaredBy, "a")
	}
	if _, err := os.Stat(filepath.Join(project, "addons", "b")); err == nil {
		t.Error("b was re-installed despite the decline")
	}
}

// TestInstallDepsForRequestFields checks the confirmer is handed something it can
// actually show a user: the declaring addon, the resolved download url, and the entry
// name the dependency would be recorded as.
func TestInstallDepsForRequestFields(t *testing.T) {
	ds := newDepServer(t)
	project := t.TempDir()

	ds.publish(t, "b", "1.0.0")
	aURL := ds.publish(t, "a", "1.0.0", ds.spec("b"))

	manifest := writeManifest(t, project, "")
	a := installRoot(t, "a", aURL, project)

	rec := newRecorder()
	// Declined: the request is captured before anything is acted on, which is the point.
	rec.answer[strings.ToLower(ds.spec("b"))] = false
	if _, err := InstallDepsFor(context.Background(), manifest, a, project, rec.confirm, func(string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	if len(rec.seen) != 1 {
		t.Fatalf("confirmer saw %d requests, want 1", len(rec.seen))
	}
	req := rec.seen[0]
	if req.Action != DepAdd {
		t.Errorf("Action = %v, want DepAdd", req.Action)
	}
	if req.DeclaredBy != "a" {
		t.Errorf("DeclaredBy = %q, want a", req.DeclaredBy)
	}
	if req.EntryName != "b" {
		t.Errorf("EntryName = %q, want b", req.EntryName)
	}
	// Tagless dep: recorded repo-only, so the url shown is the repo it clones from.
	if req.AssetURL == "" {
		t.Error("AssetURL is empty; the prompt has no url to show")
	}

}

// TestInstallAllDepsDeclineIsRemembered pins the cross-round declined set: importDeps
// recomputes the missing set from scratch every round, so without it a declined
// dependency would be re-offered on each of the remaining rounds.
func TestInstallAllDepsDeclineIsRemembered(t *testing.T) {
	ds := newDepServer(t)
	project := t.TempDir()

	ds.publish(t, "b", "1.0.0")
	ds.publish(t, "c", "1.0.0")
	aURL := ds.publish(t, "a", "1.0.0", ds.spec("b"), ds.spec("c"))

	manifest := writeManifest(t, project, entryBlock("a", aURL))

	rec := newRecorder()
	rec.answer[strings.ToLower(ds.spec("c"))] = false

	if _, err := InstallAllDeps(context.Background(), manifest, project, rec.confirm, func(string, ...any) {}); err != nil {
		t.Fatal(err)
	}

	// Accepting b adds an entry, which makes the loop run a second round — exactly the
	// condition under which a forgotten decline would be re-offered.
	if n := rec.count(strings.ToLower(ds.spec("c"))); n != 1 {
		t.Errorf("c was offered %d times, want exactly 1", n)
	}

	// importDeps' contract is the manifest, not the disk (installing is InstallAll's
	// next round): the accepted dep is recorded, the declined one is not.
	entries, err := Parse(manifest)
	if err != nil {
		t.Fatal(err)
	}
	recorded := map[string]bool{}
	for _, e := range entries {
		recorded[e.Name] = true
	}
	if !recorded["b"] {
		t.Error("b (accepted) should have been recorded in the manifest")
	}
	if recorded["c"] {
		t.Error("c (declined) was recorded in the manifest")
	}
}

// TestInstallAllDepsAbort checks the confirmer's error ends the round loop rather than
// being swallowed into a "nothing left to add" exit.
func TestInstallAllDepsAbort(t *testing.T) {
	ds := newDepServer(t)
	project := t.TempDir()

	ds.publish(t, "b", "1.0.0")
	aURL := ds.publish(t, "a", "1.0.0", ds.spec("b"))
	manifest := writeManifest(t, project, entryBlock("a", aURL))

	rec := newRecorder()
	rec.abort[strings.ToLower(ds.spec("b"))] = true

	if _, err := InstallAllDeps(context.Background(), manifest, project, rec.confirm, func(string, ...any) {}); !errors.Is(err, ErrDepAborted) {
		t.Fatalf("err = %v, want ErrDepAborted", err)
	}
	if _, err := os.Stat(filepath.Join(project, "addons", "b")); err == nil {
		t.Error("b was installed despite the abort")
	}
}

// TestInstallDirEscapeFallsBack checks a package that names an install location outside
// the project root doesn't get it. dir= comes from the *downloaded* config and its
// destination is os.RemoveAll'd, so an unchecked "../.." is an arbitrary recursive
// delete; the install falls back to the normal derivation instead.
func TestInstallDirEscapeFallsBack(t *testing.T) {
	parent := t.TempDir()
	project := filepath.Join(parent, "proj")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	// A sibling of the project root that the escape would target.
	victim := filepath.Join(parent, "victim")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(victim, "keep.txt")
	if err := os.WriteFile(keep, []byte("do not delete"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{"../victim", "../../victim", "/victim"} {
		t.Run(dir, func(t *testing.T) {
			zipPath := filepath.Join(t.TempDir(), "evil.zip")
			cfg := fmt.Sprintf("[plugin]\nname=\"evil\"\nversion=\"1.0.0\"\ndir=%q\n", dir)
			if err := os.WriteFile(zipPath, buildZip(t, map[string]string{
				"addons/evil/plugin.cfg": cfg,
			}), 0o644); err != nil {
				t.Fatal(err)
			}

			res, err := Install(context.Background(), Addon{Name: "evil", URL: zipPath}, project, func(string, ...any) {})
			if err != nil {
				t.Fatalf("install should fall back, not fail: %v", err)
			}
			if res.Path != "addons/evil" {
				t.Errorf("Path = %q, want the derived addons/evil", res.Path)
			}
			if _, err := os.Stat(keep); err != nil {
				t.Errorf("the escape target was clobbered: %v", err)
			}
		})
	}
}

// TestResolveUnder covers the backstop directly: every install path that deletes or
// writes runs its destination through it.
func TestResolveUnder(t *testing.T) {
	base := t.TempDir()
	for _, rel := range []string{"addons/foo", "addons/a/b", "."} {
		if _, err := resolveUnder(base, rel); err != nil {
			t.Errorf("resolveUnder(%q) = %v, want ok", rel, err)
		}
	}
	for _, rel := range []string{"..", "../evil", "addons/../../evil", "addons/../.."} {
		if _, err := resolveUnder(base, rel); err == nil {
			t.Errorf("resolveUnder(%q) succeeded, want a refusal", rel)
		}
	}
	// An absolute path is absorbed by Join rather than escaping, so it stays contained.
	got, err := resolveUnder(base, "/etc/passwd")
	if err != nil {
		t.Fatalf("absolute path: %v", err)
	}
	if !strings.HasPrefix(got, base) {
		t.Errorf("resolveUnder(%q) = %q, want it contained under %q", "/etc/passwd", got, base)
	}
}

// TestUninstallRefusesEscape checks the containment guard covers deletion too — a
// manifest path climbing out of the project must not remove anything.
func TestUninstallRefusesEscape(t *testing.T) {
	parent := t.TempDir()
	project := filepath.Join(parent, "proj")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(parent, "victim")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Uninstall(Addon{Name: "evil", Path: "../victim"}, project); err == nil {
		t.Error("Uninstall accepted a path outside the project root")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("the escape target was deleted: %v", err)
	}
}
