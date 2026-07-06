package addon

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gdaddon/internal/source"
)

// TestAddEntryFullAllFields is the carry-over guarantee behind Global→Project and
// Set→Project import: AddEntryFull must write every field of a fully-specified
// Addon, and Parse must read them all back unchanged — including the kind line.
func TestAddEntryFullAllFields(t *testing.T) {
	for _, kind := range []Kind{KindClone, KindSubmodule} {
		t.Run(string(kind), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "addon_manifest.yml")
			want := Addon{
				Name:    "Acme",
				URL:     "https://github.com/u/Acme.git",
				Path:    "addons/acme",
				Version: "1.2.3",
				Tag:     "v1.2.3",
				Kind:    kind,
			}
			if err := AddEntryFull(path, want); err != nil {
				t.Fatal(err)
			}
			addons, err := Parse(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(addons) != 1 {
				t.Fatalf("got %d entries, want 1", len(addons))
			}
			if got := addons[0]; !reflect.DeepEqual(got, want) {
				t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", got, want)
			}
		})
	}
}

// TestImportSetToProjectRoundTrip mirrors importSetToProject
// (internal/tui/tabs/actions/sets_import.go): parse the set, AddEntryFull each entry
// into the project. It asserts the full-field carry-over (clone kind survives), that
// a set entry pointing at a repo already in the project is deduped by repo id (.git
// vs release .zip collapse), and that pre-existing project entries are untouched.
func TestImportSetToProjectRoundTrip(t *testing.T) {
	dir := t.TempDir()
	projPath := filepath.Join(dir, "addon_manifest.yml")
	// The project already tracks u/Shared, installed from a release .zip.
	const project = `Shared:
    url: https://github.com/u/Shared/releases/download/v1.0.0/shared-1.0.0.zip
    path: addons/shared
    version: "1.0.0"
`
	if err := os.WriteFile(projPath, []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}

	setPath := filepath.Join(dir, "set.yml")
	const set = `Alpha:
    url: https://github.com/u/Alpha.git
    path: addons/alpha
    version: "2.0.0"
    tag: "v2.0.0"
    kind: clone

Beta:
    url: https://github.com/u/Beta/releases/download/v3.1.0/beta-3.1.0.zip
    path: addons/beta
    version: "3.1.0"
    tag: "v3.1.0"

SharedDup:
    url: https://github.com/u/Shared.git
    path: addons/shared2
`
	if err := os.WriteFile(setPath, []byte(set), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := Parse(setPath)
	if err != nil {
		t.Fatal(err)
	}
	added, skipped := 0, 0
	for _, e := range entries {
		if err := AddEntryFull(projPath, e); err != nil {
			skipped++
			continue
		}
		added++
	}
	if added != 2 || skipped != 1 {
		t.Fatalf("added=%d skipped=%d, want 2 added / 1 skipped (SharedDup deduped)", added, skipped)
	}

	got, err := Parse(projPath)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Addon{}
	for _, a := range got {
		byName[a.Name] = a
	}

	alpha := byName["Alpha"]
	if alpha.URL != "https://github.com/u/Alpha.git" || alpha.Path != "addons/alpha" ||
		alpha.Version != "2.0.0" || alpha.Tag != "v2.0.0" || alpha.Kind != KindClone {
		t.Errorf("Alpha not imported with all fields (clone kind expected): %+v", alpha)
	}
	beta := byName["Beta"]
	if beta.Version != "3.1.0" || beta.Tag != "v3.1.0" || beta.Kind != KindPackage {
		t.Errorf("Beta not imported correctly: %+v", beta)
	}
	if _, dup := byName["SharedDup"]; dup {
		t.Errorf("duplicate-repo entry should have been skipped, not added")
	}
	shared := byName["Shared"]
	if shared.Version != "1.0.0" || shared.Path != "addons/shared" ||
		shared.URL != "https://github.com/u/Shared/releases/download/v1.0.0/shared-1.0.0.zip" {
		t.Errorf("pre-existing Shared entry mutated: %+v", shared)
	}
}

// TestUpsertEntryAppend covers the append branch: with no entry for the repo,
// UpsertEntry writes a complete new entry (like AddEntryFull).
func TestUpsertEntryAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "set.yml")
	want := Addon{
		Name:    "New",
		URL:     "https://github.com/u/New.git",
		Path:    "addons/new",
		Version: "1.0.0",
		Tag:     "v1.0.0",
		Kind:    KindClone,
	}
	if err := UpsertEntry(path, want); err != nil {
		t.Fatal(err)
	}
	addons, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(addons) != 1 {
		t.Fatalf("want 1 entry, got %d", len(addons))
	}
	if got := addons[0]; !reflect.DeepEqual(got, want) {
		t.Errorf("append mismatch:\n got  %+v\n want %+v", got, want)
	}
}

