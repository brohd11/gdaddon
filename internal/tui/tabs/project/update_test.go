package project

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAutomaticUpdateCheckDailyLimit(t *testing.T) {
	for _, mode := range []string{"recent", "due", "network failure", "stamp failure"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			t.Setenv("GITHUB_TOKEN", "test-token")
			root := t.TempDir()
			plugin := filepath.Join(root, "addons", "alpha")
			if err := os.MkdirAll(plugin, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(plugin, "plugin.cfg"), []byte("[plugin]\nversion=\"1.0.0\"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			manifest := filepath.Join(root, "addon_manifest.yml")
			if err := addon.AddEntry(manifest, "alpha", "https://github.com/u/alpha", "addons/alpha"); err != nil {
				t.Fatal(err)
			}
			if err := addon.UpdateEntry(manifest, "alpha", "", "", "1.0.0", "v1.0.0"); err != nil {
				t.Fatal(err)
			}
			stamp := filepath.Join(root, ".godot", "addons", "gdaddon", "last_check")
			old := strconv.FormatInt(time.Now().Unix()-10, 10)
			if mode == "stamp failure" {
				if err := os.WriteFile(filepath.Join(root, ".godot"), []byte("blocked"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else if mode == "recent" {
				if err := os.MkdirAll(filepath.Dir(stamp), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(stamp, []byte(old), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var calls atomic.Int32
			transport := http.DefaultClient.Transport
			t.Cleanup(func() { http.DefaultClient.Transport = transport })
			http.DefaultClient.Transport = updateTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				if mode != "stamp failure" {
					if _, err := os.Stat(stamp); err != nil {
						t.Error("network started before timestamp was saved")
					}
				}
				if mode == "network failure" {
					return nil, fmt.Errorf("offline")
				}
				body := `[{"tag_name":"v2.0.0"}]`
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			c := &appctx.Ctx{ProjectRoot: root, ManifestPath: manifest,
				UpdateChecks: map[string]addon.UpdateInfo{"alpha": {State: addon.UpdateAvailable, LatestTag: "v1.5.0"}}}
			sh := core.NewShared(c)
			screen := NewProjectScreen(sh)
			statuses := drain(sh, screen, core.Async(checkUpdatesCmd(sh)))
			if mode == "recent" {
				if calls.Load() != 0 || c.UpdateChecks["alpha"].LatestTag != "v1.5.0" {
					t.Fatal("recent stamp must skip network and preserve cached results")
				}
				if data, err := os.ReadFile(stamp); err != nil || string(data) != old {
					t.Fatalf("skipped check renewed stamp: %q, %v", data, err)
				}
			} else if calls.Load() == 0 {
				t.Fatal("due check did not use network")
			} else if mode == "network failure" {
				if c.UpdateChecks["alpha"].State != addon.UpdateUnknown {
					t.Fatal("failed check must report unknown")
				}
			} else if c.UpdateChecks["alpha"].LatestTag != "v2.0.0" {
				t.Fatalf("missing fresh result: %+v", c.UpdateChecks)
			}
			if mode == "stamp failure" {
				if !strings.Contains(strings.Join(statuses, "\n"), "could not save update check timestamp") {
					t.Fatalf("missing persistence diagnostic: %q", statuses)
				}
			} else {
				before := calls.Load()
				drain(sh, screen, screen.Receive(sh, appctx.ProjectDirty{}))
				drain(sh, screen, screen.Receive(sh, appctx.PathRefresh{}))
				if calls.Load() != before {
					t.Fatal("refresh repeated a recent or failed daily attempt")
				}
			}
		})
	}
}

func TestAutomaticUpdateCheckWithoutManifest(t *testing.T) {
	root := t.TempDir()
	sh := core.NewShared(&appctx.Ctx{ProjectRoot: root})
	if msg := checkUpdatesCmd(sh)(); msg != nil {
		t.Fatalf("no manifest should do no work: %v", msg)
	}
	if _, err := os.Stat(filepath.Join(root, ".godot")); !os.IsNotExist(err) {
		t.Fatalf("no manifest should not create a timestamp: %v", err)
	}
}
