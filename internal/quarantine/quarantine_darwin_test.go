package quarantine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// quarantined writes a file and tags it with com.apple.quarantine.
func quarantined(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(path, Attr, []byte("0081;00000000;test;"), 0); err != nil {
		t.Fatal(err)
	}
}

func hasAttr(t *testing.T, path string) bool {
	t.Helper()
	_, err := unix.Getxattr(path, Attr, nil)
	return err == nil
}

// TestClearSkipsHiddenDirs is the regression: xattr -dr descended into an addon's
// .git, whose mode-0444 objects answered every removal with EACCES. The walk must
// prune them instead — clearing the real binary and reporting no denials.
func TestClearSkipsHiddenDirs(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "syntax_plus", "bin", "plugin.dylib")
	obj := filepath.Join(root, "syntax_plus", ".git", "objects", "24", "c2a606")
	quarantined(t, bin)
	quarantined(t, obj)

	// Reproduce git's read-only object store: the file and its directory both
	// refuse writes, which is what made xattr fail.
	objDir := filepath.Dir(obj)
	if err := os.Chmod(obj, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(objDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(objDir, 0o755) }) // let TempDir clean up

	res, err := Clear(context.Background(), root)
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if res.Cleared != 1 {
		t.Errorf("Cleared = %d, want 1 (the dylib)", res.Cleared)
	}
	if res.Denied != 0 || len(res.Errs) != 0 {
		t.Errorf("Denied = %d, Errs = %v, want none", res.Denied, res.Errs)
	}
	if hasAttr(t, bin) {
		t.Error("dylib still quarantined")
	}
	if !hasAttr(t, obj) {
		t.Error("git object was touched; hidden dirs should be pruned, not walked")
	}
}

// TestClearMissingRoot: an unreadable root is the one fatal case.
func TestClearMissingRoot(t *testing.T) {
	if _, err := Clear(context.Background(), filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("want error for a missing root")
	}
}

// TestClearCancelled: esc aborts the walk (TaskScreen's contract).
func TestClearCancelled(t *testing.T) {
	root := t.TempDir()
	quarantined(t, filepath.Join(root, "a", "plugin.dylib"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Clear(ctx, root); err != context.Canceled {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
