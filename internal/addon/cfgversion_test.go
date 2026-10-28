package addon

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetConfigVersionPreservesFile(t *testing.T) {
	for _, name := range pluginCfgNames {
		for _, tc := range []struct {
			name, body, current, next, want string
		}{
			{"quoted", "; keep this\n[plugin]\nname=\"Example\"\n  version = \"0.1.0\"  ; release\nscript=\"plugin.gd\"\n", "0.1.0", "0.1.1", "; keep this\n[plugin]\nname=\"Example\"\n  version = \"0.1.1\"  ; release\nscript=\"plugin.gd\"\n"},
			{"CRLF", "[addon]\r\nversion='v1.2.3'\t# keep\r\nrequire=[]\r\n", "v1.2.3", "v1.2.4", "[addon]\r\nversion='v1.2.4'\t# keep\r\nrequire=[]\r\n"},
			{"no final newline", "[plugin]\nversion=1.0.0", "1.0.0", "1.0.1", "[plugin]\nversion=1.0.1"},
			{"addon override", "[plugin]\nversion=\"1.0.0\"\n[addon]\nversion=\"2.0.0\"\n", "2.0.0", "2.0.1", "[plugin]\nversion=\"1.0.0\"\n[addon]\nversion=\"2.0.1\"\n"},
			{"plugin fallback", "[plugin]\nversion=\"1.0.0\"\n[addon]\nname=\"Other\"\n", "1.0.0", "1.0.1", "[plugin]\nversion=\"1.0.1\"\n[addon]\nname=\"Other\"\n"},
			{"custom", "[addon]\nversion=\"nightly\"\n", "nightly", "release-2026-10-28", "[addon]\nversion=\"release-2026-10-28\"\n"},
			{"missing key", "[addon]\nname=\"Library\"\n", "", "1.0.0", "[addon]\nversion=\"1.0.0\"\nname=\"Library\"\n"},
			{"missing plugin key", "[plugin]\nname=\"Plugin\"\n", "", "1.0.0", "[plugin]\nversion=\"1.0.0\"\nname=\"Plugin\"\n"},
			{"missing section", "; keep\n[other]\nversion=\"untouched\"", "", "1.0.0", "; keep\n[other]\nversion=\"untouched\"\n[plugin]\nversion=\"1.0.0\"\n"},
			{"missing addon key CRLF", "[plugin]\r\nname=\"X\"\r\n[addon]", "", "1.0.0", "[plugin]\r\nname=\"X\"\r\n[addon]\r\nversion=\"1.0.0\"\r\n"},
			{"empty version", "[plugin]\nversion=  ; keep\n", "", "1.0.0", "[plugin]\nversion=  \"1.0.0\"; keep\n"},
			{"empty file", "", "", "1.0.0", "[plugin]\nversion=\"1.0.0\"\n"},
			{"BOM", "\uFEFF[plugin]\nversion=\"1.0.0\"\n", "1.0.0", "1.0.1", "\uFEFF[plugin]\nversion=\"1.0.1\"\n"},
			{"same value", "[plugin]\nversion = \"1.0.0\" ; keep", "1.0.0", "1.0.0", "[plugin]\nversion = \"1.0.0\" ; keep"},
			{"multiline dependencies", "[plugin]\r\nrequire=[\r\n\"u/r@v1.0.0\", ; keep\r\n]\r\nversion=\"1.0.0\"\r\n", "1.0.0", "1.0.1", "[plugin]\r\nrequire=[\r\n\"u/r@v1.0.0\", ; keep\r\n]\r\nversion=\"1.0.1\"\r\n"},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, []byte(tc.body), 0o640); err != nil {
					t.Fatal(err)
				}
				cfg, err := ReadConfigVersion(dir)
				if err != nil || cfg.Path != path || cfg.Version != tc.current {
					t.Fatalf("ReadConfigVersion = %+v, %v; want version %q", cfg, err, tc.current)
				}
				if err := SetConfigVersion(path, cfg.Version, tc.next); err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(path)
				if err != nil || string(got) != tc.want {
					t.Fatalf("saved config = %q, %v; want %q", got, err, tc.want)
				}
				info, err := os.Stat(path)
				if err != nil || info.Mode().Perm() != 0o640 {
					t.Fatalf("permissions changed: %v, %v", info, err)
				}
				cfg, err = ReadConfigVersion(dir)
				if err != nil || cfg.Version != tc.next {
					t.Fatalf("saved version = %q, %v; want %q", cfg.Version, err, tc.next)
				}
			})
		}
	}
}

