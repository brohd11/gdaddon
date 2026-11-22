package addon

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// updateCacheMu serializes the cache's read-modify-write within this process; like the
// stamp, coordination with other processes is best effort (writes are atomic renames).
var updateCacheMu sync.Mutex

// updateCacheFile holds the last update results beside the daily stamp, so a session that
// skips the daily check still shows them.
type updateCacheFile struct {
	Entries map[string]cachedUpdate `json:"entries"`
}

// cachedUpdate is one entry's result and the pin it was computed for (a sha256 of
// UpdatePinKey), so a result for an entry changed since is never shown.
type cachedUpdate struct {
	Pin       string `json:"pin"`
	State     string `json:"state"`
	LatestTag string `json:"latest_tag,omitempty"`
}

func updateCachePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".godot", "addons", "gdaddon", "update_cache.json")
}

func pinHash(a Addon) string {
	sum := sha256.Sum256([]byte(UpdatePinKey(a)))
	return hex.EncodeToString(sum[:])
}

// parseUpdateState reverses UpdateState.String; anything else is UpdateUnknown.
func parseUpdateState(s string) UpdateState {
	for _, st := range []UpdateState{UpdateCurrent, UpdateAvailable, UpdateLocked} {
		if st.String() == s {
			return st
		}
	}
	return UpdateUnknown
}

// readUpdateCache returns the cached entries; a missing or unreadable file is empty.
func readUpdateCache(projectRoot string) map[string]cachedUpdate {
	data, err := os.ReadFile(updateCachePath(projectRoot))
	if err != nil {
		return map[string]cachedUpdate{}
	}
	var f updateCacheFile
	if json.Unmarshal(data, &f) != nil || f.Entries == nil {
		return map[string]cachedUpdate{}
	}
	return f.Entries
}

func writeUpdateCache(projectRoot string, entries map[string]cachedUpdate) error {
	path := updateCachePath(projectRoot)
	data, err := json.MarshalIndent(updateCacheFile{Entries: entries}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("could not save update results: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".update_cache-*")
	if err != nil {
		return fmt.Errorf("could not save update results: %w", err)
	}
	_, werr := tmp.Write(append(data, '\n'))
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), path)
	}
	if werr != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("could not save update results: %w", werr)
	}
	return nil
}

// cacheEntries records checks for addons, keyed by name with each addon's pin.
func cacheEntries(into map[string]cachedUpdate, addons []Addon, checks map[string]UpdateInfo) {
	for _, a := range addons {
		info, ok := checks[a.Name]
		if !ok {
			continue
		}
		into[a.Name] = cachedUpdate{Pin: pinHash(a), State: info.State.String(), LatestTag: info.LatestTag}
	}
}

// SaveUpdateCache replaces the cached results with checks for addons (a full check).
func SaveUpdateCache(projectRoot string, addons []Addon, checks map[string]UpdateInfo) error {
	updateCacheMu.Lock()
	defer updateCacheMu.Unlock()
	entries := make(map[string]cachedUpdate, len(checks))
	cacheEntries(entries, addons, checks)
	return writeUpdateCache(projectRoot, entries)
}

// MergeUpdateCache overwrites only addons' cached results with checks (a targeted
// re-check), keeping every other entry.
func MergeUpdateCache(projectRoot string, addons []Addon, checks map[string]UpdateInfo) error {
	updateCacheMu.Lock()
	defer updateCacheMu.Unlock()
	entries := readUpdateCache(projectRoot)
	cacheEntries(entries, addons, checks)
	return writeUpdateCache(projectRoot, entries)
}

// CachedUpdates returns the cached results whose pin still matches each addon, and the
// addons whose cached pin differs (changed since, so their result is withheld for a
// re-check). Addons absent from the cache are in neither.
func CachedUpdates(projectRoot string, addons []Addon) (valid map[string]UpdateInfo, stale []Addon) {
	updateCacheMu.Lock()
	entries := readUpdateCache(projectRoot)
	updateCacheMu.Unlock()
	valid = make(map[string]UpdateInfo, len(entries))
	for _, a := range addons {
		e, ok := entries[a.Name]
		if !ok {
			continue
		}
		if e.Pin != pinHash(a) {
			stale = append(stale, a)
			continue
		}
		valid[a.Name] = UpdateInfo{State: parseUpdateState(e.State), LatestTag: e.LatestTag}
	}
	return valid, stale
}
