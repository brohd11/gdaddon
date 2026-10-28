package addon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Serializes read-and-stamp within this process. The editor uses the same file,
// but coordination between processes remains best effort.
var updateCheckMu sync.Mutex

// BeginUpdateCheck shares the editor's daily attempt stamp. Explicit project-wide
// checks pass force=true to renew it even when recent. A persistence error is
// nonfatal: the caller should report it and still run when begin is true.
func BeginUpdateCheck(projectRoot string, now time.Time, force bool) (begin bool, err error) {
	updateCheckMu.Lock()
	defer updateCheckMu.Unlock()

	path := filepath.Join(projectRoot, ".godot", "addons", "gdaddon", "last_check")
	seconds := now.Unix()
	if !force && !updateCheckDue(path, seconds) {
		return false, nil
	}
	// Stamp before discovery/network work so failed checks consume the interval.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return true, fmt.Errorf("could not save update check timestamp: %w", err)
	}
	if err := os.WriteFile(path, []byte(strconv.FormatInt(seconds, 10)), 0o644); err != nil {
		return true, fmt.Errorf("could not save update check timestamp: %w", err)
	}
	return true, nil
}

func updateCheckDue(path string, now int64) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	last, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	return err != nil || last <= 0 || last > now || now-last >= 24*60*60
}
