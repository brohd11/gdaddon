package addon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const pluginReleasesPath = "/repos/brohd11/gdaddon-editor-plugin/releases"
const pluginDownloadPath = "/brohd11/gdaddon-editor-plugin/releases/download/v1.0.0/gdaddon.zip"
const pluginReleaseJSON = `[{"tag_name":"v1.0.0","assets":[{"name":"gdaddon.zip","browser_download_url":"https://github.com` + pluginDownloadPath + `"}]}]`

func editorPluginFiles() map[string]string {
	return map[string]string{
		"addons/gdaddon/plugin.cfg":   "[plugin]\nname=\"gdaddon\"\nversion=\"1.0.0\"\nscript=\"plugin.gd\"\nrequire=[\"u/should-not-be-downloaded\"]\ndir=\"addons/wrong\"\n",
		"addons/gdaddon/plugin.gd":    "@tool\nextends EditorPlugin\n",
		"addons/gdaddon/src/check.gd": "extends RefCounted\n",
		"README.md":                   "archive sibling must not be installed",
	}
}

func editorPluginProject(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
	writeEditorPluginFile(t, project, "project.godot", "config_version=5\n")
	return project
}

func writeEditorPluginFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertEditorPluginFile(t *testing.T, root, rel, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil || string(got) != want {
		t.Fatalf("%s = %q, %v; want %q", rel, got, err, want)
	}
}

func TestEditorPluginInstallAndRefresh(t *testing.T) {
	for _, ignore := range []bool{false, true} {
		for _, manifest := range []bool{false, true} {
			t.Run(fmt.Sprintf("ignore=%v/manifest=%v", ignore, manifest), func(t *testing.T) {
				bodies, calls := tagInstallHTTP(t)
				withGitignore(t, ignore)
				files := editorPluginFiles()
				bodies[pluginReleasesPath] = []byte(pluginReleaseJSON)
				bodies[pluginDownloadPath] = buildZip(t, files)
				project := editorPluginProject(t)
				const manifestBody = "# untouched\nproject_root: elsewhere\ninvalid: [\n"
				if manifest {
					writeEditorPluginFile(t, project, "addon_manifest.yml", manifestBody)
				}
				res, err := InstallEditorPlugin(context.Background(), project, false, nil)
				if err != nil || res.Path != editorPluginPath || res.Version != "1.0.0" {
					t.Fatalf("install = %+v, %v", res, err)
				}
				assertEditorPluginFile(t, project, "addons/gdaddon/plugin.cfg", files["addons/gdaddon/plugin.cfg"])
				assertEditorPluginFile(t, project, "project.godot", "config_version=5\n")
				if manifest {
					assertEditorPluginFile(t, project, "addon_manifest.yml", manifestBody)
				} else if _, err := os.Stat(filepath.Join(project, "addon_manifest.yml")); !os.IsNotExist(err) {
					t.Fatalf("manifest created: %v", err)
				}
				for _, unwanted := range []string{"addons/wrong", "addons/gdaddon/README.md", "README.md"} {
					if _, err := os.Stat(filepath.Join(project, unwanted)); !os.IsNotExist(err) {
						t.Fatalf("unexpected installed file %s: %v", unwanted, err)
					}
				}
				if entries, err := os.ReadDir(os.Getenv("HOME")); err != nil || len(entries) != 0 {
					t.Fatalf("global addon state changed: %v, %v", entries, err)
				}
				writeEditorPluginFile(t, project, "addons/gdaddon/old/sub.gd", "stale")
				writeEditorPluginFile(t, project, "addons/gdaddon/old/sub.gd.uid", "stale uid")
				writeEditorPluginFile(t, project, "addons/gdaddon/plugin.gd.uid", "uid://keep")
				writeEditorPluginFile(t, project, "addons/gdaddon/src/check.gd.import", "keep import")
				if _, err := InstallEditorPlugin(context.Background(), project, false, nil); err != nil {
					t.Fatal(err)
				}
				assertEditorPluginFile(t, project, "addons/gdaddon/plugin.gd.uid", "uid://keep")
				assertEditorPluginFile(t, project, "addons/gdaddon/src/check.gd.import", "keep import")
				if _, err := os.Stat(filepath.Join(project, "addons/gdaddon/old")); !os.IsNotExist(err) {
					t.Fatalf("obsolete directory remains: %v", err)
				}
				wantIgnore := ""
				if ignore {
					wantIgnore = "/addons/gdaddon/\n"
				}
				if got := readIgnore(t, project); got != wantIgnore {
					t.Fatalf("ignore = %q, want %q", got, wantIgnore)
				}
				if calls[pluginDownloadPath] != 2 || len(calls) != 2 {
					t.Fatalf("unexpected requests (dependencies/source must not be fetched): %v", calls)
				}
			})
		}
	}
}

