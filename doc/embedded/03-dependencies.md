# Dependencies

An addon can declare that it needs other addons, and gdaddon will help you get them.

## Declaring them

An addon's author adds a `require` line to its `plugin.cfg`, `addon.cfg` or `version.cfg`,
under an `[addon]` or `[plugin]` section (keys in `[addon]` override the same keys in `[plugin]`):

```
require=["owner/repo@v1.0.0", "owner/other"]
```

The existing `deps` spelling remains valid. If both keys are present, `require` wins,
even when its list is empty.

A spec is `owner/repo[/release][@ref]`, the same grammar PackageExporter builds
against, so one `require` line means the same thing to both. The host defaults to
github.com; write `host/owner/repo` for anything else (a first segment is a host only when
it has a dot).

| spec | means |
| --- | --- |
| `owner/repo@v1.0.0` | v1.0.0's source, or newer |
| `owner/repo/release@v1.0.0` | v1.0.0's uploaded release asset |
| `owner/repo/release@v1.0.0:*-linux.zip` | the upload matching the glob |
| `owner/repo@latest` | the newest published non-prerelease, pinned when it's recorded |
| `owner/repo` | any version — recorded unpinned for you to pin |

A plain tag is the tag's **source**: the repo as it was at that tag, which is what
PackageExporter reads when it builds. `/source` says the same thing explicitly. A tag
needs no published release, and an uploaded build does not replace it.

`/release` is for what can't be used from source — a GDExtension, or a third-party zip.
It installs the tag's uploaded asset and never falls back to source; a release with no
upload is an error that tells you to drop `/release`. When a release uploads several
assets, `:<glob>` picks one (git forbids `:` in tags, so the split is unambiguous). It is
a glob rather than a name so the pin survives a tag bump.

The local package archive is checked first, allowing saved packages to work offline.

### Clones are an install-time choice

A `require` can't ask for a live checkout: a build reads a tag, never a branch, so
PackageExporter skips a `/clone` spec and so does gdaddon. Clone what you are working on
when you install it — `gdaddon install owner/repo/clone@main`, or **Clone** in the TUI. A
repo can list those commands in a "for development" section of its readme.

The older `clone:owner/repo` prefix is read as plain source, as PackageExporter reads it.

A clone entry satisfies any requirement on its repo: it is a checkout being worked on, so
gdaddon doesn't compare its branch against the required tag.

### The reserved word

`latest` is reserved. It resolves to the newest published non-prerelease **when the entry
is recorded**, and the manifest stores the tag it resolved to, not the word — so the entry
is a real pin and moving it forward stays an explicit act (`gdaddon update-addons`). A repo
that publishes a literal rolling tag named `latest` can't be pinned to it by name; the
reserved meaning wins.

Matching is against the entry's `tag` — the release identity — not its `version`, since
the version string in a `plugin.cfg` is the author's to invent and often disagrees with
the release it shipped in. Comparison is semver `>=`, so a newer tag satisfies a dep.
Source and release aren't compared: an entry installed either way satisfies a requirement
at its tag.

## The Dependencies screen

Any installed addon that declares dependencies grows a Dependencies action. It lists every
declared dep with its status:

- `installed` — present and satisfying the version
- `not installed` — in your manifest, but not on disk yet
- `missing` — not in the manifest at all
- `outdated` — installed, but older than the dep asks for
- `suppressed` — declared, but you've told gdaddon to ignore it

From that screen, Add all missing writes the manifest-absent deps into the manifest
(pinned to the requested tag, or untagged when the dep didn't ask for one). It only adds
them — Install All then installs them. A submenu on any single dep adds just that one.

## Confirming them on the command line

`gdaddon install <owner/repo>` installs the addon you named *and* the closure of
dependencies it declares — and those are declared by the addon's author, not by you, so
an install can reach repos you never chose. Since a Godot addon is editor code that runs
when you open the project, each dependency is confirmed before it is recorded or
downloaded:

```
  my-addon declares a dependency:
    github.com/someone/util-lib @v0.4.1
    https://github.com/someone/util-lib/archive/refs/tags/v0.4.1.zip
  Install it? [y/N/a(ll)/q(uit)]:
```

- `y` installs it, `n` (or just enter) skips it, `a` accepts every remaining dependency
  this run, `q` stops the walk and leaves the rest alone.
- Declining a dependency also skips whatever *it* declares. Refusing a package refuses
  the subtree under it, which is the point of asking.
- Nothing is written until you answer, so declining leaves no manifest entry behind.

`--trust-deps` accepts the whole closure up front, for a publisher you already trust.
`--no-deps` installs nothing but the addon you named. The two can't be combined.

Off a terminal — a script, a pipeline, CI — there is nobody to ask, so dependencies are
skipped and listed rather than installed:

```
installed my-addon 1.2.0 → addons/my_addon

skipped 1 dependency (not a terminal, nothing to confirm with):
  github.com/someone/util-lib @v0.4.1
pass --trust-deps to install them
```

The addon you named is never confirmed — you asked for that one. Neither are manifest
entries that already exist: `install --all` confirms only the dependencies it newly
discovers, since anything already recorded is in a file you control. The TUI's
Actions ▸ Install All Deps keeps its single up-front confirm.

## Suppressing a dep

Sometimes a declared dep is wrong, already vendored, or simply not wanted. Press `s` on
it (or use its submenu) to suppress it.

That writes a `suppress_deps` list onto the *declaring* addon's manifest entry:

```
my_addon:
  url: https://github.com/owner/my_addon.git
  path: addons/my_addon
  suppress_deps: ["owner/unwanted"]
```

A suppressed dep never raises the missing-deps warning and is never picked up by Add all.
It's recorded in the manifest, so the decision travels with the project.

## The row marker

A Project row keeps its missing-deps marker until every declared, non-suppressed dep is
actually *installed* — not merely listed in the manifest. Adding a dep quiets nothing on
its own; installing it does.
