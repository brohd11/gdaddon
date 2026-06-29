#!/usr/bin/env bash
#
# gdaddon installer (macOS + Linux).
#
# Run this from inside the unpacked release zip — it installs the `gdaddon`
# binary sitting next to this script. It prompts for one of three destinations:
#
#   1) system  — /usr/local/bin           (on PATH by default, needs sudo)
#   2) user    — ~/.local/bin             (no sudo, may need PATH setup)
#   3) gdaddon — ~/.gdaddon/bin           (no sudo, not on PATH; for the Godot plugin)
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN="$SCRIPT_DIR/gdaddon"

if [ ! -x "$BIN" ]; then
    echo "error: $BIN not found or not executable" >&2
    echo "run this script from the unpacked release zip (it installs the gdaddon next to it)" >&2
    exit 1
fi

cat <<'EOF'
Where should gdaddon be installed?

  1) system   /usr/local/bin       on PATH by default, requires sudo
  2) user     ~/.local/bin         no sudo, may need PATH setup
  3) gdaddon  ~/.gdaddon/bin        no sudo, not on PATH (launched by the Godot plugin)

EOF

read -r -p "Choose [1/2/3]: " choice

install_to() {
    # install_to <dest_dir> [use_sudo]
    local dest_dir="$1" use_sudo="${2:-}"
    if [ -n "$use_sudo" ]; then
        sudo mkdir -p "$dest_dir"
        sudo cp "$BIN" "$dest_dir/gdaddon"
        sudo chmod +x "$dest_dir/gdaddon"
    else
        mkdir -p "$dest_dir"
        cp "$BIN" "$dest_dir/gdaddon"
        chmod +x "$dest_dir/gdaddon"
    fi
}

case "$choice" in
    1)
        DEST="/usr/local/bin"
        if [ -w "$DEST" ]; then
            install_to "$DEST"
        else
            echo "writing to $DEST (sudo)..."
            install_to "$DEST" sudo
        fi
        echo "installed to $DEST/gdaddon — run: gdaddon"
        ;;
    2)
        DEST="$HOME/.local/bin"
        install_to "$DEST"
        echo "installed to $DEST/gdaddon"
        case ":$PATH:" in
            *":$DEST:"*)
                echo "run: gdaddon"
                ;;
            *)
                # Pick the profile that matches the current shell.
                case "${SHELL##*/}" in
                    zsh)  PROFILE="$HOME/.zshrc" ;;
                    bash) PROFILE="$HOME/.bashrc" ;;
                    *)    PROFILE="$HOME/.profile" ;;
                esac
                echo
                echo "$DEST is not on your PATH. Add it with:"
                echo "  echo 'export PATH=\"$DEST:\$PATH\"' >> $PROFILE"
                echo "then restart your shell (or 'source $PROFILE')."
                ;;
        esac
        ;;
    3)
        DEST="$HOME/.gdaddon/bin"
        install_to "$DEST"
        echo "installed to $DEST/gdaddon"
        echo "this location is intentionally not on PATH — the Godot plugin launches it"
        echo "directly, or run it with the full path: $DEST/gdaddon"
        ;;
    *)
        echo "error: invalid choice '$choice'" >&2
        exit 1
        ;;
esac
