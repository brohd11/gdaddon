package addon

import (
	"context"

	"gdaddon/internal/store"
)

// storeInstall installs an Asset Store entry: resolve the store-hosted release zip
// for the entry's pinned version (the newest stable when none is pinned) from the
// canonical url, download and unwrap it (fetchZip handles any url, skipping
// fetchToStaging's .zip/.git suffix gate), then place it like a normal package. The
// pinned version comes from the installed plugin.cfg, falling back to the entry's
// recorded store version when the package declares none.
func storeInstall(ctx context.Context, a Addon, baseDir string, report Reporter) (InstallResult, error) {
	id, err := store.AssetID(a.URL)
	if err != nil {
		return InstallResult{}, err
	}

	report("[%s] Resolving store release...", a.Name)
	downloadURL, err := store.ResolveDownload(ctx, id, a.Version)
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
		res.Version = a.Version
	}
	return res, nil
}
