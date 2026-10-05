package addon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// withGitignore forces the gitignore_installs toggle for one test.
func withGitignore(t *testing.T, on bool) {
	t.Helper()
	prev := gitignoreEnabled
	gitignoreEnabled = func() bool { return on }
	t.Cleanup(func() { gitignoreEnabled = prev })
}

func readIgnore(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

func writeIgnore(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIgnoreInstallAppends(t *testing.T) {
	withGitignore(t, true)
	cases := []struct{ name, before, want string }{
		{"missing file", "", "/addons/foo/\n"},
		{"trailing newline", "*.tmp\n", "*.tmp\n/addons/foo/\n"},
		{"no trailing newline", "*.tmp", "*.tmp\n/addons/foo/\n"},
		{"comment does not cover", "# addons/foo\n", "# addons/foo\n/addons/foo/\n"},
		{"negation does not cover", "!addons/foo\n", "!addons/foo\n/addons/foo/\n"},
		{"sibling does not cover", "/addons/foobar/\n", "/addons/foobar/\n/addons/foo/\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.before != "" {
				writeIgnore(t, root, tc.before)
			}
			added, err := IgnoreInstall(root, "addons/foo")
			if err != nil || !added {
				t.Fatalf("IgnoreInstall = %v, %v; want added", added, err)
			}
			if got := readIgnore(t, root); got != tc.want {
				t.Errorf(".gitignore = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIgnoreInstallSkipsCovered(t *testing.T) {
	withGitignore(t, true)
	for _, line := range []string{"addons/foo", "/addons/foo", "addons/foo/", "/addons/foo/", "addons/", "/addons", "  addons/foo  "} {
		t.Run(line, func(t *testing.T) {
			root := t.TempDir()
			writeIgnore(t, root, line+"\n")
			added, err := IgnoreInstall(root, "addons/foo")
			if err != nil || added {
				t.Fatalf("IgnoreInstall = %v, %v; want skipped", added, err)
			}
			if got := readIgnore(t, root); got != line+"\n" {
				t.Errorf(".gitignore changed: %q", got)
			}
		})
	}
}

func TestIgnoreInstallOff(t *testing.T) {
	withGitignore(t, false)
	root := t.TempDir()
	if added, err := IgnoreInstall(root, "addons/foo"); err != nil || added {
		t.Fatalf("IgnoreInstall = %v, %v; want no-op", added, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".gitignore")); !os.IsNotExist(err) {
		t.Errorf(".gitignore written with the toggle off")
	}
}

func TestMoveIgnore(t *testing.T) {
	withGitignore(t, true)
	root := t.TempDir()
	writeIgnore(t, root, "*.tmp\n/addons/foo/\naddons/foo\n")
	if err := MoveIgnore(root, "addons/foo", "addons/vendor/foo"); err != nil {
		t.Fatal(err)
	}
	// The gdaddon line is swapped; the user's own spelling is left alone.
	want := "*.tmp\naddons/foo\n/addons/vendor/foo/\n"
	if got := readIgnore(t, root); got != want {
		t.Errorf(".gitignore = %q, want %q", got, want)
	}

	withGitignore(t, false)
	if err := MoveIgnore(root, "addons/vendor/foo", "addons/bar"); err != nil {
		t.Fatal(err)
	}
	if got := readIgnore(t, root); got != want {
		t.Errorf("MoveIgnore with the toggle off changed .gitignore: %q", got)
	}
}

// Install ignores the final path, and a re-install does not duplicate the line.
func TestInstallAddsGitignore(t *testing.T) {
	withGitignore(t, true)
	data := buildZip(t, map[string]string{
		"MyRepo-1.0.0/addons/at_icons/icon.svg": "<svg/>",
	})
	zipPath := filepath.Join(t.TempDir(), "pkg.zip")
	if err := os.WriteFile(zipPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	a := Addon{Name: "at_icons", URL: zipPath, Tag: "v1.0.0"}
	for range 2 {
		if _, err := Install(context.Background(), a, project, quietReport); err != nil {
			t.Fatal(err)
		}
	}
	if got := readIgnore(t, project); got != "/addons/at_icons/\n" {
		t.Errorf(".gitignore = %q, want one /addons/at_icons/ line", got)
	}
}
