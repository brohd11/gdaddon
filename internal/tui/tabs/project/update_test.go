package project

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brohd11/bubblestack/core"
	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/tui/appctx"

	tea "charm.land/bubbletea/v2"
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

// TestPinChangeRechecksOnlyThatEntry: with the daily check already spent, re-pinning one
// entry re-checks just that entry, and a ProjectDirty that changes no pin stays offline.
func TestPinChangeRechecksOnlyThatEntry(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("GITHUB_TOKEN", "test-token")
	root := t.TempDir()
	manifest := filepath.Join(root, "addon_manifest.yml")
	asset := func(repo, tag string) string {
		return "https://github.com/u/" + repo + "/releases/download/" + tag + "/addon.zip"
	}
	for _, name := range []string{"alpha", "beta"} {
		plugin := filepath.Join(root, "addons", name)
		if err := os.MkdirAll(plugin, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(plugin, "plugin.cfg"), []byte("[plugin]\nversion=\"1.0.0\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := addon.AddEntry(manifest, name, asset(name, "v1.0.0"), "addons/"+name); err != nil {
			t.Fatal(err)
		}
		if err := addon.UpdateEntry(manifest, name, "", "", "1.0.0", "v1.0.0"); err != nil {
			t.Fatal(err)
		}
	}
	stamp := filepath.Join(root, ".godot", "addons", "gdaddon", "last_check")
	old := strconv.FormatInt(time.Now().Unix()-10, 10)
	if err := os.MkdirAll(filepath.Dir(stamp), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stamp, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	calls := map[string]int{}
	transport := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = transport })
	http.DefaultClient.Transport = updateTransport(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		calls[r.URL.Path]++
		mu.Unlock()
		repo := strings.Split(r.URL.Path, "/")[3]
		body := fmt.Sprintf(`[{"tag_name":"v2.0.0","assets":[{"name":"addon.zip","browser_download_url":%q}]},`+
			`{"tag_name":"v1.0.0","assets":[{"name":"addon.zip","browser_download_url":%q}]}]`,
			asset(repo, "v2.0.0"), asset(repo, "v1.0.0"))
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	stale := addon.UpdateInfo{State: addon.UpdateAvailable, LatestTag: "v2.0.0"}
	c := &appctx.Ctx{ProjectRoot: root, ManifestPath: manifest,
		UpdateChecks: map[string]addon.UpdateInfo{"alpha": stale, "beta": stale}}
	sh := core.NewShared(c)
	screen := NewProjectScreen(sh)
	drain(sh, screen, screen.Receive(sh, appctx.ProjectDirty{}))
	if len(calls) != 0 || c.UpdateChecks["alpha"] != stale {
		t.Fatalf("unchanged pins went online: calls=%v checks=%v", calls, c.UpdateChecks)
	}

	if err := addon.UpdateEntry(manifest, "alpha", asset("alpha", "v2.0.0"), "addons/alpha", "2.0.0", "v2.0.0"); err != nil {
		t.Fatal(err)
	}
	drain(sh, screen, screen.Receive(sh, appctx.ProjectDirty{}))
	if got := c.UpdateChecks["alpha"]; got.State != addon.UpdateCurrent || got.LatestTag != "v2.0.0" {
		t.Fatalf("alpha after update = %+v", got)
	}
	if c.UpdateChecks["beta"] != stale {
		t.Fatalf("beta's cached result changed: %+v", c.UpdateChecks["beta"])
	}
	if len(calls) != 1 || calls["/repos/u/alpha/releases"] != 1 {
		t.Fatalf("calls = %v, want one alpha releases request", calls)
	}
	if data, err := os.ReadFile(stamp); err != nil || string(data) != old {
		t.Fatalf("targeted re-check touched the daily stamp: %q, %v", data, err)
	}
}

// TestRecheckDroppedAfterFurtherEdit: a re-check result is stored only while the entry
// still has the pin that was checked, and only for the current project root.
func TestRecheckDroppedAfterFurtherEdit(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "addon_manifest.yml")
	if err := addon.AddEntry(manifest, "alpha", "https://github.com/u/alpha", "addons/alpha"); err != nil {
		t.Fatal(err)
	}
	c := &appctx.Ctx{ProjectRoot: root, ManifestPath: manifest}
	sh := core.NewShared(c)
	screen := NewProjectScreen(sh)
	current := addon.UpdatePinKey(c.ProjectStatuses()[0].Addon)
	result := map[string]addon.UpdateInfo{"alpha": {State: addon.UpdateCurrent}}
	for _, tc := range []struct {
		name string
		msg  updateRecheckReady
		want bool
	}{
		{"older pin", updateRecheckReady{root: root, keys: map[string]string{"alpha": "old"}, checks: result}, false},
		{"other root", updateRecheckReady{root: t.TempDir(), keys: map[string]string{"alpha": current}, checks: result}, false},
		{"current pin", updateRecheckReady{root: root, keys: map[string]string{"alpha": current}, checks: result}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c.UpdateChecks = nil
			drain(sh, screen, core.Async(func() tea.Msg { return core.PropagateAll(tc.msg) }))
			if _, stored := c.UpdateChecks["alpha"]; stored != tc.want {
				t.Fatalf("stored=%v want %v", stored, tc.want)
			}
		})
	}
}
