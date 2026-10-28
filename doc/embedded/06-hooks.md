# Git action hooks

Run your own commands before and after Git actions.

At startup, gdaddon creates `~/.gdaddon/hooks/config.yml` if it is missing,
including the directory. It contains every supported event as an empty
list and a commented command example. No commands run until you add them.
Existing files are left untouched.

The apps use separate files: repoview reads `~/.repoview/hooks/config.yml`
and gdaddon reads `~/.gdaddon/hooks/config.yml`. Config is re-read for each
action. Scripts can live beside it, or anywhere else you choose.

```yaml
hooks:
  pre_tag:
    - command: python
      args: ["~/.gdaddon/hooks/check-plugin-version.py"]
    - command: go
      args: ["test", "./..."]
  post_push:
    - command: python
      args: ["~/.gdaddon/hooks/notify-push.py"]
```

These script names are examples: provide your own scripts and choose an
interpreter installed on your machine. For PowerShell, for example:

```yaml
hooks:
  pre_tag:
    - command: powershell
      args: ["-NoProfile", "-File", "~/.gdaddon/hooks/check-version.ps1"]
```

## Events and order

Each of these actions supports both a `pre_` and a `post_` event:

| Action | Events |
| --- | --- |
| Fetch | `pre_fetch`, `post_fetch` |
| Pull | `pre_pull`, `post_pull` |
| Push branch or tag | `pre_push`, `post_push` |
| Commit | `pre_commit`, `post_commit` |
| Create tag | `pre_tag`, `post_tag` |
| Delete local tag | `pre_delete_tag`, `post_delete_tag` |

Commands run in list order. Exit 0 continues; any other exit or a command
that cannot start stops the sequence. A failed pre-hook prevents the Git
action. Post-hooks run only after Git succeeds. A failed post-hook is
reported as a hook failure **after the Git action completed**; Git's
changes remain in place. Later commands in that event are skipped.

Commit's pre-hooks run before staging, including before `git add -A`.
Tag creation tags the current commit: changing `plugin.cfg` in a hook does
not put those changes into the tagged commit. A version-check script can
reject a mismatch and ask you to update and commit the version first.

Hooks apply to the app's Git menu, tag actions, batch actions, and quick
fetch. A batch failure is reported for that repo and the batch continues
with the next repo. Concurrent fetches can run hooks in several repos at
once; their output is collected and logged per repo.

Status, diff, log, and listing remote tags do not trigger hooks.
Commands run through `gdaddon repos -- ...` or a terminal use their own
behavior; this hook system wraps the app's Git actions. Git's native hooks
continue to work as usual.

## Command contract

The executable and its arguments are passed separately, without a shell
command string. Each argument stays a single argument, even with spaces.
Pipes, wildcards, redirects, and environment variables inside arguments
are literal text. To run a script, name its interpreter as `command`.

A leading `~/` or `~\\` in the command or an argument expands to your
home directory. Bare executable names use PATH. Relative paths resolve
from the target repository root, which is also the command's working
directory. Use a home path for scripts beside this config file.

Commands inherit the app's environment plus:

| Variable | Value |
| --- | --- |
| `RM_REPO_ROOT` | Absolute path of the target checkout |
| `RM_REPO_NAME` | Checkout directory's base name |
| `RM_OPERATION` | `fetch`, `pull`, `push`, `commit`, `tag`, or `delete_tag` |
| `RM_HOOK` | Event name, such as `pre_tag` |
| `RM_TAG` | Tag being created, pushed, or deleted; empty for other actions |
| `RM_VERSION` | `RM_TAG` with one leading lowercase `v` removed |

For a branch push, `RM_TAG` and `RM_VERSION` are empty. A push of
`v1.4.0` sets them to `v1.4.0` and `1.4.0`. Version checking, including
which repos and config files to check, belongs to your command. Return 0
to skip repos where your check does not apply.

Both stdout and stderr appear in the operation log. Cancelling an action
cancels its running command. Fetch-all's existing timeout also bounds its
hooks. Commands receive no interactive terminal input.

Malformed YAML, unknown events or fields, and missing commands abort the
action with the config path in the error. Use empty event lists or
`hooks: {}` to disable all hooks. Deleting the file restores the empty
defaults at the next startup.
