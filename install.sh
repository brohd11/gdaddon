#!/bin/sh
# Install gdaddon into ~/.local/bin.
#
#   curl -fsSL https://raw.githubusercontent.com/brohd11/gdaddon/main/install.sh | sh
#
# Env overrides:
#   BIN_DIR=/usr/local/bin   install target   (default: ~/.local/bin)
#   VERSION=v0.3.0           pin a release    (default: latest)
#   --no-modify-path         never touch shell rc files
#
# Body below "end config" is shared with ~/main/go/install.template.sh -- to update,
# chop at that line and paste the current template body.

set -eu

# ---- config ----
REPO="brohd11/gdaddon"
BINARY="gdaddon"
ARCHIVE_EXT="zip"
SUPPORTED="darwin-arm64 darwin-amd64 linux-amd64 linux-arm64"

# Printed after a successful install. Leave the body as ':' for nothing.
post_install_note() {
  cat <<EOF
Run '$BINARY' in a Godot project to manage its addons.
Later updates: '$BINARY self-update'.

To also place a copy where the Godot EditorPlugin looks for it (~/.gdaddon/bin):
  $BINARY install --dest home
EOF
}
# ---- end config ----

BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
VERSION="${VERSION:-latest}"

# auto   = prompt when a tty is available, otherwise just print the line
# never  = --no-modify-path, never touch dotfiles
# always = --modify-path, write without prompting even with no tty (for env
#          setup scripts, which have no terminal to answer a prompt with)
PATH_MODE=auto

for arg in "$@"; do
  case "$arg" in
    --no-modify-path) PATH_MODE=never ;;
    --modify-path) PATH_MODE=always ;;
    -h|--help)
      # Not derived from $0: under `curl | sh` there is no script path to read.
      cat <<EOF
install $BINARY into \$BIN_DIR (default: \$HOME/.local/bin)

  BIN_DIR=<dir>       install target
  VERSION=<tag>       pin a release (default: latest)
  --modify-path       update the shell rc file without prompting
  --no-modify-path    never touch shell rc files
EOF
      exit 0 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

die() { echo "error: $*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

# --- detect platform ---------------------------------------------------------

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)

case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) die "unsupported architecture: $arch" ;;
esac

case "$os" in
  darwin|linux) ;;
  *) die "unsupported OS: $os (Windows: download the .zip from https://github.com/$REPO/releases)" ;;
esac

target="$os-$arch"

# Word-match against SUPPORTED so a missing build fails here with a useful message
# rather than as a 404 later.
case " $SUPPORTED " in
  *" $target "*) ;;
  *) die "no $target build is published for $BINARY
  supported: $SUPPORTED
  build from source: https://github.com/$REPO" ;;
esac

asset="$BINARY-$target.$ARCHIVE_EXT"

# --- resolve URL -------------------------------------------------------------

# Asset names are deliberately version-less so the /latest/download redirect works
# and we never touch the GitHub API (no JSON parsing, no rate limit).
if [ "$VERSION" = latest ]; then
  url="https://github.com/$REPO/releases/latest/download/$asset"
else
  url="https://github.com/$REPO/releases/download/$VERSION/$asset"
fi

have curl || die "curl is required"
case "$ARCHIVE_EXT" in
  zip) have unzip || die "unzip is required to install $BINARY" ;;
  tar.gz) have tar || die "tar is required" ;;
  *) die "bad ARCHIVE_EXT in this script: $ARCHIVE_EXT" ;;
esac

# --- install -----------------------------------------------------------------

mkdir -p "$BIN_DIR" || die "cannot create $BIN_DIR"
[ -w "$BIN_DIR" ] || die "$BIN_DIR is not writable (set BIN_DIR to somewhere you own)"

echo "downloading $BINARY ($target, $VERSION)"

# Stage in a temp dir so a failed download can't leave a half-written binary in
# place of a working one.
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

# Download and extract as separate steps rather than piping curl into the
# extractor: in a POSIX pipeline only the last command's status is visible, and
# both tar and unzip can exit 0 on empty input, so a 404 from curl would surface
# as a bogus "bad archive" error.
curl -fsSL -o "$tmp/$asset" "$url" \
  || die "download failed: $url
  (check https://github.com/$REPO/releases for available versions)"

case "$ARCHIVE_EXT" in
  zip) unzip -q -o "$tmp/$asset" -d "$tmp" || die "could not extract $asset" ;;
  tar.gz) tar -xzf "$tmp/$asset" -C "$tmp" || die "could not extract $asset" ;;
esac

[ -f "$tmp/$BINARY" ] || die "archive did not contain $BINARY"

chmod +x "$tmp/$BINARY"
mv -f "$tmp/$BINARY" "$BIN_DIR/$BINARY"

installed="$BIN_DIR/$BINARY"
echo "installed -> $installed"

# --- PATH --------------------------------------------------------------------

export_line="export PATH=\"\$PATH:$BIN_DIR\""

# Keyed on the install directory, NOT on $BINARY: several of these installers share
# one BIN_DIR, and a per-binary marker meant the second one appended a duplicate
# `export PATH` line for a directory that was already handled.
marker="# added by install.sh -- $BIN_DIR on PATH"

shell_name=$(basename "${SHELL:-}")

