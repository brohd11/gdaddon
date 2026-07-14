# Dependencies

An addon can declare that it needs other addons, and gdaddon will help you get them.

## Declaring them

An addon's author adds a `deps` line to its `plugin.cfg`:

```
deps=["owner/repo@v1.0.0", "owner/other"]
```

The host defaults to github.com, and the tag is optional. `owner/repo@v1.0.0` means "at
least v1.0.0"; `owner/other` means "any version".

Matching is against the entry's `tag` — the release identity — not its `version`, since
the version string in a `plugin.cfg` is the author's to invent and often disagrees with
the release it shipped in. Comparison is semver `>=`, so a newer tag satisfies a dep.

## The Dependencies screen

Any installed addon that declares deps grows a Dependencies action. It lists every
declared dep with its status:

- `installed` — present and satisfying the version
- `not installed` — in your manifest, but not on disk yet
- `missing` — not in the manifest at all
- `outdated` — installed, but older than the dep asks for
- `suppressed` — declared, but you've told gdaddon to ignore it

From that screen, Add all missing writes the manifest-absent deps into the manifest
(pinned to the requested tag, or untagged when the dep didn't ask for one). It only adds
them — Install All then installs them. A submenu on any single dep adds just that one.

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
