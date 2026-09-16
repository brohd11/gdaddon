# Dependencies

An addon can declare that it needs other addons, and gdaddon will help you get them.

## Declaring them

An addon's author adds a `require` line to its `plugin.cfg` or `version.cfg`:

```
require=["owner/repo@v1.0.0", "owner/other"]
```

The existing `deps` spelling remains valid. If both keys are present, `require` wins,
even when its list is empty.

A spec is `[clone:]owner/repo[@ref]`. The host defaults to github.com — write
`host/owner/repo` for anything else — and both the prefix and the `@ref` are optional:

| spec | means |
| --- | --- |
| `owner/repo@v1.0.0` | at least v1.0.0 |
| `owner/repo@latest` | the newest published non-prerelease, pinned when it's recorded |
| `owner/repo` | any version — recorded unpinned, installed from the default branch |
| `clone:owner/repo@main` | a live git checkout of branch `main` |
| `clone:owner/repo` | a live git checkout of the remote's default branch |

`@` always names the ref and nothing else. A clone is asked for with the `clone:` prefix,
never by spelling it into the ref: `@clone-main` is just a branch called `clone-main`.
`package:` is accepted as the explicit spelling of the default. `submodule:` is not —
a submodule belongs to the parent repo, and gdaddon never installs one.

A tag does not need a published release. gdaddon prefers the matching release's
uploaded package; when the tag has no release, it downloads and extracts the tag's
source ZIP. Multiple uploaded builds still require an explicit asset choice.
The local package archive is checked first, allowing saved packages to work offline.

Tags without releases can also be selected through **Tags** in the package browser.
They do not participate in “latest” installs or automatic updates, which use published
releases. `@main` names a tag, not a branch — use `clone:owner/repo@main` for the branch.

### The two words that aren't tags

`latest` is reserved. It resolves to the newest published non-prerelease **when the entry
is recorded**, and the manifest stores the tag it resolved to, not the word — so the entry
is a real pin and moving it forward stays an explicit act (`gdaddon update-addons`). A repo
that publishes a literal rolling tag named `latest` can't be pinned to it by name; the
reserved meaning wins.

A `clone:` dep's `@ref` is a *branch*, so it isn't version-compared at all. The entry it
creates records `kind: clone` with the branch in `tag:` and no `version:` — a checkout
tracks a branch, so there is nothing to pin. Leaving the ref off clones the remote's
default branch, and the branch it lands on is written back to the entry.

Requiring a clone says how a dependency the manifest *lacks* should be added. It does not
override an entry you already have: if you deliberately pinned that repo to a release, it
still satisfies the requirement and gdaddon leaves it alone.

Matching is against the entry's `tag` — the release identity — not its `version`, since
the version string in a `plugin.cfg` is the author's to invent and often disagrees with
the release it shipped in. Comparison is semver `>=`, so a newer tag satisfies a dep.

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
    https://github.com/someone/util-lib/releases/download/v0.4.1/util-lib.zip
  Install it? [y/N/a(ll)/q(uit)]:
```

A dependency that asked for a checkout says so, since accepting it means live code with
its own `.git` that tracks a branch rather than a pinned snapshot:

```
  my-addon declares a dependency:
    github.com/someone/util-lib (clone: main)
    https://github.com/someone/util-lib.git
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
