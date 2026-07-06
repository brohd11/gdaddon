package addon

import (
	"context"
	"strings"

	"gdaddon/internal/store"
)

// storeInstall installs an Asset Store entry: resolve the store-hosted release zip
// for the entry's pinned release (the newest stable when none is pinned) from the
// canonical url, download and unwrap it (fetchZip handles any url, skipping
// fetchToStaging's .zip/.git suffix gate), then place it like a normal package. The
// release is selected by the entry's tag (the store release identity, e.g. "v3.10.2"),
// falling back to the older version field for manifests predating the tag split. The
// recorded version comes from the installed plugin.cfg (the "v"-stripped release id
// when the package ships no config).
func storeInstall(ctx context.Context, a Addon, baseDir string, report Reporter) (InstallResult, error) {
	id, err := store.AssetID(a.URL)
	if err != nil {
		return InstallResult{}, err
	}

	// The store release identity lives in tag now; fall back to version so entries
	// written before the tag split (release id under version:) still resolve.
	sel := a.Tag
	if sel == "" {
		sel = a.Version
	}

	report("[%s] Resolving store release...", a.Name)
	downloadURL, err := store.ResolveDownload(ctx, id, sel)
	if err != nil {
		return InstallResult{}, err
	}

	stagingRoot, pkgName, cleanup, err := fetchZip(ctx, downloadURL, a.Name, report)
	if err != nil {
		return InstallResult{}, err
	}
	defer cleanup()

	res, err := installStaged(stagingRoot, pkgName, a, baseDir, report)
	if err != nil {
		return InstallResult{}, err
	}
	if res.Version == "" {
		res.Version = strings.TrimPrefix(sel, "v")
	}
	return res, nil
}
