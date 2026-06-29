//go:build windows

package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gdaddon/internal/config"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func optionLabel(d Dest) string {
	switch d {
	case System:
		return `System  (%ProgramFiles%\gdaddon)`
	case User:
		return `User  (%LOCALAPPDATA%\Programs\gdaddon)`
	case Home:
		return `gdaddon home  (%USERPROFILE%\.gdaddon\bin)`
	}
	return ""
}

func optionDesc(d Dest) string {
	switch d {
	case System:
		return "Machine PATH. Requires Administrator."
	case User:
		return "User PATH. No elevation."
	case Home:
		return "Not on PATH. Launched by the Godot plugin."
	}
	return ""
}

// exeName is the binary's filename on this platform.
func exeName() string { return "gdaddon.exe" }

// removeAt deletes path. (No sudo equivalent; system removal relies on the admin
// shell the user already needed to install there.)
func removeAt(path string) error { return os.Remove(path) }

// destDir maps a Dest to its concrete directory.
func destDir(d Dest) (string, error) {
	switch d {
	case System:
		return filepath.Join(os.Getenv("ProgramFiles"), "gdaddon"), nil
	case User:
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "gdaddon"), nil
	case Home:
		base, err := config.Dir()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, config.BinSubdir), nil
	}
	return "", fmt.Errorf("unknown destination %v", d)
}

func doInstall(d Dest, src string) (Result, error) {
	dir, err := destDir(d)
	if err != nil {
		return Result{}, err
	}
	dst := filepath.Join(dir, exeName())
	const pathNote = "Open a new terminal for the PATH change to take effect."

	switch d {
	case System:
		if !isAdmin() {
			return Result{}, fmt.Errorf("system install needs an elevated shell — re-run PowerShell as Administrator")
		}
		if err := copyExe(src, dst); err != nil {
			return Result{}, err
		}
		if err := addToPath(dir, registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`); err != nil {
			return Result{}, err
		}
		return Result{Path: dst, Note: pathNote}, nil
	case User:
		if err := copyExe(src, dst); err != nil {
			return Result{}, err
		}
		if err := addToPath(dir, registry.CURRENT_USER, `Environment`); err != nil {
			return Result{}, err
		}
		return Result{Path: dst, Note: pathNote}, nil
	case Home:
		if err := copyExe(src, dst); err != nil {
			return Result{}, err
		}
		return Result{Path: dst, Note: "This location is not on PATH by design — the Godot plugin launches it, or run it via the full path."}, nil
	}
	return Result{}, fmt.Errorf("unknown destination %v", d)
}

// isAdmin reports whether the process token is a member of the Administrators group.
func isAdmin() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)
	member, err := windows.GetCurrentProcessToken().IsMember(sid)
	return err == nil && member
}

// addToPath appends dir to the Path value under the given registry key,
// idempotently (no-op if already present, case-insensitively).
func addToPath(dir string, root registry.Key, path string) error {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	cur, _, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return err
	}
	for _, p := range strings.Split(cur, ";") {
		if strings.EqualFold(strings.TrimSpace(p), dir) {
			return nil
		}
	}
	updated := dir
	if cur != "" {
		updated = cur + ";" + dir
	}
	return k.SetStringValue("Path", updated)
}
