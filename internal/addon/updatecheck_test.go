package addon

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBeginUpdateCheck(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	root := t.TempDir()
	path := filepath.Join(root, ".godot", "addons", "gdaddon", "last_check")
	if begin, err := BeginUpdateCheck(root, now, false); !begin || err != nil {
		t.Fatalf("missing stamp: begin=%v, err=%v", begin, err)
	}
	want := strconv.FormatInt(now.Unix(), 10)
	if data, err := os.ReadFile(path); err != nil || string(data) != want {
		t.Fatalf("stamp bytes=%q, err=%v; want %q without newline", data, err, want)
	}
	for _, tc := range []struct {
		name string
		text string
		due  bool
	}{
		{"same time", want, false},
		{"whitespace", " \t" + want + "\r\n", false},
		{"before boundary", strconv.FormatInt(now.Unix()-86_399, 10), false},
		{"at boundary", strconv.FormatInt(now.Unix()-86_400, 10), true},
		{"after boundary", strconv.FormatInt(now.Unix()-86_401, 10), true},
		{"future", strconv.FormatInt(now.Unix()+1, 10), true},
		{"empty", "", true},
		{"text", "invalid", true},
		{"fractional", "1.5", true},
		{"zero", "0", true},
		{"negative", "-10", true},
		{"overflow", "9223372036854775808", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.text), 0o644); err != nil {
				t.Fatal(err)
			}
			if begin, err := BeginUpdateCheck(root, now, false); begin != tc.due || err != nil {
				t.Fatalf("begin=%v, err=%v; want %v", begin, err, tc.due)
			}
			expected := tc.text
			if tc.due {
				expected = want
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != expected {
				t.Fatalf("stamp=%q, err=%v; want %q", data, err, expected)
			}
		})
	}
	if begin, err := BeginUpdateCheck(root, now.Add(time.Second), true); !begin || err != nil {
		t.Fatalf("explicit attempt: begin=%v, err=%v", begin, err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "1800000001" {
		t.Fatalf("explicit attempt did not renew stamp: %q, %v", data, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if begin, err := BeginUpdateCheck(root, now, false); !begin || err != nil {
		t.Fatalf("deleted stamp: begin=%v, err=%v", begin, err)
	}
}

func TestBeginUpdateCheckPersistenceFailure(t *testing.T) {
	for _, blocked := range []string{"parent", "stamp"} {
		t.Run(blocked, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ".godot")
			if blocked == "parent" {
				if err := os.WriteFile(path, []byte("blocked"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else {
				// A directory at last_check makes both reading and writing fail on every OS.
				if err := os.MkdirAll(filepath.Join(path, "addons", "gdaddon", "last_check"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if begin, err := BeginUpdateCheck(root, time.Unix(1_800_000_000, 0), false); !begin || err == nil {
				t.Fatalf("persistence failure must still allow check: begin=%v, err=%v", begin, err)
			}
		})
	}
}

func TestBeginUpdateCheckConcurrent(t *testing.T) {
	root := t.TempDir()
	now := time.Unix(1_800_000_000, 0)
	var attempts atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			begin, err := BeginUpdateCheck(root, now, false)
			if err != nil {
				t.Error(err)
			}
			if begin {
				attempts.Add(1)
			}
		})
	}
	wg.Wait()
	if got := attempts.Load(); got != 1 {
		t.Fatalf("started %d concurrent attempts, want 1", got)
	}
	if begin, err := BeginUpdateCheck(t.TempDir(), now, false); !begin || err != nil {
		t.Fatalf("different project must be independent: begin=%v, err=%v", begin, err)
	}
}
