package sysopen

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"kitty", []string{"kitty"}},
		{"kitty --directory {dir}", []string{"kitty", "--directory", "{dir}"}},
		{"  wezterm   start  --cwd {dir} ", []string{"wezterm", "start", "--cwd", "{dir}"}},
		{`"/opt/my terminal/bin/term" -d {dir}`, []string{"/opt/my terminal/bin/term", "-d", "{dir}"}},
		{`term --title 'my project'`, []string{"term", "--title", "my project"}},
		{`term --title=""`, []string{"term", "--title="}},
	}
	for _, tc := range tests {
		if got := splitCommand(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitCommand(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}

// TestConfiguredTerminal checks that the config.yml `terminal` key wins over probing
// and that {dir} is substituted in every argument.
func TestConfiguredTerminal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".gdaddon", "config"), 0o755); err != nil {
		t.Fatal(err)
	}

	// No terminal key → no configured command (falls through to probing).
	writeConfig(t, home, "archive_dir: ~/.gdaddon/archive\n")
	if cmd := configuredTerminal("/tmp/addons/mine"); cmd != nil {
		t.Fatalf("expected nil for an unset terminal key, got %v", cmd.Args)
	}

	writeConfig(t, home, "terminal: \"kitty --directory {dir} --title {dir}\"\n")
	cmd := configuredTerminal("/tmp/addons/mine")
	if cmd == nil {
		t.Fatal("expected a command from the configured terminal")
	}
	want := []string{"kitty", "--directory", "/tmp/addons/mine", "--title", "/tmp/addons/mine"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("cmd.Args = %#v, want %#v", cmd.Args, want)
	}
}

// TestProbeTerminal checks the probe picks an emulator off PATH and fills its own
// directory option, using a stub binary so no real terminal is involved.
func TestProbeTerminal(t *testing.T) {
	bin := t.TempDir()
	stub := filepath.Join(bin, "konsole")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	cmd := probeTerminal("/tmp/addons/mine")
	if cmd == nil {
		t.Fatal("expected the stub konsole to be found")
	}
	want := []string{"konsole", "--workdir", "/tmp/addons/mine"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("cmd.Args = %#v, want %#v", cmd.Args, want)
	}
	if cmd.Path != stub {
		t.Fatalf("cmd.Path = %q, want the stub at %q", cmd.Path, stub)
	}

	t.Setenv("PATH", t.TempDir()) // empty dir: nothing to find
	if cmd := probeTerminal("/tmp/addons/mine"); cmd != nil {
		t.Fatalf("expected nil with no emulator on PATH, got %v", cmd.Args)
	}
}

// TestTerminalCmdWorkingDir is the regression test for the reported bug: a terminal
// that ignores the working-directory option must still come up in the target
// directory. A stub emulator records the cwd it was actually launched in.
func TestTerminalCmdWorkingDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".gdaddon", "config"), 0o755); err != nil {
		t.Fatal(err)
	}

	// A stub standing in for an emulator that drops --working-directory (what
	// x-terminal-emulator's gnome-terminal wrapper does): it ignores its arguments and
	// just records where it was started.
	out := filepath.Join(home, "cwd.txt")
	stub := filepath.Join(home, "fake-terminal")
	script := "#!/bin/sh\npwd > " + out + "\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, home, "terminal: \""+stub+" --working-directory={dir}\"\n")

	target := filepath.Join(home, "addons", "my_addon")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := terminalCmd(target)
	if cmd == nil {
		t.Fatal("terminalCmd returned nil for a configured terminal")
	}
	if cmd.Dir != target {
		t.Fatalf("cmd.Dir = %q, want %q", cmd.Dir, target)
	}
	if err := cmd.Run(); err != nil {
		t.Fatalf("stub terminal: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	// macOS resolves /var → /private/var, so compare the resolved forms.
	wantDir, _ := filepath.EvalSymlinks(target)
	gotDir, _ := filepath.EvalSymlinks(strings.TrimSpace(string(got)))
	if gotDir != wantDir {
		t.Fatalf("terminal launched in %q, want %q", gotDir, wantDir)
	}
}

func writeConfig(t *testing.T, home, body string) {
	t.Helper()
	path := filepath.Join(home, ".gdaddon", "config", "config.yml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
