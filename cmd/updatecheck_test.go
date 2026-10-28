package cmd

import (
	"encoding/json"
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

	"github.com/brohd11/gdaddon/internal/addon"
)

func captureCheckOutput(t *testing.T, run func() error) (stdout, stderr string, runErr error) {
	t.Helper()
	dir := t.TempDir()
	out, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	diag, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	defer diag.Close()
	previousOut, previousErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = out, diag
	defer func() { os.Stdout, os.Stderr = previousOut, previousErr }()
	runErr = run()
	outBytes, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	diagBytes, err := os.ReadFile(diag.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(outBytes), string(diagBytes), runErr
}

func TestExplicitProjectUpdateChecks(t *testing.T) {
	for _, mode := range []string{"table", "json", "network failure", "stamp failure", "update-addons"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			t.Setenv("GITHUB_TOKEN", "test-token")
			root := t.TempDir()
			writePlugin(t, root, "alpha", "")
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
			} else {
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
			http.DefaultClient.Transport = tagTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				if mode != "stamp failure" {
					if data, err := os.ReadFile(stamp); err != nil || string(data) == old {
						t.Errorf("check started before stamp was renewed: %q, %v", data, err)
					}
				}
				if mode == "network failure" {
					return nil, fmt.Errorf("offline")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`[{"tag_name":"v1.0.0"}]`))}, nil
			})
			out, diag, err := captureCheckOutput(t, func() error {
				if mode == "update-addons" {
					return runUpdateAddons(updateAddonsCmd, []string{root})
				}
				return runList(root, mode != "table", true)
			})
			if err != nil || calls.Load() == 0 {
				t.Fatalf("explicit check was blocked: err=%v, calls=%d", err, calls.Load())
			}
			if mode != "table" && mode != "update-addons" {
				var entries []listEntryJSON
				if err := json.Unmarshal([]byte(out), &entries); err != nil || len(entries) != 1 {
					t.Fatalf("invalid JSON stdout: %q, %v", out, err)
				}
				want := "current"
				if mode == "network failure" {
					want = "unknown"
				}
				if entries[0].Update != want {
					t.Fatalf("update=%q, want %q", entries[0].Update, want)
				}
			}
			if mode == "stamp failure" {
				if !strings.Contains(diag, "could not save update check timestamp") {
					t.Fatalf("missing stderr diagnostic: %q", diag)
				}
			} else if begin, err := addon.BeginUpdateCheck(root, time.Now(), false); begin || err != nil {
				t.Fatalf("explicit attempt did not suppress automatic check: begin=%v, err=%v", begin, err)
			}
		})
	}
}

func TestPlainListLeavesTimestampAlone(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "addon_manifest.yml"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stamp := filepath.Join(root, ".godot", "addons", "gdaddon", "last_check")
	for _, existing := range []bool{false, true} {
		if existing {
			if err := os.MkdirAll(filepath.Dir(stamp), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(stamp, []byte("invalid"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		for _, asJSON := range []bool{false, true} {
			if _, _, err := captureCheckOutput(t, func() error { return runList(root, asJSON, false) }); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(stamp)
			if existing && (err != nil || string(data) != "invalid") {
				t.Fatalf("plain list modified stamp: %q, %v", data, err)
			}
			if !existing && !os.IsNotExist(err) {
				t.Fatalf("plain list created stamp: %q, %v", data, err)
			}
		}
	}
}
