package addon

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/brohd11/gdaddon/internal/source"
	"github.com/brohd11/gdaddon/internal/version"
)

const editorPluginRepo = "https://github.com/brohd11/gdaddon-editor-plugin"
const editorPluginPath = "addons/gdaddon"

// InstallEditorPlugin installs the companion's published export without involving a
// manifest, dependency resolution, or the global addon registry. project must itself
// contain project.godot; unlike other addon operations, there is no discovery walk.
func InstallEditorPlugin(ctx context.Context, project string, force bool, report Reporter) (InstallResult, error) {
	if report == nil {
		report = func(string, ...any) {}
	}
	marker, err := os.Stat(filepath.Join(project, "project.godot"))
	if err != nil || !marker.Mode().IsRegular() {
		return InstallResult{}, fmt.Errorf("run plugin install from the Godot project root: no regular project.godot in %s", project)
	}
	dest := filepath.Join(project, filepath.FromSlash(editorPluginPath))
	if err := checkEditorPluginPaths(project, dest); err != nil {
		return InstallResult{}, err
	}
	listing, err := source.AvailableVersions(ctx, editorPluginRepo)
	if err != nil {
		return InstallResult{}, err
	}
	var stable []source.Release
	for _, release := range listing.Releases {
		if !release.IsPrerelease() {
			stable = append(stable, release)
		}
	}
	rel, err := SelectRelease(stable, "")
	if err != nil {
		return InstallResult{}, fmt.Errorf("gdaddon editor plugin: %w", err)
	}
	var uploads []source.Asset
	for _, asset := range source.Uploads(rel) {
		if strings.HasSuffix(strings.ToLower(asset.Name), ".zip") {
			uploads = append(uploads, asset)
		}
	}
	if len(uploads) != 1 {
		return InstallResult{}, fmt.Errorf("gdaddon editor plugin release %s: expected one uploaded ZIP, found %d (%s)", rel.Tag, len(uploads), strings.Join(assetNames(uploads), ", "))
	}
	report("gdaddon editor plugin %s (%s)", rel.Tag, uploads[0].Name)
	staging, _, cleanup, err := fetchZip(ctx, uploads[0].URL, "gdaddon", report)
	if err != nil {
		return InstallResult{}, err
	}
	defer cleanup()
	// Inspect the staged export before changing anything in the project. Pin the
	// destination here rather than accepting dir/path overrides from the package.
	dirs := pluginDirs(staging)
	if len(dirs) != 1 {
		return InstallResult{}, fmt.Errorf("expected one gdaddon editor plugin in release ZIP, found %d", len(dirs))
	}
	src := dirs[0]
	name, incoming, script, err := editorPluginConfig(src)
	if err != nil || name != "gdaddon" || incoming == "" || !filepath.IsLocal(filepath.FromSlash(script)) {
		return InstallResult{}, fmt.Errorf("release ZIP must contain a gdaddon plugin.cfg with a version and a local script")
	}
	if info, err := os.Stat(filepath.Join(src, filepath.FromSlash(script))); err != nil || !info.Mode().IsRegular() {
		return InstallResult{}, fmt.Errorf("release ZIP is missing the plugin script %q", script)
	}
	// Check again after downloading, before walking or overwriting the destination.
	if err := checkEditorPluginPaths(project, dest); err != nil {
		return InstallResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return InstallResult{}, err
	}
	if info, err := os.Lstat(dest); err == nil {
		name, installed, _, cfgErr := editorPluginConfig(dest)
		if !force {
			if !info.IsDir() || cfgErr != nil || name != "gdaddon" {
				return InstallResult{}, fmt.Errorf("%s exists but is not the gdaddon editor plugin; use --force to replace it", dest)
			}
			if order, ok := version.Compare(installed, incoming); ok && order > 0 {
				return InstallResult{}, fmt.Errorf("installed gdaddon editor plugin %s is newer than release %s; use --force to replace it", installed, incoming)
			}
		}
		// Never follow existing links when overwriting files, including with --force.
		if info.IsDir() {
			if err := rejectEditorPluginLinks(dest); err != nil {
				return InstallResult{}, err
			}
		} else if err := os.Remove(dest); err != nil {
			return InstallResult{}, err
		}
	} else if !os.IsNotExist(err) {
		return InstallResult{}, err
	}
	if err := copyDir(src, dest); err != nil {
		return InstallResult{}, err
	}
	if err := pruneEditorPlugin(src, dest); err != nil {
		return InstallResult{}, err
	}
	if added, err := IgnoreInstall(project, editorPluginPath); err != nil {
		report("could not update .gitignore: %v", err)
	} else if added {
		report("added %s to .gitignore", ignoreLine(editorPluginPath))
	}
	return InstallResult{Path: editorPluginPath, Version: incoming, Name: "gdaddon"}, nil
}

// Read the actual [plugin] section, rather than package metadata overlays: this is
// an EditorPlugin whose name and script Godot must recognize.
func editorPluginConfig(dir string) (name, ver, script string, err error) {
	cfg, err := loadConfig(filepath.Join(dir, "plugin.cfg"))
	if err != nil {
		return "", "", "", err
	}
	s, err := cfg.GetSection("plugin")
	if err != nil {
		return "", "", "", err
	}
	value := func(key string) string { return strings.Trim(strings.TrimSpace(s.Key(key).String()), `"'`) }
	return value("name"), value("version"), value("script"), nil
}

func checkEditorPluginPaths(project, dest string) error {
	for _, p := range []string{project, filepath.Dir(dest), dest} {
		info, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to install through symlink %s", p)
		}
	}
	return nil
}

func rejectEditorPluginLinks(dest string) error {
	return filepath.WalkDir(dest, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to overwrite symlink %s", p)
		}
		return nil
	})
}

// Remove stale files and empty directories, retaining Godot-generated sidecars
// only when their underlying file is still shipped by this release.
func pruneEditorPlugin(src, dest string) error {
	var dirs []string
	err := filepath.WalkDir(dest, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dest {
				dirs = append(dirs, p)
			}
			return nil
		}
		rel, err := filepath.Rel(dest, p)
		if err != nil {
			return err
		}
		base := strings.TrimSuffix(strings.TrimSuffix(rel, ".uid"), ".import")
		for _, candidate := range []string{rel, base} {
			if info, err := os.Stat(filepath.Join(src, candidate)); err == nil && info.Mode().IsRegular() {
				return nil
			}
		}
		return os.Remove(p)
	})
	if err != nil {
		return err
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		entries, err := os.ReadDir(dirs[i])
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			if err := os.Remove(dirs[i]); err != nil {
				return err
			}
		}
	}
	return nil
}
