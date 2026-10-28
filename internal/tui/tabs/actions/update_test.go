package actions

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/brohd11/gdaddon/internal/addon"
)

func TestUpdateAllRenewsTimestamp(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "recent", true: "write failure"}[blocked], func(t *testing.T) {
			root := t.TempDir()
			manifest := filepath.Join(root, "addon_manifest.yml")
			if err := os.WriteFile(manifest, []byte("{}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			stamp := filepath.Join(root, ".godot", "addons", "gdaddon", "last_check")
			if blocked {
				if err := os.WriteFile(filepath.Join(root, ".godot"), []byte("blocked"), 0o644); err != nil {
					t.Fatal(err)
				}
			} else if _, err := addon.BeginUpdateCheck(root, time.Now().Add(-time.Hour), true); err != nil {
				t.Fatal(err)
			}
			msg := resolveUpdatePlansCmd(manifest, root)(context.Background())().(updatePlansMsg)
			if msg.err != nil || (msg.stampErr != nil) != blocked {
				t.Fatalf("plan error=%v, stamp error=%v", msg.err, msg.stampErr)
			}
			if !blocked {
				data, err := os.ReadFile(stamp)
				if err != nil {
					t.Fatal(err)
				}
				seconds, err := strconv.ParseInt(string(data), 10, 64)
				if err != nil || time.Since(time.Unix(seconds, 0)) > time.Minute {
					t.Fatalf("Update All did not renew stamp: %q, %v", data, err)
				}
			}
		})
	}
}
