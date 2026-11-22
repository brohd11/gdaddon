package addon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateCache(t *testing.T) {
	root := t.TempDir()
	alpha := Addon{Name: "alpha", URL: "https://github.com/u/alpha", Tag: "v1.0.0"}
	beta := Addon{Name: "beta", URL: "https://github.com/u/beta", Tag: "v1.0.0"}
	if valid, stale := CachedUpdates(root, []Addon{alpha}); len(valid) != 0 || len(stale) != 0 {
		t.Fatalf("missing file: valid=%v stale=%v", valid, stale)
	}
	avail := UpdateInfo{State: UpdateAvailable, LatestTag: "v2.0.0"}
	if err := SaveUpdateCache(root, []Addon{alpha, beta}, map[string]UpdateInfo{"alpha": avail, "beta": {State: UpdateCurrent}}); err != nil {
		t.Fatal(err)
	}
	valid, stale := CachedUpdates(root, []Addon{alpha, beta, {Name: "gamma"}})
	if valid["alpha"] != avail || valid["beta"].State != UpdateCurrent || len(valid) != 2 || len(stale) != 0 {
		t.Fatalf("round trip: valid=%v stale=%v", valid, stale)
	}

	moved := alpha
	moved.Tag = "v2.0.0"
	valid, stale = CachedUpdates(root, []Addon{moved, beta})
	if _, ok := valid["alpha"]; ok || len(stale) != 1 || stale[0].Name != "alpha" {
		t.Fatalf("changed pin: valid=%v stale=%v", valid, stale)
	}

	if err := MergeUpdateCache(root, []Addon{moved}, map[string]UpdateInfo{"alpha": {State: UpdateCurrent, LatestTag: "v2.0.0"}}); err != nil {
		t.Fatal(err)
	}
	valid, stale = CachedUpdates(root, []Addon{moved, beta})
	if valid["alpha"].State != UpdateCurrent || valid["beta"].State != UpdateCurrent || len(stale) != 0 {
		t.Fatalf("merge: valid=%v stale=%v", valid, stale)
	}

	if err := SaveUpdateCache(root, []Addon{beta}, map[string]UpdateInfo{"beta": {State: UpdateLocked}}); err != nil {
		t.Fatal(err)
	}
	if valid, _ := CachedUpdates(root, []Addon{moved, beta}); len(valid) != 1 || valid["beta"].State != UpdateLocked {
		t.Fatalf("save replaces: %v", valid)
	}

	if err := os.WriteFile(updateCachePath(root), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if valid, stale := CachedUpdates(root, []Addon{beta}); len(valid) != 0 || len(stale) != 0 {
		t.Fatalf("corrupt file: valid=%v stale=%v", valid, stale)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(updateCachePath(root)), ".update_cache-*")); len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v", leftovers)
	}
}
