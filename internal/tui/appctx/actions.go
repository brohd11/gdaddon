package appctx

import (
	tea "charm.land/bubbletea/v2"
	"github.com/brohd11/bubblestack/components"
	"github.com/brohd11/bubblestack/core"
	bsupdate "github.com/brohd11/bubblestack/selfupdate"
	"github.com/brohd11/gdaddon/internal/addon"
)

// LockToggle flips the lock on name in the manifest at path, returning the new state and
// "locked"/"unlocked". Errors are returned as-is.
func LockToggle(path, name string, cur bool) (newLock bool, verb string, err error) {
	newLock = !cur
	if e := addon.SetLock(path, name, newLock); e != nil {
		return false, "", e
	}
	verb = "locked"
	if !newLock {
		verb = "unlocked"
	}
	return newLock, verb, nil
}

// LockItem is the Lock/Unlock row for an entry whose lock state is locked.
func LockItem(locked bool, pick func(*core.Shared) core.Action) components.Item {
	if locked {
		return components.Item{Name: "🔓 Unlock", Desc: "resume update checks", Pick: pick}
	}
	return components.Item{Name: "🔒 Lock", Desc: "pin this version — stop update alerts", Pick: pick}
}

// SelfUpdateHooks points the shared self-update flow at gdaddon's release repo (also
// named in cmd/update.go). Used by the startup check and Actions ▸ Update gdaddon.
func SelfUpdateHooks(version string) components.SelfUpdateHooks {
	return bsupdate.Hooks("gdaddon", "brohd11/gdaddon", version)
}

// SelfUpdateCheckCmd is the startup check; it reports only when an update is available.
func SelfUpdateCheckCmd(sh *core.Shared) tea.Cmd {
	return components.SelfUpdateCheckCmd(SelfUpdateHooks(Of(sh).Version))
}
