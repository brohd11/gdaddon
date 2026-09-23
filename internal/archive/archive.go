// Package archive keeps local copies of installed package zips (default
// ~/.gdaddon/archive, per repo and version) so addons can be reinstalled after the
// upstream disappears. They reappear in version listings as " (archived)" assets.
package archive

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/brohd11/gdaddon/internal/config"
	"github.com/brohd11/gdaddon/internal/restrule"
	"github.com/brohd11/gdaddon/internal/source"

	"gopkg.in/yaml.v3"
)

// ArchivedSuffix marks an asset name as coming from the local archive.
const ArchivedSuffix = " (archived)"

// Logf receives non-fatal archive problems (an index.yml refresh failure); the TUI sends
// them to its log. nil elsewhere.
var Logf func(format string, args ...any)

// Dir resolves the archive root: ~/.gdaddon/config/config.yml's archive_dir if set,
// otherwise ~/.gdaddon/archive. A leading "~" in archive_dir is expanded.
func Dir() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return cfg.ResolvedArchiveDir()
}

// repoDir is the per-repo folder name derived from a repo id, e.g.
// "github.com/owner/repo" -> "github.com_owner_repo".
func repoDir(repoID string) string {
	return strings.ReplaceAll(repoID, "/", "_")
}

// Store writes an asset's bytes to <root>/<repoDir>/<tag>/<assetName> and
// refreshes index.yml. It returns the absolute path of the stored file.
func Store(repoID, tag, assetName string, r io.Reader) (string, error) {
	root, err := Dir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, repoDir(repoID), tag)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, assetName)
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	refreshIndex(root)
	return dest, nil
}

// Archive downloads a remote asset zip and stores it under the repo/tag. Assets
// whose URL is already a local path are skipped (nothing to fetch).
func Archive(ctx context.Context, repoID, tag string, asset source.Asset) error {
	if !strings.HasPrefix(asset.URL, "http") {
		return nil
	}

	resp, err := restrule.Get(ctx, asset.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// A commit-pinned branch package stores under "<branch>@<sha>", so commits do not collide
	// and listDir can recover the pin.
	storeTag := tag
	if asset.Commit != "" {
		storeTag = tag + "@" + asset.Commit
	}
	if _, err := Store(repoID, storeTag, cleanAssetName(asset.Name), resp.Body); err != nil {
		return err
	}
	return nil
}

// shaTag matches a tag-dir suffix that is a git commit sha (7–40 hex chars), used to
// split a commit-pinned branch package's tag dir back into branch + sha.
var shaTag = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// parseArchiveTag splits "<branch>@<sha>" into tag and commit; other names return (dir, "").
func parseArchiveTag(dir string) (tag, commit string) {
	if i := strings.LastIndex(dir, "@"); i != -1 && shaTag.MatchString(dir[i+1:]) {
		return dir[:i], dir[i+1:]
	}
	return dir, ""
}

// cleanAssetName strips the archived suffix (so re-archiving an archived asset
// doesn't compound it) and any path separators.
func cleanAssetName(name string) string {
	name = strings.TrimSuffix(name, ArchivedSuffix)
	return filepath.Base(name)
}

// RemoveRepo deletes all archived packages for a repo and refreshes index.yml; missing is a
// no-op.
func RemoveRepo(repoID string) error {
	root, err := Dir()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(root, repoDir(repoID))); err != nil {
		return err
	}
	refreshIndex(root)
	return nil
}

// List returns a repo's archived packages as releases (newest first) with local-file urls;
// (nil, nil) when none.
func List(repoID string) ([]source.Release, error) {
	root, err := Dir()
	if err != nil {
		return nil, err
	}
	return listDir(filepath.Join(root, repoDir(repoID)))
}

