package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetFlags clears the package-level flag vars between Execute calls — cobra does
// not reset them to their defaults, so a prior parse would otherwise leak in.
func resetFlags() { installFlag, listFlag, updateFlag = false, false, false }

// TestFlagsMutuallyExclusive verifies the --install/--list/--update group is
// rejected when more than one is given. cobra's MarkFlagsMutuallyExclusive fires
// during Execute before RunE, so runRoot (and the TUI) never launches.
func TestFlagsMutuallyExclusive(t *testing.T) {
	pairs := [][]string{
		{"--install", "--list"},
		{"--install", "--update"},
		{"--list", "--update"},
	}
	for _, p := range pairs {
		t.Run(strings.Join(p, "_"), func(t *testing.T) {
			resetFlags()
			rootCmd.SetArgs(p)
			rootCmd.SetOut(io.Discard)
			rootCmd.SetErr(io.Discard)
			t.Cleanup(func() {
				resetFlags()
				rootCmd.SetArgs(nil)
			})
			if err := rootCmd.Execute(); err == nil {
				t.Errorf("expected a mutual-exclusion error for %v", p)
			}
		})
	}
}

// TestRunListReportsState exercises the read-only --list dispatch end to end:
// discover the manifest under the root, Inspect it, and print — no network, no
// installs (entries are not present on disk).
func TestRunListReportsState(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "addon_manifest.yml")
	const m = `Alpha:
    url: https://github.com/u/Alpha.git
    path: addons/alpha
    version: "1.0.0"
`
	if err := os.WriteFile(manifest, []byte(m), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runList(root); err != nil {
		t.Fatalf("runList: %v", err)
	}
}

func TestDiscoverManifestMissing(t *testing.T) {
	root := t.TempDir()
	if _, err := discoverManifest(root); err == nil {
		t.Error("expected an error when no manifest exists under the root")
	}
}

// TestIsFirstRun covers the onboarding trigger: gdaddon has never run when ~/.gdaddon
// is absent. runRoot must sample it *before* config.Ensure creates the directory, so
// this is only ever true on the very first launch.
func TestIsFirstRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if !isFirstRun() {
		t.Error("no ~/.gdaddon should read as a first run")
	}
	if err := os.MkdirAll(filepath.Join(home, ".gdaddon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if isFirstRun() {
		t.Error("an existing ~/.gdaddon should not read as a first run")
	}
}