func TestConfigVersionSelection(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadConfigVersion(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing config: %v", err)
	}
	for i := len(pluginCfgNames) - 1; i >= 0; i-- {
		name := pluginCfgNames[i]
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("[plugin]\nversion=\"1.0.0\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := ReadConfigVersion(dir)
		if err != nil || cfg.Path != path {
			t.Fatalf("selection = %+v, %v; want %s", cfg, err, path)
		}
	}
}

func TestSetConfigVersionRetainsUnrelatedEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.cfg")
	if err := os.WriteFile(path, []byte("[plugin]\nversion=\"1.0.0\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadConfigVersion(dir)
	if err != nil {
		t.Fatal(err)
	}
	body := "[plugin]\nversion=\"1.0.0\"\nname=\"edited while open\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetConfigVersion(cfg.Path, cfg.Version, "1.0.1"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != strings.Replace(body, "1.0.0", "1.0.1", 1) {
		t.Fatalf("unrelated edit lost: %s", got)
	}
}

func TestSetConfigVersionRefusesUnsafeEdits(t *testing.T) {
	for _, tc := range []struct{ name, body, expected, next string }{
		{"stale", "[plugin]\nversion=\"2.0.0\"\n", "1.0.0", "1.0.1"},
		{"blank", "[plugin]\nversion=\"1.0.0\"\n", "1.0.0", "  "},
		{"newline", "[plugin]\nversion=\"1.0.0\"\n", "1.0.0", "1.0.1\nname=bad"},
		{"quote", "[plugin]\nversion=\"1.0.0\"\n", "1.0.0", "1.0.1\""},
		{"comment", "[plugin]\nversion=\"1.0.0\"\n", "1.0.0", "1.0.1;bad"},
		{"duplicate key", "[plugin]\nversion=\"1.0.0\"\nversion=\"1.0.0\"\n", "1.0.0", "1.0.1"},
		{"duplicate section", "[plugin]\nversion=\"1.0.0\"\n[plugin]\nname=\"X\"\n", "1.0.0", "1.0.1"},
		{"multiline version", "[plugin]\nversion=\"\"\"1.0.0\nother\"\"\"\n", "1.0.0\nother", "1.0.1"},
		{"fake section in multiline", "[other]\ndescription=\"\"\"hello\n[plugin]\nversion=\"fake\"\n\"\"\"\n", "", "1.0.1"},
		{"malformed", "[plugin\nversion=\"1.0.0\"\n", "1.0.0", "1.0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "plugin.cfg")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := SetConfigVersion(path, tc.expected, tc.next); err == nil {
				t.Fatal("unsafe update succeeded")
			}
			got, _ := os.ReadFile(path)
			if string(got) != tc.body {
				t.Fatalf("failed save changed file: %q", got)
			}
		})
	}
	path := filepath.Join(t.TempDir(), "plugin.cfg")
	if err := SetConfigVersion(path, "", "1.0.0"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing config must not be created: %v", err)
	}
}

func TestSetConfigVersionSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.cfg")
	link := filepath.Join(dir, "plugin.cfg")
	if err := os.WriteFile(target, []byte("[plugin]\nversion=\"1.0.0\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := SetConfigVersion(link, "1.0.0", "1.0.1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Readlink(link); err != nil {
		t.Fatalf("symlink replaced: %v", err)
	}
	cfg, err := ReadConfigVersion(dir)
	if err != nil || cfg.Version != "1.0.1" {
		t.Fatalf("target not updated: %+v, %v", cfg, err)
	}
}

func TestSetConfigVersionWriteFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.cfg")
	body := "[plugin]\nversion=\"1.0.0\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if err := SetConfigVersion(path, "1.0.0", "1.0.1"); err == nil {
		t.Fatal("save to unwritable directory succeeded")
	}
	got, _ := os.ReadFile(path)
	if string(got) != body {
		t.Fatalf("failed save changed config: %q", got)
	}
}
