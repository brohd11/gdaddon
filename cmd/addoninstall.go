package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"

	"github.com/spf13/cobra"
)

var (
	addonInstallAll       bool
	addonInstallRoot      string
	addonInstallAsset     string
	addonInstallName      string
	addonInstallClone     bool
	addonInstallNoDeps    bool
	addonInstallTrustDeps bool
)

// manifestFileName is the manifest created when a project doesn't have one yet. It is
// the first of addon.manifestNames, which is what the discovery walk looks for.
const manifestFileName = "addon_manifest.yml"

var addonInstallCmd = &cobra.Command{
	Use:   "install [--all | <owner/repo>[@tag]]",
	Short: "Install Godot addons into this project and record them in the manifest",
	Long: `Install downloads addons into the project and records them in its addon
manifest, along with the dependencies they declare (and theirs, and so on).

Name a repo to install one addon, or pass --all to install everything the
manifest already lists.

A repo is named as owner/repo, or host/owner/repo for a host other than
github.com — the same shorthand a plugin.cfg 'deps' entry uses. An optional
@tag pins a release; without one the latest non-prerelease is installed.

The install location is worked out from the downloaded package: a repo whose
root holds a plugin.cfg is installed whole, a repo shipping an addons/ folder
has its plugin folders mirrored under the project's addons/, and anything else
is located by its plugin.cfg/version.cfg files. An addon can override this by
declaring dir="addons/whatever" in its own config.

The project root is the git toplevel unless --root says otherwise (this command
takes it as a flag because its argument is the repo). Naming one addon creates
a manifest if the project has none; --all requires one to already exist.

Dependencies are declared by the addon's own author and followed transitively, so
an install can reach repos you never named. Each one is therefore confirmed before
it is recorded or downloaded; answer 'a' to accept the rest of the run, or pass
--trust-deps to accept them all up front. Off a terminal there is nobody to ask,
so dependencies are skipped and listed instead of installed — an unattended run
needs --trust-deps to resolve them. The addon you name is never confirmed, and
neither are manifest entries that already exist: --all confirms only what it newly
discovers.

  gdaddon install --all                            # the whole manifest, plus deps
  gdaddon install --all --no-deps                  # the manifest's own entries only
  gdaddon install --all --trust-deps               # ... and don't ask about deps
  gdaddon install brohd11/my-addon
  gdaddon install brohd11/my-addon@v1.2.0
  gdaddon install codeberg.org/someone/their-addon
  gdaddon install brohd11/my-addon --clone         # git checkout, default branch
  gdaddon install brohd11/my-addon@dev --clone     # git checkout, branch dev`,
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: false,
	RunE:          runAddonInstall,
}

func init() {
	f := addonInstallCmd.Flags()
	f.BoolVar(&addonInstallAll, "all", false, "install every addon the manifest lists, instead of one named repo")
	f.StringVar(&addonInstallRoot, "root", "", "project root (default: the git toplevel, else the current directory)")
	f.StringVar(&addonInstallAsset, "asset", "", "pick a release asset by name (substring) when the release ships several")
	f.StringVar(&addonInstallName, "name", "", "manifest entry name (default: derived from the repo)")
	f.BoolVar(&addonInstallClone, "clone", false, "install as a git checkout of a branch (@ref names the branch) instead of a release")
	f.BoolVar(&addonInstallNoDeps, "no-deps", false, "don't install declared dependencies")
	f.BoolVar(&addonInstallTrustDeps, "trust-deps", false, "install declared dependencies without confirming each one")
	rootCmd.AddCommand(addonInstallCmd)
}

func runAddonInstall(cmd *cobra.Command, args []string) error {
	if err := checkInstallArgs(args); err != nil {
		return err
	}
	if addonInstallAll {
		return runInstallAll()
	}

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

	prompter, confirm := depConfirmer()
	res, err := addon.InstallOne(ctx, addon.InstallOneOpts{
		ManifestPath: manifestPath,
		ProjectRoot:  projectRoot,
		Entry:        entry,
		Deps:         !addonInstallNoDeps,
		ConfirmDep:   confirm,
		Report:       stdoutReport,
	})
	// A quit at a dependency prompt is a clean stop, not a failure: the addon the user
	// named is installed and pinned by then, and only the closure walk ended early.
	aborted := errors.Is(err, addon.ErrDepAborted)
	if err != nil && !aborted {
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
	if aborted {
		fmt.Println("\nstopped at your request; remaining dependencies were not installed")
	}
	prompter.reportSkipped(os.Stdout)
	return nil
}

// checkInstallArgs rejects the combinations that name no target, two targets, or a
// single-addon option alongside --all. Cobra can express none of these: --all and the
// positional are different kinds of thing, and --asset/--name/--clone are only
// meaningful when there is one addon to describe.
func checkInstallArgs(args []string) error {
	switch {
	case addonInstallAll && len(args) == 1:
		return fmt.Errorf("--all installs the whole manifest; drop %q (or drop --all to install just it)", args[0])
	case !addonInstallAll && len(args) == 0:
		return fmt.Errorf("name a repo to install (owner/repo[@tag]), or pass --all for the whole manifest")
	case addonInstallNoDeps && addonInstallTrustDeps:
		return fmt.Errorf("--no-deps skips dependencies and --trust-deps installs them all; pick one")
	}
	if !addonInstallAll {
		return nil
	}
	// --trust-deps is deliberately absent below: unlike --asset/--name/--clone it
	// describes the dependency policy, not the one addon being installed.
	for flag, set := range map[string]bool{
		"--asset": addonInstallAsset != "",
		"--name":  addonInstallName != "",
		"--clone": addonInstallClone,
	} {
		if set {
			return fmt.Errorf("%s describes a single addon and can't be combined with --all", flag)
		}
	}
	return nil
}

// runInstallAll installs every entry the manifest lists. It resolves declared
// dependencies by default (InstallAllDeps), matching the single-addon form — --no-deps
// stops at the manifest's own entries, which is what the old --install flag did.
//
// Unlike the targeted install this needs a manifest to already exist: creating an empty
// one only to install nothing out of it would be a confusing no-op.
func runInstallAll() error {
	projectRoot, err := resolveRootQuiet(addonInstallRoot)
	if err != nil {
		return err
	}
	manifest, err := discoverManifest(projectRoot)
	if err != nil {
		return err
	}
	statuses, err := addon.Inspect(manifest, projectRoot)
	if err != nil {
		return err
	}
	if len(statuses) == 0 {
		fmt.Println("No addons found in YAML.")
		return nil
	}

	ctx := context.Background()
	if addonInstallNoDeps {
		_, err = addon.InstallAll(ctx, manifest, statuses, projectRoot, stdoutReport)
		return err
	}

	prompter, confirm := depConfirmer()
	_, err = addon.InstallAllDeps(ctx, manifest, projectRoot, confirm, stdoutReport)
	if errors.Is(err, addon.ErrDepAborted) {
		fmt.Println("\nstopped at your request; remaining dependencies were not installed")
		err = nil
	}
	prompter.reportSkipped(os.Stdout)
	return err
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