func TestEditorPluginReplacementGuards(t *testing.T) {
	for _, tc := range []struct {
		name, cfg string
		blocked   bool
	}{
		{"upgrade", "[plugin]\nname=gdaddon\nversion=0.9.0", false},
		{"unversioned", "[plugin]\nname=gdaddon", false},
		{"dev", "[plugin]\nname=gdaddon\nversion=dev", false},
		{"downgrade", "[plugin]\nname=gdaddon\nversion=99.0.0", true},
		{"foreign", "[plugin]\nname=other\nversion=0.1.0", true},
		{"malformed", "[plugin", true},
		{"no config", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bodies, _ := tagInstallHTTP(t)
			bodies[pluginReleasesPath] = []byte(pluginReleaseJSON)
			bodies[pluginDownloadPath] = buildZip(t, editorPluginFiles())
			project := editorPluginProject(t)
			if tc.cfg != "" {
				writeEditorPluginFile(t, project, "addons/gdaddon/plugin.cfg", tc.cfg)
			}
			writeEditorPluginFile(t, project, "addons/gdaddon/sentinel", "original")
			_, err := InstallEditorPlugin(context.Background(), project, false, nil)
			if (err != nil) != tc.blocked {
				t.Fatalf("install error = %v, blocked=%v", err, tc.blocked)
			}
			if tc.blocked {
				if !strings.Contains(err.Error(), "--force") {
					t.Fatal(err)
				}
				assertEditorPluginFile(t, project, "addons/gdaddon/sentinel", "original")
				if _, err := InstallEditorPlugin(context.Background(), project, true, nil); err != nil {
					t.Fatalf("forced install: %v", err)
				}
			}
			assertEditorPluginFile(t, project, "addons/gdaddon/plugin.gd", editorPluginFiles()["addons/gdaddon/plugin.gd"])
		})
	}
}

