package addon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brohd11/gdaddon/internal/archive"
	"github.com/brohd11/gdaddon/internal/source"
)

type tagTransport func(*http.Request) (*http.Response, error)

func (f tagTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Serve source archives with a synthetic wrapper and namespaced version.cfg
// libraries. Unexpected requests fail, including any attempt to clone a repo.
func tagInstallHTTP(t *testing.T) (map[string][]byte, map[string]int) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("GITHUB_TOKEN", "test-token")
	bodies, calls := map[string][]byte{}, map[string]int{}
	orig := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = orig })
	http.DefaultClient.Transport = tagTransport(func(r *http.Request) (*http.Response, error) {
		if err := r.Context().Err(); err != nil {
			return nil, err
		}
		calls[r.URL.Path]++
		body, ok := bodies[r.URL.Path]
		if !ok {
			return nil, fmt.Errorf("unexpected request: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	return bodies, calls
}

func publishSource(t *testing.T, bodies map[string][]byte, repo, requires string) string {
	t.Helper()
	bodies["/repos/u/"+repo+"/releases"] = []byte(`[]`)
	bodies["/repos/u/"+repo+"/tags"] = []byte(`[{"name":"v1.0.0"}]`)
	path := "/u/" + repo + "/archive/refs/tags/v1.0.0.zip"
	bodies[path] = buildZip(t, map[string]string{
		repo + "-1.0.0/addons/libs/" + repo + "/version.cfg": "[plugin]\nversion=\"1.0.0\"\nrequire=" + requires + "\n",
		repo + "-1.0.0/addons/libs/" + repo + "/library.gd":  "extends RefCounted\n",
	})
	return "https://github.com" + path
}

func TestTagOnlyDependencyInstall(t *testing.T) {
	for _, allow := range []bool{true, false} {
		t.Run(fmt.Sprintf("allow=%v", allow), func(t *testing.T) {
			bodies, calls := tagInstallHTTP(t)
			rootURL := publishSource(t, bodies, "root", `["u/child@v1.0.0"]`)
			publishSource(t, bodies, "child", `["u/leaf@v1.0.0"]`)
			publishSource(t, bodies, "leaf", `[]`)
			project := t.TempDir()
			manifest := writeManifest(t, project, "")
			confirmed := 0
			out, err := InstallOne(context.Background(), InstallOneOpts{
				ManifestPath: manifest, ProjectRoot: project, Deps: true,
				Entry: Addon{Name: "root", URL: rootURL, Tag: "v1.0.0", Kind: KindPackage},
				ConfirmDep: func(req DepRequest) (bool, error) {
					confirmed++
					if !strings.Contains(req.AssetURL, "/archive/refs/tags/v1.0.0.zip") {
						t.Errorf("confirmation URL = %s", req.AssetURL)
					}
					entries, err := Parse(manifest)
					if err != nil {
						t.Fatal(err)
					}
					if _, ok := IndexByRepo(entries)[req.Dep.RepoID]; ok {
						t.Error("dependency recorded before confirmation")
					}
					return allow, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			entries, err := Parse(manifest)
			if err != nil {
				t.Fatal(err)
			}
			if !allow {
				if confirmed != 1 || len(out.Deps) != 0 || len(entries) != 1 {
					t.Fatalf("decline: confirmed=%d, deps=%v, entries=%v", confirmed, out.Deps, entries)
				}
				if calls["/u/child/archive/refs/tags/v1.0.0.zip"] != 0 || calls["/repos/u/leaf/releases"] != 0 {
					t.Fatal("declined subtree was fetched")
				}
				return
			}
			if confirmed != 2 || len(out.Deps) != 2 || len(entries) != 3 {
				t.Fatalf("install: confirmed=%d, deps=%v, entries=%v", confirmed, out.Deps, entries)
			}
			for _, entry := range entries {
				if entry.Tag != "v1.0.0" || entry.Version != "1.0.0" || !strings.HasPrefix(entry.Path, "addons/libs/") {
					t.Errorf("incorrect pin/placement: %+v", entry)
				}
				if _, err := os.Stat(filepath.Join(project, entry.Path, "library.gd")); err != nil {
					t.Error(err)
				}
				if _, err := os.Stat(filepath.Join(project, entry.Path, ".git")); !os.IsNotExist(err) {
					t.Errorf("unexpected git checkout at %s", entry.Path)
				}
			}
		})
	}
}

func TestTagOnlyAddAndOfflineArchive(t *testing.T) {
	bodies, calls := tagInstallHTTP(t)
	url := publishSource(t, bodies, "child", `[]`)
	dep, _ := ParseRepoSpec("u/child@v1.0.0")
	manifest := writeManifest(t, t.TempDir(), "")
	_, added, err := AddDepEntry(context.Background(), manifest, dep, true)
	if err != nil || !added {
		t.Fatalf("add = %v, %v", added, err)
	}
	entries, err := Parse(manifest)
	if err != nil || len(entries) != 1 || entries[0].URL != url || entries[0].Tag != dep.Tag || !entries[0].Dependency {
		t.Fatalf("entries=%+v, err=%v", entries, err)
	}
	if calls["/u/child/archive/refs/tags/v1.0.0.zip"] != 0 {
		t.Fatal("add-only downloaded the ZIP")
	}

	rel, err := source.ResolveTag(context.Background(), dep.RepoURL, dep.Tag)
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.Archive(context.Background(), dep.RepoID, rel.Tag, rel.Assets[0]); err != nil {
		t.Fatal(err)
	}
	clear(calls)
	for key := range bodies {
		delete(bodies, key)
	}
	dead, cancel := context.WithCancel(context.Background())
	cancel()
	asset, ok := ResolveDepAsset(dead, dep)
	if !ok || !filepath.IsAbs(asset.URL) {
		t.Fatalf("offline asset = %+v, ok=%v", asset, ok)
	}
	project := t.TempDir()
	res, err := Install(context.Background(), Addon{Name: "child", URL: asset.URL, Tag: dep.Tag}, project, func(string, ...any) {})
	if err != nil || res.Path != "addons/libs/child" {
		t.Fatalf("offline install = %+v, err=%v", res, err)
	}
	if len(calls) != 0 {
		t.Fatal("offline install attempted network")
	}
}

func TestUnpublishedTagsDoNotChangeLatestOrUpdates(t *testing.T) {
	bodies, calls := tagInstallHTTP(t)
	publishSource(t, bodies, "child", `[]`)
	bodies["/repos/u/child/releases"] = []byte(`[{"tag_name":"v1.0.0"}]`)
	bodies["/repos/u/child/tags"] = []byte(`[{"name":"v2.0.0"},{"name":"v1.0.0"}]`)
	rel, err := ResolveVersion(context.Background(), "https://github.com/u/child", "")
	if err != nil || rel.Tag != "v1.0.0" {
		t.Fatalf("latest=%+v, err=%v", rel, err)
	}
	for _, tag := range []string{"v1.0.0", "v2.0.0"} {
		info := CheckUpdate(context.Background(), Addon{URL: "https://github.com/u/child/archive/refs/tags/" + tag + ".zip", Tag: tag})
		if info.State != UpdateCurrent {
			t.Errorf("update for %s = %+v", tag, info)
		}
	}
	if calls["/repos/u/child/tags"] != 0 {
		t.Fatal("latest/update fetched unpublished tags")
	}
}
