package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"

	"github.com/spf13/cobra"
)

var (
	addonInstallRoot   string
	addonInstallAsset  string
	addonInstallName   string
	addonInstallClone  bool
	addonInstallNoDeps bool
)

// manifestFileName is the manifest created when a project doesn't have one yet. It is
// the first of addon.manifestNames, which is what the discovery walk looks for.
const manifestFileName = "addon_manifest.yml"

var addonInstallCmd = &cobra.Command{
	Use:   "install <owner/repo>[@tag]",
	Short: "Install a Godot addon into this project and record it in the manifest",
	Long: `Install downloads one addon and records it in the project's addon manifest,
along with the dependencies that addon declares (and theirs, and so on).

The repo is named as owner/repo, or host/owner/repo for a host other than
github.com — the same shorthand a plugin.cfg 'deps' entry uses. An optional
@tag pins a release; without one the latest non-prerelease is installed.

The install location is worked out from the downloaded package: a repo whose
root holds a plugin.cfg is installed whole, a repo shipping an addons/ folder
has its plugin folders mirrored under the project's addons/, and anything else
is located by its plugin.cfg/version.cfg files. An addon can override this by
declaring dir="addons/whatever" in its own config.

The project root is the git toplevel unless --root says otherwise, and the
manifest is found by walking down from it — one is created if there is none.

  gdaddon install brohd11/my-addon
  gdaddon install brohd11/my-addon@v1.2.0
  gdaddon install codeberg.org/someone/their-addon
  gdaddon install brohd11/my-addon --clone         # git checkout, default branch
  gdaddon install brohd11/my-addon@dev --clone     # git checkout, branch dev

To install the gdaddon binary itself, see 'gdaddon self-install'.`,
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: false,
	RunE:          runAddonInstall,
}

func init() {
	f := addonInstallCmd.Flags()
	f.StringVar(&addonInstallRoot, "root", "", "project root (default: the git toplevel, else the current directory)")
	f.StringVar(&addonInstallAsset, "asset", "", "pick a release asset by name (substring) when the release ships several")
	f.StringVar(&addonInstallName, "name", "", "manifest entry name (default: derived from the repo)")
	f.BoolVar(&addonInstallClone, "clone", false, "install as a git checkout of a branch (@ref names the branch) instead of a release")
	f.BoolVar(&addonInstallNoDeps, "no-deps", false, "don't install the dependencies the addon declares")
	rootCmd.AddCommand(addonInstallCmd)
}

func runAddonInstall(cmd *cobra.Command, args []string) error {
	spec, ok := addon.ParseRepoSpec(args[0])
	if !ok {
		return fmt.Errorf("could not parse %q: expected owner/repo, host/owner/repo, either with an optional @tag", args[0])
	}

	projectRoot, err := resolveRootQuiet(addonInstallRoot)
	if err != nil {
		return err
	}
	manifestPath, err := findOrCreateManifest(projectRoot)
	if err != nil {
		return err
	}

	ctx := context.Background()
	entry, err := resolveEntry(ctx, spec)
	if err != nil {
		return err
	}

	report := func(format string, a ...any) { fmt.Printf(format+"\n", a...) }
	res, err := addon.InstallOne(ctx, addon.InstallOneOpts{
		ManifestPath: manifestPath,
		ProjectRoot:  projectRoot,
		Entry:        entry,
		Deps:         !addonInstallNoDeps,
		Report:       report,
	})
	if err != nil {
		return installHint(err, entry.Name)
	}

	fmt.Printf("\ninstalled %s", res.Name)
	if res.Version != "" {
		fmt.Printf(" %s", res.Version)
	}
	if res.Path != "" {
		fmt.Printf(" → %s", res.Path)
	}
	fmt.Println()
	for _, d := range res.Deps {
		fmt.Printf("  dependency %s → %s\n", d.Name, d.Path)
	}
	return nil
}

// resolveEntry turns the parsed spec into the manifest entry to install: a clone entry
// pointing at the canonical .git url (the branch, if any, coming from @ref), or a
// release entry pinned to one downloadable asset.
func resolveEntry(ctx context.Context, spec addon.Dependency) (addon.Addon, error) {
	name := addonInstallName
	if name == "" {
		name = addon.DeriveName(spec.RepoURL)
	}

	if addonInstallClone {
		return addon.Addon{
			Name: name,
			URL:  "https://" + spec.RepoID + ".git",
			Tag:  spec.Tag, // empty → whatever the remote's default branch is
			Kind: addon.KindClone,
		}, nil
	}

	listing, err := source.AvailableVersions(ctx, spec.RepoURL)
	if err != nil {
		return addon.Addon{}, fmt.Errorf("could not list versions of %s: %w", spec.RepoID, err)
	}
	rel, err := addon.SelectRelease(listing.Releases, spec.Tag)
	if err != nil {
		return addon.Addon{}, fmt.Errorf("%s: %w", spec.RepoID, err)
	}
	asset, err := addon.SelectAsset(rel, addonInstallAsset)
	if err != nil {
		return addon.Addon{}, fmt.Errorf("%s: %w", spec.RepoID, err)
	}

	fmt.Printf("%s %s (%s)\n", spec.RepoID, rel.Tag, asset.Name)
	return addon.Addon{Name: name, URL: asset.URL, Tag: rel.Tag, Kind: addon.KindPackage}, nil
}

// findOrCreateManifest locates the project's manifest, creating an empty one at the
// project root when there is none — so installing into a fresh project just works.
// This is deliberately unlike discoverManifest, which errors on a miss: the read-only
// paths (--list) have nothing useful to do without a manifest, but an install does.
func findOrCreateManifest(projectRoot string) (string, error) {
	manifestPath, err := addon.FindManifest(projectRoot)
	if err != nil {
		return "", err
	}
	if manifestPath != "" {
		return manifestPath, nil
	}
	manifestPath = filepath.Join(projectRoot, manifestFileName)
	if err := addon.CreateManifest(manifestPath); err != nil {
		return "", err
	}
	fmt.Printf("created %s\n", manifestPath)
	return manifestPath, nil
}

// installHint annotates the errors whose fix isn't obvious from the message alone.
// AddEntry rejects a name already used by a *different* repo's entry (a same-repo
// re-install goes through UpsertEntry and never lands here), and the fix is a flag.
func installHint(err error, name string) error {
	if errors.Is(err, addon.ErrNameTaken) {
		return fmt.Errorf("%w — install it under another name with --name", err)
	}
	return err
}
