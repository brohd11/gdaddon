package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/brohd11/gdaddon/internal/addon"
)

// writePlugin installs a plugin.cfg at root/addons/<dir>, declaring require when set.
func writePlugin(t *testing.T, root, dir, require string) {
	t.Helper()
	d := filepath.Join(root, "addons", dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[plugin]\nname=\"" + dir + "\"\nversion=\"1.0.0\"\n"
	if require != "" {
		body += "require=" + require + "\n"
	}
	if err := os.WriteFile(filepath.Join(d, "plugin.cfg"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestMissingDepsJSON pins `list --json`'s missing_deps to the TUI's marker: a dep on the
// manifest but not on disk counts too, not only one the manifest lacks or holds too old.
func TestMissingDepsJSON(t *testing.T) {
	root := t.TempDir()
	writePlugin(t, root, "a", `["u/present@v1.0.0", "u/absent@v1.0.0", "u/stale@v2.0.0", "u/notinst@v1.0.0", "u/hushed"]`)
	writePlugin(t, root, "present", "")
	writePlugin(t, root, "stale", "")

	manifest := []addon.Addon{
		{Name: "a", URL: "https://github.com/u/a", Path: "addons/a", SuppressDeps: []string{"github.com/u/hushed"}},
		{Name: "present", URL: "https://github.com/u/present", Path: "addons/present", Tag: "v1.1.0"},
		{Name: "stale", URL: "https://github.com/u/stale", Path: "addons/stale", Tag: "v1.0.0"},
		{Name: "notinst", URL: "https://github.com/u/notinst", Path: "addons/notinst", Tag: "v1.0.0"},
		// u/absent has no entry; u/hushed has none either but is suppressed.
	}
	statuses := make([]addon.Status, 0, len(manifest))
	for _, a := range manifest {
		statuses = append(statuses, addon.InspectOne(a, root))
	}

	got := missingDepsJSON(statuses[0], root, statuses)
	want := []missDepJSON{
		{RepoID: "github.com/u/absent", Tag: "v1.0.0", URL: "https://github.com/u/absent", State: "missing"},
		{RepoID: "github.com/u/stale", Tag: "v2.0.0", URL: "https://github.com/u/stale", State: "outdated", LocalTag: "v1.0.0"},
		{RepoID: "github.com/u/notinst", Tag: "v1.0.0", URL: "https://github.com/u/notinst", State: "not_installed", LocalTag: "v1.0.0"},
	}
	if len(got) != len(want) {
		t.Fatalf("missingDepsJSON = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("missing_deps[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// A plugin that declares nothing still serializes as an empty array, not null.
	if deps := missingDepsJSON(statuses[1], root, statuses); deps == nil || len(deps) != 0 {
		t.Errorf("missingDepsJSON(present) = %#v, want an empty non-nil slice", deps)
	}
}
