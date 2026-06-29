//go:build !windows

package installer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"gdaddon/internal/config"
)

func optionLabel(d Dest) string {
	switch d {
	case System:
		return "System  (/usr/local/bin)"
	case User:
		return "User  (~/.local/bin)"
	case Home:
		return "gdaddon home  (~/.gdaddon/bin)"
	}
	return ""
}

func optionDesc(d Dest) string {
	switch d {
	case System:
		return "On PATH by default. Needs sudo."
	case User:
		return "No sudo. May need PATH setup."
	case Home:
		return "No sudo, not on PATH. Launched by the Godot plugin."
	}
	return ""
}

// exeName is the binary's filename on this platform.
func exeName() string { return "gdaddon" }

// destDir maps a Dest to its concrete directory.
func destDir(d Dest) (string, error) {
	switch d {
	case System:
		return "/usr/local/bin", nil
	case User:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "bin"), nil
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

	switch d {
	case System:
		if err := installSystem(src, dir, dst); err != nil {
			return Result{}, err
		}
		return Result{Path: dst}, nil
	case User:
		if err := copyExe(src, dst); err != nil {
			return Result{}, err
		}
		return Result{Path: dst, Note: pathNote(dir)}, nil
	case Home:
		if err := copyExe(src, dst); err != nil {
			return Result{}, err
		}
		return Result{Path: dst, Note: "This location is not on PATH by design — the Godot plugin launches it, or run it via the full path."}, nil
	}
	return Result{}, fmt.Errorf("unknown destination %v", d)
}

// installSystem copies into /usr/local/bin, falling back to sudo when the dir
// isn't writable. sudo prompts on the (now TUI-free) terminal.
func installSystem(src, dir, dst string) error {
	err := copyExe(src, dst)
	if err == nil {
		return nil
	}
	if !errors.Is(err, fs.ErrPermission) {
		return err
	}
	fmt.Printf("writing to %s requires elevated permissions (sudo)...\n", dir)
	if err := runSudo("mkdir", "-p", dir); err != nil {
		return err
	}
	if err := runSudo("cp", src, dst); err != nil {
		return err
	}
	return runSudo("chmod", "+x", dst)
}

// removeAt deletes path, falling back to sudo when it isn't writable (the system
// copy in /usr/local/bin).
func removeAt(path string) error {
	err := os.Remove(path)
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		return err
	}
	fmt.Printf("removing %s requires elevated permissions (sudo)...\n", path)
	return runSudo("rm", "-f", path)
}

func runSudo(args ...string) error {
	cmd := exec.Command("sudo", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// pathNote returns guidance to add dir to PATH, or "" if it's already there.
func pathNote(dir string) string {
	if onPath(dir) {
		return ""
	}
	profile := profileFile()
	return fmt.Sprintf("%s is not on your PATH. Add it with:\n  echo 'export PATH=\"%s:$PATH\"' >> %s\nthen restart your shell (or 'source %s').", dir, dir, profile, profile)
}

func onPath(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == dir {
			return true
		}
	}
	return false
}

// profileFile picks the shell profile to suggest based on $SHELL.
func profileFile() string {
	home, _ := os.UserHomeDir()
	switch filepath.Base(os.Getenv("SHELL")) {
	case "zsh":
		return filepath.Join(home, ".zshrc")
	case "bash":
		return filepath.Join(home, ".bashrc")
	default:
		return filepath.Join(home, ".profile")
	}
}
