package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
)

func TestMultilineDependenciesMenuAndErrors(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "addons", "example")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "plugin.cfg")
	if err := os.WriteFile(path, []byte("[plugin]\nversion=\"1.0.0\"\nrequire=[\n\"u/first@v1.0.0\",\n\"u/second@v2.0.0\",\n]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "addon_manifest.yml")
	if err := addon.AddEntry(manifest, "example", "https://github.com/u/example", "addons/example"); err != nil {
		t.Fatal(err)
	}
	c := &appctx.Ctx{ProjectRoot: root, ManifestPath: manifest}
	sh := core.NewShared(c)
	statuses := c.ProjectStatuses()
	if len(statuses) != 1 || len(c.DepStatuses["example"]) != 2 {
		t.Fatalf("dependency cache missing: %+v", c.DepStatuses)
	}
	menu := newSubmenuScreen(statuses[0], sh)
	found := false
	for _, item := range menu.List().Items() {
		if row, ok := item.(components.Item); ok && row.Name == "⛓ Dependencies" {
			found = true
		}
	}
	if !found {
		t.Fatal("Dependencies menu missing")
	}
	items := newDepsScreen(statuses[0], sh).List().Items()
	if len(items) != 3 || !strings.Contains(items[1].(components.Item).Name, "github.com/u/first  [missing]") ||
		!strings.Contains(items[2].(components.Item).Name, "github.com/u/second  [missing]") {
		t.Fatalf("unexpected dependency rows: %+v", items)
	}
	if errs := c.DrainLoadErrs(); len(errs) != 0 {
		t.Fatalf("valid config errors: %v", errs)
	}
	if err := os.WriteFile(path, []byte("[plugin]\nrequire=[\n\"u/first\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c.RefreshProject()
	errs := c.DrainLoadErrs()
	if len(errs) != 1 || !strings.Contains(errs[0], "dependencies") || !strings.Contains(errs[0], "plugin.cfg") ||
		!strings.Contains(errs[0], "key require at line 2: unterminated array") {
		t.Fatalf("config failure was not reported: %v", errs)
	}
	if len(c.DepStatuses["example"]) != 0 {
		t.Fatal("failed config retained stale dependency statuses")
	}
}
