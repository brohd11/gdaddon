package cmd

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginInstallCommand(t *testing.T) {
	found, remaining, err := rootCmd.Find([]string{"plugin", "install"})
	if err != nil || found != pluginInstallCmd || len(remaining) != 0 {
		t.Fatalf("plugin install registration: %v, %v, %v", found, remaining, err)
	}
	if err := found.ValidateArgs([]string{"some-directory"}); err == nil {
		t.Fatal("plugin install accepted a project-directory argument")
	}
	if found.Flags().Lookup("force") == nil || found.Flags().Lookup("root") != nil {
		t.Fatal("plugin install should accept --force, with no --root option")
	}
	if pluginCmd.PersistentPreRunE != nil || pluginCmd.PersistentPreRun != nil {
		t.Fatal("plugin must inherit the root config bootstrap")
	}

	project := t.TempDir()
	t.Chdir(project)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("GITHUB_TOKEN", "test-token")
	if err := os.WriteFile("project.godot", []byte("config_version=5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	for name, body := range map[string]string{
		"addons/gdaddon/plugin.cfg": "[plugin]\nname=gdaddon\nversion=1.0.0\nscript=plugin.gd",
		"addons/gdaddon/plugin.gd":  "@tool\nextends EditorPlugin\n",
	} {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	original := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = original })
	http.DefaultClient.Transport = tagTransport(func(r *http.Request) (*http.Response, error) {
		var body []byte
		switch r.URL.Path {
		case "/repos/brohd11/gdaddon-editor-plugin/releases":
			body = []byte(`[{"tag_name":"v1.0.0","assets":[{"name":"gdaddon.zip","browser_download_url":"https://github.com/plugin.zip"}]}]`)
		case "/plugin.zip":
			body = archive.Bytes()
		default:
			return nil, fmt.Errorf("unexpected request %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	var output bytes.Buffer
	pluginInstallCmd.SetContext(context.Background())
	pluginInstallCmd.SetOut(&output)
	t.Cleanup(func() {
		pluginInstallCmd.SetContext(nil)
		pluginInstallCmd.SetOut(nil)
		_ = pluginInstallCmd.Flags().Set("force", "false")
	})
	if err := runPluginInstall(pluginInstallCmd, nil); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !strings.Contains(got, "Installed gdaddon editor plugin 1.0.0") || !strings.Contains(got, `Enable "gdaddon" in Project Settings > Plugins`) {
		t.Fatalf("install output: %s", got)
	}
	if _, err := os.Stat("addon_manifest.yml"); !os.IsNotExist(err) {
		t.Fatalf("manifest created: %v", err)
	}
	// The command forwards --force to the installer's downgrade guard.
	if err := os.WriteFile(filepath.Join("addons", "gdaddon", "plugin.cfg"), []byte("[plugin]\nname=gdaddon\nversion=99.0.0"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runPluginInstall(pluginInstallCmd, nil); err == nil {
		t.Fatal("downgrade accepted without --force")
	}
	if err := pluginInstallCmd.Flags().Set("force", "true"); err != nil {
		t.Fatal(err)
	}
	if err := runPluginInstall(pluginInstallCmd, nil); err != nil {
		t.Fatalf("forced command: %v", err)
	}
	// A parent project does not make a subdirectory a valid command target.
	if err := os.Mkdir("scenes", 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir("scenes")
	if err := runPluginInstall(pluginInstallCmd, nil); err == nil || !strings.Contains(err.Error(), "project.godot") {
		t.Fatalf("subdirectory error: %v", err)
	}
}