func TestEditorPluginReleaseSelectionAndFailures(t *testing.T) {
	for _, mode := range []string{"stable", "no releases", "prereleases only", "metadata prerelease", "no uploads", "ambiguous", "bad zip", "missing config", "foreign config", "no version", "missing script", "script escape", "multiple plugins", "failed download"} {
		t.Run(mode, func(t *testing.T) {
			bodies, calls := tagInstallHTTP(t)
			files := editorPluginFiles()
			releases := pluginReleaseJSON
			switch mode {
			case "stable":
				releases = strings.TrimSuffix(releases, "]") + `,{"tag_name":"v99.0.0-beta"},{"tag_name":"v88.0.0","prerelease":true},{"tag_name":"v0.9.0"}]`
			case "no releases":
				releases = `[]`
			case "prereleases only":
				releases = strings.ReplaceAll(releases, `"tag_name":"v1.0.0"`, `"tag_name":"v1.0.0-beta"`)
			case "metadata prerelease":
				releases = strings.ReplaceAll(releases, `"tag_name":"v1.0.0"`, `"tag_name":"v1.0.0","prerelease":true`)
			case "no uploads":
				releases = `[{"tag_name":"v1.0.0"}]`
			case "ambiguous":
				releases = strings.ReplaceAll(releases, `"assets":[`, `"assets":[{"name":"another.zip","browser_download_url":"https://github.com/another.zip"},`)
			case "missing config":
				delete(files, "addons/gdaddon/plugin.cfg")
			case "foreign config":
				files["addons/gdaddon/plugin.cfg"] = "[plugin]\nname=other\nversion=1.0.0\nscript=plugin.gd"
			case "no version":
				files["addons/gdaddon/plugin.cfg"] = "[plugin]\nname=gdaddon\nscript=plugin.gd"
			case "missing script":
				delete(files, "addons/gdaddon/plugin.gd")
			case "script escape":
				files["addons/gdaddon/plugin.cfg"] = "[plugin]\nname=gdaddon\nversion=1.0.0\nscript=../../outside.gd"
			case "multiple plugins":
				files["addons/other/plugin.cfg"] = "[plugin]\nname=other"
			}
			bodies[pluginReleasesPath] = []byte(releases)
			bodies[pluginDownloadPath] = buildZip(t, files)
			if mode == "bad zip" {
				bodies[pluginDownloadPath] = []byte("not a zip")
			} else if mode == "failed download" {
				delete(bodies, pluginDownloadPath)
			}
			project := editorPluginProject(t)
			writeEditorPluginFile(t, project, "addons/gdaddon/sentinel", "original")
			_, err := InstallEditorPlugin(context.Background(), project, true, nil)
			if mode == "stable" {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error")
				}
				assertEditorPluginFile(t, project, "addons/gdaddon/sentinel", "original")
				if _, err := os.Stat(filepath.Join(project, "addons/gdaddon/plugin.gd")); !os.IsNotExist(err) {
					t.Fatalf("failed install wrote files: %v", err)
				}
			}
			for request := range calls {
				if request != pluginReleasesPath && request != pluginDownloadPath {
					t.Fatalf("unexpected request: %s", request)
				}
			}
		})
	}
}

func TestEditorPluginRequiresProjectRoot(t *testing.T) {
	_, calls := tagInstallHTTP(t)
	project := editorPluginProject(t)
	sub := filepath.Join(project, "scenes")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{sub, t.TempDir()} {
		if _, err := InstallEditorPlugin(context.Background(), dir, false, nil); err == nil || !strings.Contains(err.Error(), "project.godot") {
			t.Fatalf("missing project: %v", err)
		}
	}
	if len(calls) != 0 {
		t.Fatalf("downloaded before project validation: %v", calls)
	}
}

func TestEditorPluginRejectsSymlinks(t *testing.T) {
	for _, rel := range []string{"addons", "addons/gdaddon", "addons/gdaddon/plugin.gd"} {
		t.Run(rel, func(t *testing.T) {
			bodies, _ := tagInstallHTTP(t)
			bodies[pluginReleasesPath] = []byte(pluginReleaseJSON)
			bodies[pluginDownloadPath] = buildZip(t, editorPluginFiles())
			project, outside := editorPluginProject(t), t.TempDir()
			writeEditorPluginFile(t, outside, "sentinel", "safe")
			link := filepath.Join(project, rel)
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			for _, force := range []bool{false, true} {
				if _, err := InstallEditorPlugin(context.Background(), project, force, nil); err == nil {
					t.Fatal("installed through symlink")
				}
			}
			assertEditorPluginFile(t, outside, "sentinel", "safe")
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 1 {
				t.Fatalf("outside directory changed: %v, %v", entries, err)
			}
		})
	}
}

func TestEditorPluginIgnoreFailureIsNonfatal(t *testing.T) {
	bodies, _ := tagInstallHTTP(t)
	withGitignore(t, true)
	bodies[pluginReleasesPath] = []byte(pluginReleaseJSON)
	bodies[pluginDownloadPath] = buildZip(t, editorPluginFiles())
	project := editorPluginProject(t)
	if err := os.Mkdir(filepath.Join(project, ".gitignore"), 0o755); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	_, err := InstallEditorPlugin(context.Background(), project, false, func(format string, args ...any) { fmt.Fprintf(&output, format, args...) })
	if err != nil || !strings.Contains(output.String(), "could not update .gitignore") {
		t.Fatalf("install = %v, output=%s", err, output.String())
	}
}