// listDir reads one repo folder into releases, newest first; (nil, nil) when missing.
func listDir(base string) ([]source.Release, error) {
	tagDirs, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var releases []source.Release
	for _, td := range tagDirs {
		if !td.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(base, td.Name()))
		if err != nil {
			continue
		}
		tag, commit := parseArchiveTag(td.Name())
		rel := source.Release{Tag: tag}
		rel.Prerelease = rel.IsPrerelease()
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			rel.Assets = append(rel.Assets, source.Asset{
				Name:   f.Name() + ArchivedSuffix,
				URL:    filepath.Join(base, td.Name(), f.Name()),
				Commit: commit,
			})
		}
		if len(rel.Assets) > 0 {
			releases = append(releases, rel)
		}
	}
	source.SortReleases(releases)
	return releases, nil
}

// RepoArchive is one repo's archived packages. ID is a lossy display label from the folder
// name; asset paths are the real key.
type RepoArchive struct {
	ID       string
	Releases []source.Release
}

// Repos enumerates every archived repo (one per folder under the archive root),
// newest-archived-first by display id. A missing/empty archive returns (nil, nil).
func Repos() ([]RepoArchive, error) {
	root, err := Dir()
	if err != nil {
		return nil, err
	}
	dirs, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var repos []RepoArchive
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		releases, err := listDir(filepath.Join(root, d.Name()))
		if err != nil || len(releases) == 0 {
			continue
		}
		repos = append(repos, RepoArchive{
			ID:       strings.ReplaceAll(d.Name(), "_", "/"),
			Releases: releases,
		})
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].ID < repos[j].ID })
	return repos, nil
}

// Remove deletes one archived asset by path, prunes empty folders up to the root, and
// refreshes index.yml.
func Remove(path string) error {
	if err := os.Remove(path); err != nil {
		return err
	}
	root, err := Dir()
	if err != nil {
		return err
	}
	// Walk up from the asset's tag dir, removing now-empty folders, but never the
	// archive root itself.
	for dir := filepath.Dir(path); dir != root && strings.HasPrefix(dir, root); dir = filepath.Dir(dir) {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		if err := os.Remove(dir); err != nil {
			break
		}
	}
	refreshIndex(root)
	return nil
}

// refreshIndex rewrites index.yml, reporting failures through Logf: the index is only a
// readable cache.
func refreshIndex(root string) {
	if err := writeIndex(root); err != nil && Logf != nil {
		Logf("archive: index refresh failed: %v", err)
	}
}

// Merge adds archived releases to a listing (joining matching tags); a nil listing yields
// an archive-only one.
func Merge(listing *source.Listing, archived []source.Release) *source.Listing {
	if listing == nil {
		listing = &source.Listing{}
	}
	for _, ar := range archived {
		found := false
		for i := range listing.Releases {
			if listing.Releases[i].Tag == ar.Tag {
				listing.Releases[i].Assets = append(listing.Releases[i].Assets, ar.Assets...)
				found = true
				break
			}
		}
		if !found {
			listing.Releases = append(listing.Releases, ar)
		}
	}
	return listing
}

// writeIndex regenerates a human-readable index.yml (repoDir -> tag -> files).
// It is not load-bearing; List reads the directory tree directly.
func writeIndex(root string) error {
	repos, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	index := map[string]map[string][]string{}
	for _, r := range repos {
		if !r.IsDir() {
			continue
		}
		tags, err := os.ReadDir(filepath.Join(root, r.Name()))
		if err != nil {
			continue
		}
		tagMap := map[string][]string{}
		for _, t := range tags {
			if !t.IsDir() {
				continue
			}
			files, err := os.ReadDir(filepath.Join(root, r.Name(), t.Name()))
			if err != nil {
				continue
			}
			var names []string
			for _, f := range files {
				if !f.IsDir() {
					names = append(names, f.Name())
				}
			}
			if len(names) > 0 {
				tagMap[t.Name()] = names
			}
		}
		if len(tagMap) > 0 {
			index[r.Name()] = tagMap
		}
	}

	data, err := yaml.Marshal(index)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "index.yml"), data, 0o644)
}