// TestUpsertEntryUpdateInPlace covers the update branch (a set's "Add Version",
// tracking an installed plugin): an entry already exists for the repo under a
// different label and via its .git url; re-upserting the SAME repo's release .zip
// url with a new version must re-pin it in place — matched by source.RepoID, not by
// name, with no duplicate key. An empty tag/path arg leaves those lines untouched
// (per the UpsertEntry contract), and a non-package Kind is applied.
func TestUpsertEntryUpdateInPlace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addon_manifest.yml")
	const existing = `Widget:
    url: https://github.com/u/Widget.git
    path: addons/widget
    version: "1.0.0"
    tag: "v1.0.0"
`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	up := Addon{
		Name:    "WidgetZip", // a different label — must match by repo id, not name
		URL:     "https://github.com/u/Widget/releases/download/v2.0.0/widget-2.0.0.zip",
		Version: "2.0.0",
		Kind:    KindClone,
	}
	if err := UpsertEntry(path, up); err != nil {
		t.Fatal(err)
	}

	addons, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(addons) != 1 {
		t.Fatalf("upsert should update in place, not duplicate; got %d entries", len(addons))
	}
	got := addons[0]
	if got.Name != "Widget" {
		t.Errorf("existing key should be kept, got %q", got.Name)
	}
	if got.URL != up.URL {
		t.Errorf("url not updated: %q", got.URL)
	}
	if got.Version != "2.0.0" {
		t.Errorf("version not updated: %q", got.Version)
	}
	if got.Tag != "v1.0.0" {
		t.Errorf("empty tag arg should leave the existing tag untouched, got %q", got.Tag)
	}
	if got.Path != "addons/widget" {
		t.Errorf("empty path arg should leave the existing path untouched, got %q", got.Path)
	}
	if got.Kind != KindClone {
		t.Errorf("kind not applied: %q", got.Kind)
	}
}

// TestExportToGlobalDropsVersionTagKind mirrors exportToGlobal
// (internal/tui/tabs/project/submenu.go): strip the pinned release url down to its
// canonical repo form via source.RepoURL, then AddEntry (url + path only). The
// global list is a library of reusable repos, so version/tag/kind must be dropped.
func TestExportToGlobalDropsVersionTagKind(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, ".gdaddon", "plugins.yml")

	a := Addon{
		Name:    "Gadget",
		URL:     "https://github.com/u/Gadget/releases/download/v1.5.0/gadget-1.5.0.zip",
		Path:    "addons/gadget",
		Version: "1.5.0",
		Tag:     "v1.5.0",
		Kind:    KindClone,
	}
	url := a.URL
	if stripped, err := source.RepoURL(a.URL); err == nil {
		url = stripped
	}
	if err := AddEntry(globalPath, a.Name, url, a.Path); err != nil {
		t.Fatal(err)
	}

	got := string(mustRead(t, globalPath))
	if !strings.Contains(got, "url: https://github.com/u/Gadget\n") {
		t.Errorf("url not canonicalized to repo form; got:\n%s", got)
	}
	if !strings.Contains(got, "path: addons/gadget") {
		t.Errorf("path should be carried into the global entry; got:\n%s", got)
	}
	for _, dropped := range []string{"version:", "tag:", "kind:"} {
		if strings.Contains(got, dropped) {
			t.Errorf("global entry should not contain %q; got:\n%s", dropped, got)
		}
	}
}
