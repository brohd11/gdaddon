# Getting started

gdaddon installs and tracks Godot addons from a manifest file kept in your project.

## The idea

A Godot project accumulates addons: a zip from the Asset Store here, a git clone there,
a submodule someone added two years ago. Nothing records where any of them came from or
what version they are.

gdaddon puts that in one file — `addon_manifest.yml` — next to your project. Each entry
says where an addon comes from and which version is pinned. From there, installing every
addon on a fresh checkout is one action, and so is updating them.

## Starting up

Run `gdaddon` in your project (or `gdaddon /path/to/project`). It finds the project root
from git, then looks for an `addon_manifest.yml` underneath it.

No manifest yet? Actions ▸ Create manifest writes one. The row disappears once it exists.

## The tabs

- Project — the addons in this project's manifest, with their install state
- Global — your personal list of addons, to pull into any project
- Sets — named groups of addons you install together
- Archive — package zips gdaddon has downloaded, reusable offline
- Actions — everything that acts on the whole project, plus these docs
- Search — the Godot Asset Store and any source you configure

Move between tabs with `[` and `]`. `q` quits, `?` expands the help bar, `o` opens the
output pane (`w` wraps its long lines), and `esc` steps back out of any screen.

## Installing your first addon

Search for an addon, or add one by URL from Actions ▸ New Plugin. Either way it lands in
the manifest. Open it on the Project tab, pick a version, and install.

A row's state tells you where it stands:

- `missing` — in the manifest, not on disk
- `installed` — on disk, and the version matches what's pinned
- `mismatch` — on disk, but a different version than the pin
- `unversioned` — on disk, no version recorded to compare
- `branch changed` — a git checkout sitting on a different branch than the one recorded

Actions ▸ Install/Update All does the whole manifest at once — that's the fresh-checkout
path. It never touches a git clone or submodule you already have; those are yours.

## Addons you develop

An addon installed as a clone or submodule is a live git checkout — you edit it in place,
maybe from two projects at once. The Project tab keeps an eye on those:

- `t` opens a terminal at the highlighted addon's folder, for the git work gdaddon doesn't do
- `f` runs `git fetch` in every checkout in the manifest, then refreshes

Rows then mark what needs attention: `uncommitted changes`, `ahead 2` (committed but not
pushed), `behind origin 3` (someone pushed and you haven't pulled). The ahead/behind counts
are read straight from git, so they're only as current as your last fetch — that's what `f`
is for. gdaddon never fetches on its own, since it's the one thing here that goes to the
network without being asked.

Open a checkout and pick **Git** for the rest of the round-trip: status, fetch, pull, push,
and commit, each streaming git's own output to the log. It's not a git client, and doesn't
try to be — the operations that need a decision from you refuse instead of guessing. Pull is
`--ff-only`, so a branch that has diverged aborts having changed nothing rather than dropping
you into a merge conflict inside a TUI. When something fails, git says why in the log and you
go sort it out in a terminal.

Commit asks for a message and what to stage, because git's own default is a trap: `-a` stages
changes to files git already tracks, so a script you *just created* is untracked and would
miss the commit. The confirm screen lists what's going in — and names the new files being left
out, so you can flip to "all, incl. new files" if that's what you meant.

## Where to next

- Manifest format — what the entries actually mean
- Dependencies — addons that declare they need other addons
- Config and sources — `~/.gdaddon`, themes, and adding your own hosts
