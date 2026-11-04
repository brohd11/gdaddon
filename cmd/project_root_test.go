package cmd

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brohd11/gdaddon/internal/addon"
	"github.com/brohd11/gdaddon/internal/config"
)

func projectArchive(t *testing.T, name, deps string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, err := z.Create(name + "/plugin.cfg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(w, "[plugin]\nname=\"%s\"\nversion=\"1.0.0\"\n%s", name, deps); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCommandsUseManifestProjectRoot(t *testing.T) {
	for _, mode := range []string{"one", "all", "all with deps", "update"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			t.Setenv("GITHUB_TOKEN", "test-token")
			configDir, err := config.ConfigDir()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(configDir, "config.yml"), []byte("gitignore_installs: true\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			base := t.TempDir()
			project := filepath.Join(base, "godot")
			if err := os.MkdirAll(filepath.Join(base, "rust"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(project, 0o755); err != nil {
				t.Fatal(err)
			}
			manifest := filepath.Join(base, "addon_manifest.yml")
			content := "project_root: godot\n"
			if mode != "one" {
				content += "demo:\n  url: https://github.com/u/demo/archive/refs/tags/v1.0.0.zip\n  path: addons/demo\n  version: '1.0.0'\n  tag: v0.9.0\n"
			}
			if mode == "update" {
				content = strings.Replace(content, "/v1.0.0.zip", "/v0.9.0.zip", 1)
			}
			if err := os.WriteFile(manifest, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if mode == "update" {
				writePlugin(t, project, "demo", "")
			}
			deps := ""
			if mode == "all with deps" {
				deps = "require=[\"u/dep@v1.0.0\"]\n"
			}
			archive, depArchive := projectArchive(t, "demo", deps), projectArchive(t, "dep", "")
			original := http.DefaultClient.Transport
			t.Cleanup(func() { http.DefaultClient.Transport = original })
			http.DefaultClient.Transport = tagTransport(func(r *http.Request) (*http.Response, error) {
				var body []byte
				switch {
				case strings.HasSuffix(r.URL.Path, "/releases"):
					body = []byte(`[{"tag_name":"v1.0.0"}]`)
				case strings.HasSuffix(r.URL.Path, "/tags"):
					body = []byte(`[{"name":"v1.0.0"}]`)
				case strings.HasPrefix(r.URL.Path, "/u/demo/") && strings.HasSuffix(r.URL.Path, ".zip"):
					body = archive
				case strings.HasPrefix(r.URL.Path, "/u/dep/") && strings.HasSuffix(r.URL.Path, ".zip"):
					body = depArchive
				default:
					return nil, fmt.Errorf("unexpected request %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}, nil
			})
			addonInstallRoot = base
			addonInstallAll = mode != "one"
			addonInstallNoDeps = mode != "all with deps"
			addonInstallTrustDeps = mode == "all with deps"
			t.Cleanup(func() {
				addonInstallRoot, addonInstallName, addonInstallAsset = "", "", ""
				addonInstallAll, addonInstallNoDeps, addonInstallTrustDeps = false, false, false
			})
			switch mode {
			case "one":
				err = runAddonInstall(rootCmd, []string{"u/demo@v1.0.0"})
			case "update":
				err = runUpdateAddons(rootCmd, []string{base})
			default:
				err = runAddonInstall(rootCmd, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			_, gotRoot, statuses, err := inspectManifest(base)
			wantCount := 1
			if mode == "all with deps" {
				wantCount = 2
			}
			if err != nil || gotRoot != project || len(statuses) != wantCount {
				t.Fatalf("inspection = %s, %+v, %v", gotRoot, statuses, err)
			}
			for _, s := range statuses {
				if !s.Present() || !strings.HasPrefix(s.FullPath, project+string(filepath.Separator)) || s.Addon.Version != "1.0.0" {
					t.Fatalf("wrong installed state or path: %+v", s)
				}
				if mode == "update" && s.Addon.Tag != "v1.0.0" {
					t.Fatalf("update did not pin the new release: %+v", s)
				}
			}
			for _, misplaced := range []string{"addons", ".godot", ".gitignore"} {
				if _, err := os.Stat(filepath.Join(base, misplaced)); !os.IsNotExist(err) {
					t.Fatalf("command wrote outside the project root: %s, %v", misplaced, err)
				}
			}
			ignore, err := os.ReadFile(filepath.Join(project, ".gitignore"))
			if err != nil || !strings.Contains(string(ignore), "/addons/demo/") {
				t.Fatalf("missing project-relative ignore: %s, %v", ignore, err)
			}
			if err := runList(base, true, false); err != nil {
				t.Fatal(err)
			}
			// An explicit update check must also keep its timestamp under godot/.
			if err := runList(base, true, true); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(project, ".godot", "addons", "gdaddon", "last_check")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestListFromToolsManifest(t *testing.T) {
	base := t.TempDir()
	manifest := filepath.Join(base, "tools", "addon_manifest.yml")
	if err := addon.CreateProjectManifest(manifest, base); err != nil {
		t.Fatal(err)
	}
	if err := addon.AddEntry(manifest, "demo", "https://github.com/u/demo", "addons/demo"); err != nil {
		t.Fatal(err)
	}
	writePlugin(t, base, "demo", "")
	_, root, statuses, err := inspectManifest(base)
	if err != nil || root != base || len(statuses) != 1 || !statuses[0].Present() {
		t.Fatalf("tools manifest inspection = %s, %+v, %v", root, statuses, err)
	}
}
