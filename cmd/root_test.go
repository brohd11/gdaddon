package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestSubcommandsRegistered pins the CLI surface: one verb per job, and no leftover
// mode flags on the root command (--install/--list/--update-packages were folded into
// install/list/update-addons, and a stale registration surviving an edit is exactly the
// regression this catches).
func TestSubcommandsRegistered(t *testing.T) {
	var got []string
	for _, c := range rootCmd.Commands() {
		got = append(got, c.Name())
	}
	sort.Strings(got)
	// cobra adds `completion`/`help` lazily during Execute, so they aren't here.
	want := []string{"install", "list", "repos", "update", "update-addons"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("subcommands =\n got %v\nwant %v", got, want)
	}

	for _, name := range []string{"install", "list", "update-packages", "check-updates", "json"} {
		if f := rootCmd.Flags().Lookup(name); f != nil {
			t.Errorf("root should carry no --%s flag; it belongs to a subcommand now", name)
		}
	}
}

// TestCheckInstallArgs covers the combinations cobra can't express: --all and a repo
// spec are different kinds of target, and --asset/--name/--clone only describe one addon.
func TestCheckInstallArgs(t *testing.T) {
	cases := []struct {
		name    string
		all     bool
		args    []string
		asset   string
		entry   string
		clone   bool
		wantErr bool
	}{
		{name: "repo spec alone", args: []string{"u/r"}},
		{name: "all alone", all: true},
		{name: "single-addon flags with a repo", args: []string{"u/r"}, asset: "linux", clone: true},
		{name: "no target", wantErr: true},
		{name: "both targets", all: true, args: []string{"u/r"}, wantErr: true},
		{name: "all with --asset", all: true, asset: "linux", wantErr: true},
		{name: "all with --name", all: true, entry: "thing", wantErr: true},
		{name: "all with --clone", all: true, clone: true, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addonInstallAll, addonInstallAsset = tc.all, tc.asset
			addonInstallName, addonInstallClone = tc.entry, tc.clone
			t.Cleanup(func() {
				addonInstallAll, addonInstallAsset = false, ""
				addonInstallName, addonInstallClone = "", false
			})
			err := checkInstallArgs(tc.args)
			if (err != nil) != tc.wantErr {
				t.Errorf("checkInstallArgs(%v) error = %v, wantErr %v", tc.args, err, tc.wantErr)
			}
		})
	}
}

// TestRunListReportsState exercises the read-only list path end to end: discover the
// manifest under the root, Inspect it, and print — no network (--updates off), no
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
	if err := runList(root, false, false); err != nil {
		t.Fatalf("runList: %v", err)
	}
	if err := runList(root, true, false); err != nil {
		t.Fatalf("runList --json: %v", err)
	}
}

func TestDiscoverManifestMissing(t *testing.T) {
	root := t.TempDir()
	if _, err := discoverManifest(root); err == nil {
		t.Error("expected an error when no manifest exists under the root")
	}
}

// TestIsFirstRun covers the onboarding trigger: gdaddon has never run when ~/.gdaddon
// is absent. bootstrap must sample it *before* config.Ensure creates the directory, so
// this is only ever true on the very first launch.
func TestIsFirstRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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

// TestBootstrapRunsForSubcommands is the guard on the PersistentPreRunE hoist: the
// non-interactive modes used to be root flags handled inside runRoot, so they got the
// config bootstrap for free. As subcommands they must still get it.
func TestBootstrapRunsForSubcommands(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	if rootCmd.PersistentPreRunE == nil {
		t.Fatal("rootCmd needs a PersistentPreRunE so subcommands bootstrap ~/.gdaddon")
	}
	for _, c := range rootCmd.Commands() {
		if c.PersistentPreRunE != nil || c.PersistentPreRun != nil {
			t.Errorf("%s defines its own PersistentPreRun*, which shadows root's bootstrap", c.Name())
		}
	}

	if err := bootstrap(listCmd, nil); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".gdaddon", "config")); err != nil {
		t.Errorf("bootstrap should have created ~/.gdaddon/config: %v", err)
	}
}
