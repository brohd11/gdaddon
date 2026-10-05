package addon

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/brohd11/gdaddon/internal/config"
)

// gitignoreEnabled reports config.yml's gitignore_installs toggle. A var so tests can
// pin it instead of reading the user's real config.
var gitignoreEnabled = func() bool {
	cfg, err := config.Load()
	return err == nil && cfg.GitignoreInstalls
}

// ignoreLine is the .gitignore line gdaddon writes for an install path: anchored to the
// project root and matching the directory only.
func ignoreLine(rel string) string {
	return "/" + filepath.ToSlash(filepath.Clean(rel)) + "/"
}

// IgnoreInstall adds rel (project-relative) to projectRoot's .gitignore when the toggle is
// on and no existing line already covers it or an ancestor. It reports whether a line was
// added.
func IgnoreInstall(projectRoot, rel string) (bool, error) {
	if rel == "" || !gitignoreEnabled() {
		return false, nil
	}
	path := filepath.Join(projectRoot, ".gitignore")
	data, mode, err := readGitignore(path)
	if err != nil {
		return false, err
	}
	if ignoreCovered(string(data), rel) {
		return false, nil
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	data = append(data, ignoreLine(rel)+"\n"...)
	return true, os.WriteFile(path, data, mode)
}

// MoveIgnore follows a relocation: it drops the line gdaddon wrote for fromRel (lines in
// any other spelling are the user's and left alone) and ignores toRel.
func MoveIgnore(projectRoot, fromRel, toRel string) error {
	if fromRel == toRel || !gitignoreEnabled() {
		return nil
	}
	path := filepath.Join(projectRoot, ".gitignore")
	data, mode, err := readGitignore(path)
	if err != nil {
		return err
	}
	old := ignoreLine(fromRel)
	lines := strings.SplitAfter(string(data), "\n")
	kept := slices.DeleteFunc(slices.Clone(lines), func(l string) bool {
		return strings.TrimSpace(l) == old
	})
	if len(kept) != len(lines) {
		if err := os.WriteFile(path, []byte(strings.Join(kept, "")), mode); err != nil {
			return err
		}
	}
	_, err = IgnoreInstall(projectRoot, toRel)
	return err
}

// readGitignore returns the file's contents and mode; a missing file is empty, 0o644.
func readGitignore(path string) ([]byte, os.FileMode, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, 0o644, nil
	}
	if err != nil {
		return nil, 0, err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return data, mode, nil
}

// ignoreCovered reports whether a plain line in content names rel or one of its
// ancestors, ignoring leading and trailing slashes. Comments and negations never cover;
// globs are not interpreted (at worst a redundant line is added).
func ignoreCovered(content, rel string) bool {
	want := map[string]bool{}
	for p := filepath.ToSlash(filepath.Clean(rel)); p != "." && p != "/"; p = filepath.ToSlash(filepath.Dir(p)) {
		want[p] = true
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		if want[strings.Trim(line, "/")] {
			return true
		}
	}
	return false
}
