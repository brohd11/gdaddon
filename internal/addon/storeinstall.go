package addon

import (
	"context"
	"strings"

	"github.com/brohd11/gdaddon/internal/store"
)

// storeInstall installs an Asset Store entry: resolve the release zip for its tag (else
// the older version field, else the newest stable), download and unwrap it with fetchZip,
// and place it like a package. The version comes from the installed config (or the
// release id without "v").
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

	report("[%s] Resolving store release...", a.Label())
	downloadURL, err := store.ResolveDownload(ctx, id, sel)
	if err != nil {
		return InstallResult{}, err
	}

	stagingRoot, pkgName, cleanup, err := fetchZip(ctx, downloadURL, a.Label(), report)
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