# Is `export VAR=value` valid syntax in the user's login shell? fish and csh/tcsh
# use their own forms, so printing a POSIX export line at them is worse than saying
# nothing. Unknown shells are treated as non-POSIX -- better to describe the goal
# than to emit syntax that may not parse.
case "$shell_name" in
  zsh|bash|sh|ksh|ksh93|mksh|dash|ash) posix_syntax=1 ;;
  *) posix_syntax=0 ;;
esac

# The rc file to append to, or "" when we don't know one. Empty is not the same as
# "non-POSIX": ksh and dash take the export line fine, but their startup file is
# $ENV-dependent and not ours to guess, so we print and never write.
rc_file() {
  case "$shell_name" in
    zsh)
      # zsh reads .zshenv/.zprofile/.zshrc independently -- there is no first-match
      # chain, so creating .zshrc cannot shadow another file.
      echo "$HOME/.zshrc" ;;
    bash)
      if [ "$os" = darwin ]; then
        # macOS terminals start LOGIN shells, and bash sources only the FIRST of
        # .bash_profile / .bash_login / .profile that exists. Creating .bash_profile
        # for someone whose setup lives in .profile would silently stop .profile from
        # ever being read again -- so prefer whichever already exists, and only create
        # .bash_profile when none of the three do (nothing to shadow in that case).
        for cand in "$HOME/.bash_profile" "$HOME/.bash_login" "$HOME/.profile"; do
          [ -f "$cand" ] && { echo "$cand"; return; }
        done
        echo "$HOME/.bash_profile"
      else
        # Linux terminals start non-login interactive shells, which read .bashrc.
        echo "$HOME/.bashrc"
      fi ;;
    *) echo "" ;;
  esac
}

# Print the manual instructions, tailored to what the shell can actually use.
print_manual() {
  if [ "$posix_syntax" -eq 1 ]; then
    echo "add it with:"
    echo "  $export_line"
  else
    echo "your shell (${shell_name:-unknown}) uses different syntax for this --"
    echo "add this directory to your PATH using your shell's own mechanism:"
    echo "  $BIN_DIR"
  fi
}

add_to_path() {
  rc=$1
  # Match the directory anywhere in the file, not just our marker: the line may have
  # been added by a previous run of a different installer sharing this BIN_DIR, or by
  # the user, or by another tool. A false positive here just means we skip the append
  # and print the line instead, which is the safe direction to be wrong in.
  if [ -f "$rc" ] && grep -qF "$BIN_DIR" "$rc"; then
    echo "$rc already references $BIN_DIR -- leaving it alone"
    return
  fi

  printf '\n%s\n%s\n' "$marker" "$export_line" >> "$rc" || die "could not write to $rc"
  echo "added to $rc -- open a new shell, or run this in the current one:"
  echo "  $export_line"
}

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *)
    echo
    echo "$BIN_DIR is not on your PATH, so '$BINARY' won't be runnable by name."
    # Read from the terminal, not stdin: under `curl | sh` stdin is the script
    # itself, so a bare `read` would silently consume the remaining source.
    #
    # Probe by actually opening /dev/tty rather than testing -e/-r. The device node
    # can exist and look readable while open(2) still fails with ENXIO ("Device not
    # configured") when the process has no controlling terminal -- which is the norm
    # in CI and under some agent/daemon runners. Testing first avoids printing a
    # prompt we can't answer, and keeps the open error off the user's screen.
    #
    # The probe must run in a subshell, NOT as `exec 3</dev/tty`. exec is a POSIX
    # special builtin, so a failed redirection on it terminates a non-interactive
    # shell outright -- dash exits 2 and skips everything below, while bash and zsh
    # carry on. dash is /bin/sh on Debian and Ubuntu, so the exec form breaks exactly
    # the audience this installer targets. A subshell confines the failure.
    # Resolve the rc file up front: with no known one there is nothing to offer,
    # so print the line rather than prompting and then refusing the answer.
    rc=$(rc_file)

    # Dotfiles are often symlinks into a managed dotfiles repo. Appending follows
    # the link, which is what the user wants -- but say so before they answer, so
    # nobody is surprised by a dirty repo. readlink is not POSIX; guard it.
    if [ -n "$rc" ] && [ -L "$rc" ]; then
      link_target=$(readlink "$rc" 2>/dev/null || echo "?")
      echo "note: $rc is a symlink -> $link_target"
    fi

    if [ "$PATH_MODE" = never ] || [ -z "$rc" ]; then
      print_manual
    elif [ "$PATH_MODE" = always ]; then
      add_to_path "$rc"
    elif (: < /dev/tty) 2>/dev/null; then
      printf 'Add it to %s? [y/N] ' "$rc"
      reply=""
      # `read` is not a special builtin, so a redirection failure here fails only
      # the command, and the || keeps us going.
      read -r reply < /dev/tty || reply=""
      case "$reply" in
        [yY]|[yY][eE][sS]) add_to_path "$rc" ;;
        *) echo "skipped."; print_manual ;;
      esac
    else
      # Non-interactive (CI, piped with no tty) and not explicitly asked via
      # --modify-path: never edit dotfiles unasked.
      print_manual
    fi
    ;;
esac

echo
post_install_note
