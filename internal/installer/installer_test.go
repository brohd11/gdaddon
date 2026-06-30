package installer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDest(t *testing.T) {
	for in, want := range map[string]Dest{"system": System, "user": User, "home": Home} {
		got, err := ParseDest(in)
		if err != nil || got != want {
			t.Fatalf("ParseDest(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseDest("nope"); err == nil {
		t.Fatal("ParseDest(nope) should error")
	}
}

func TestDests(t *testing.T) {
	d := Dests()
	if len(d) != 3 {
		t.Fatalf("Dests() len = %d, want 3", len(d))
	}
	if d[0].Dest != System || d[2].Dest != Home {
		t.Fatalf("Dests() order wrong: %+v", d)
	}
	for _, o := range d {
		if o.Label == "" || o.Desc == "" {
			t.Fatalf("Dests() missing label/desc: %+v", o)
		}
	}
}

func TestBinPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	user, err := binPath(User)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".local", "bin", exeName()); user != want {
		t.Fatalf("binPath(User) = %q, want %q", user, want)
	}
	h, err := binPath(Home)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".gdaddon", "bin", exeName()); h != want {
		t.Fatalf("binPath(Home) = %q, want %q", h, want)
	}
}

func TestUninstallFrom(t *testing.T) {
	dir := t.TempDir()
	normal := filepath.Join(dir, "normal")
	self := filepath.Join(dir, "self")
	absent := filepath.Join(dir, "absent")
	for _, p := range []string{normal, self} {
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	removed, skipped, err := uninstallFrom([]string{normal, self, absent}, self)
	if err != nil {
		t.Fatalf("uninstallFrom: %v", err)
	}
	if _, err := os.Stat(normal); !os.IsNotExist(err) {
		t.Fatalf("normal should be gone, stat err = %v", err)
	}

	if selfRemovable {
		// unix: the running binary is removed too.
		if len(removed) != 2 || removed[0] != normal || removed[1] != self {
			t.Fatalf("removed = %v, want [%s %s]", removed, normal, self)
		}
		if len(skipped) != 0 {
			t.Fatalf("skipped = %v, want []", skipped)
		}
		if _, err := os.Stat(self); !os.IsNotExist(err) {
			t.Fatalf("self should be gone, stat err = %v", err)
		}
		return
	}

	// windows: the running binary is left in place and reported as skipped.
	if len(removed) != 1 || removed[0] != normal {
		t.Fatalf("removed = %v, want [%s]", removed, normal)
	}
	if len(skipped) != 1 || skipped[0] != self {
		t.Fatalf("skipped = %v, want [%s]", skipped, self)
	}
	if _, err := os.Stat(self); err != nil {
		t.Fatalf("self (running binary) must survive: %v", err)
	}
}

func TestCopyExe(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("binary-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "sub", "gdaddon")

	if err := copyExe(src, dst); err != nil {
		t.Fatalf("copyExe: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "binary-bytes" {
		t.Fatalf("copied contents = %q, %v", got, err)
	}
	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o100 == 0 {
		t.Fatalf("copied file not executable: %v", fi.Mode())
	}

	// Same-path copy is a no-op (and must not truncate the file).
	if err := copyExe(dst, dst); err != nil {
		t.Fatalf("copyExe same-path: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "binary-bytes" {
		t.Fatalf("same-path copy clobbered the file: %q", got)
	}
}
