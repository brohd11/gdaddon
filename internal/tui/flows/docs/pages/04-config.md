# Config and sources

gdaddon keeps everything of its own under `~/.gdaddon`, created on first run.

## What's in there

```
~/.gdaddon/
  config/config.yml    archive dir, theme, last search source
  config/sources.yml   search sources and per-host VCS rules
  archive/             downloaded package zips
  plugins.yml          your global addon list
  bin/                 the gdaddon binary, when installed here
```

Both config files are written once, with defaults, and then left alone — they're yours to
edit, and gdaddon reads whatever it finds. The folder is safe to commit: the shipped
`.gitignore` excludes `bin/` (delete that line if you want the binary in there too).

## config.yml

- `archive_dir` — where downloaded package zips are kept. Defaults to `~/.gdaddon/archive`
- `theme` — the color theme. Actions ▸ Theme changes it and writes it here

The archive is what makes a re-install offline-capable: a zip gdaddon has already
downloaded is stored per repo, listed back as an `(archived)` release, and reinstalled
from disk. The Archive tab browses and prunes it.

## sources.yml

Two things live here.

### Search sources

Where the Search tab looks. The Godot Asset Store is configured out of the box; add your
own and it appears alongside it.

### Per-host VCS rules

How to talk to a git host: where its releases live, how to list branches, how to build a
source-archive URL. github.com and codeberg.org ship as defaults, and any host with a
rule gets full release/branch listing. A host *without* one still works — gdaddon falls
back to a plain git clone, it just can't enumerate versions for you.

Branch pinning also comes from these rules (`commit_archive_url` and
`branches.commit_path`). A `sources.yml` written before those keys existed will still
work, but branch packages will float on the branch HEAD instead of pinning a commit.
Delete the file and re-run gdaddon to regenerate it with the current defaults — you'll
lose any edits you made to it, so copy them out first.

## Updating gdaddon itself

Actions ▸ Update gdaddon checks for a newer release and installs it in place. gdaddon
also checks on startup and drops a line in the status bar when one is waiting. A
self-update doesn't change the process already running, so relaunch to pick it up.
