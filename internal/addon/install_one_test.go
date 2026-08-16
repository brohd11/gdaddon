package addon

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// depServer serves addon zips over http and hands out urls shaped like a release
// download, so source.RepoID parses them to <host:port>/<owner>/<repo> — the identity
// the dependency resolver matches on. Zips are registered after the server starts,
// because an addon's declared deps have to name the live host:port.
type depServer struct {
	*httptest.Server
	zips map[string][]byte
}

func newDepServer(t *testing.T) *depServer {
	t.Helper()
	ds := &depServer{zips: map[string][]byte{}}
	ds.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, ok := ds.zips[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(ds.Close)
	return ds
}

// spec is how an addon on this server is named in a plugin.cfg `deps` list.
func (ds *depServer) spec(repo string) string {
	return strings.TrimPrefix(ds.URL, "http://") + "/o/" + repo
}

// publish registers an addon zip holding a single plugin folder and returns its url.
// deps, when non-empty, becomes the addon's declared dependency list.
func (ds *depServer) publish(t *testing.T, repo, version string, deps ...string) string {
	t.Helper()
	cfg := fmt.Sprintf("[plugin]\nname=%q\nversion=%q\n", repo, version)
	if len(deps) > 0 {
		// Built by hand rather than with %q: the verb would escape the separating
		// quotes of a multi-dep list into one malformed item.
		cfg += `deps=["` + strings.Join(deps, `","`) + "\"]\n"
	}
	path := "/o/" + repo + "/releases/download/v" + version + "/" + repo + ".zip"
	ds.zips[path] = buildZip(t, map[string]string{
		"addons/" + repo + "/plugin.cfg": cfg,
	})
	return ds.URL + path
}

func writeManifest(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "addon_manifest.yml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func entryBlock(name, url string) string {
	return fmt.Sprintf("%s:\n    url: %s\n\n", name, url)
}

// TestInstallDepsForChain covers the property that distinguishes InstallDepsFor from
// InstallAllDeps: it installs the closure reachable from one addon (a → b → c) and
// leaves every unrelated manifest entry alone.
func TestInstallDepsForChain(t *testing.T) {
	ds := newDepServer(t)
	project := t.TempDir()

	cURL := ds.publish(t, "c", "1.0.0")
	bURL := ds.publish(t, "b", "1.0.0", ds.spec("c"))
	aURL := ds.publish(t, "a", "1.0.0", ds.spec("b"))
	// An entry nothing depends on, pointing at a path the server 404s: if the walk
	// touched it, the install would fail loudly instead of leaving it missing.
	unrelatedURL := ds.URL + "/o/unrelated/releases/download/v1/unrelated.zip"

	manifest := writeManifest(t, project,
		entryBlock("b", bURL)+entryBlock("c", cURL)+entryBlock("unrelated", unrelatedURL))

	a := Addon{Name: "a", URL: aURL}
	res, err := Install(context.Background(), a, project, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	a.Path = res.Path

	outcomes, err := InstallDepsFor(context.Background(), manifest, a, project, nil, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}

	if len(outcomes) != 2 {
		t.Fatalf("installed %d deps, want 2 (b and c): %+v", len(outcomes), outcomes)
	}
	for _, name := range []string{"b", "c"} {
		if _, err := os.Stat(filepath.Join(project, "addons", name, "plugin.cfg")); err != nil {
			t.Errorf("%s should be installed: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(project, "addons", "unrelated")); err == nil {
		t.Error("the unrelated entry should not have been installed")
	}

	// The unrelated entry's manifest block is untouched — still url-only, no pinned path.
	entries, err := Parse(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name == "unrelated" && e.Path != "" {
			t.Errorf("unrelated entry was modified: path = %q", e.Path)
		}
	}
}

// TestInstallDepsForCycle checks the walk terminates when the graph loops back on
// itself (a → b → a) rather than reinstalling forever.
func TestInstallDepsForCycle(t *testing.T) {
	ds := newDepServer(t)
	project := t.TempDir()

	aURL := ds.publish(t, "a", "1.0.0", ds.spec("b"))
	bURL := ds.publish(t, "b", "1.0.0", ds.spec("a"))

	manifest := writeManifest(t, project, entryBlock("b", bURL))

	a := Addon{Name: "a", URL: aURL}
	res, err := Install(context.Background(), a, project, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	a.Path = res.Path

	outcomes, err := InstallDepsFor(context.Background(), manifest, a, project, nil, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || outcomes[0].Name != "b" {
		t.Fatalf("installed %+v, want just b", outcomes)
	}
}

// TestInstallDepsForSuppressed checks a dep listed in the declaring entry's
// suppress_deps is skipped, matching MissingDeps/DepStatuses.
func TestInstallDepsForSuppressed(t *testing.T) {
	ds := newDepServer(t)
	project := t.TempDir()

	bURL := ds.publish(t, "b", "1.0.0")
	aURL := ds.publish(t, "a", "1.0.0", ds.spec("b"))

	manifest := writeManifest(t, project, entryBlock("b", bURL))

	a := Addon{Name: "a", URL: aURL}
	res, err := Install(context.Background(), a, project, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	a.Path = res.Path
	a.SuppressDeps = []string{strings.ToLower(ds.spec("b"))}

	outcomes, err := InstallDepsFor(context.Background(), manifest, a, project, nil, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 0 {
		t.Fatalf("installed %+v, want nothing (b is suppressed)", outcomes)
	}
}

// TestInstallOnePinsEntry covers the single-entry orchestration: a fresh repo is added
// to the manifest, installed, and pinned to the path/version the installer resolved.
func TestInstallOnePinsEntry(t *testing.T) {
	ds := newDepServer(t)
	project := t.TempDir()

	bURL := ds.publish(t, "b", "1.0.0")
	aURL := ds.publish(t, "a", "2.1.0", ds.spec("b"))
	manifest := writeManifest(t, project, entryBlock("b", bURL))

	res, err := InstallOne(context.Background(), InstallOneOpts{
		ManifestPath: manifest,
		ProjectRoot:  project,
		Entry:        Addon{Name: "a", URL: aURL, Tag: "v2.1.0"},
		Deps:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "addons/a" || res.Version != "2.1.0" {
		t.Errorf("got path=%q version=%q, want addons/a / 2.1.0", res.Path, res.Version)
	}
	if len(res.Deps) != 1 || res.Deps[0].Name != "b" {
		t.Errorf("Deps = %+v, want just b", res.Deps)
	}

	entries, err := Parse(manifest)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Addon{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if got := byName["a"]; got.Path != "addons/a" || got.Version != "2.1.0" || got.Tag != "v2.1.0" {
		t.Errorf("entry a = %+v, want path/version/tag pinned", got)
	}
	if got := byName["b"]; got.Path != "addons/b" {
		t.Errorf("entry b = %+v, want its resolved path pinned", got)
	}

	// Re-installing the same repo re-pins the existing entry rather than adding a
	// second one for it.
	if _, err := InstallOne(context.Background(), InstallOneOpts{
		ManifestPath: manifest,
		ProjectRoot:  project,
		Entry:        Addon{Name: "a", URL: aURL, Tag: "v2.1.0"},
	}); err != nil {
		t.Fatal(err)
	}
	again, err := Parse(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != len(entries) {
		t.Errorf("re-install changed the entry count: %d → %d", len(entries), len(again))
	}
}
