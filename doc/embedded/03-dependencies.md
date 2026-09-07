# Dependencies

An addon can declare that it needs other addons, and gdaddon will help you get them.

## Declaring them

An addon's author adds a `require` line to its `plugin.cfg` or `version.cfg`:

```
require=["owner/repo@v1.0.0", "owner/other"]
```

The existing `deps` spelling remains valid. If both keys are present, `require` wins,
even when its list is empty.

The host defaults to github.com, and the tag is optional. `owner/repo@v1.0.0` means "at
least v1.0.0"; `owner/other` means "any version".

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
