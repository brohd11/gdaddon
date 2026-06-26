#!/usr/bin/env bash

cd "$(dirname "${BASH_SOURCE[0]}")"

mkdir -p "$HOME/.local/bin"

# Absolute path to the repo so the symlink target doesn't dangle.
GO_DIR="$(pwd)"
if [ "$(uname)" = "Darwin" ]; then
    GO_EXE="$GO_DIR/build/mac-arm64/gdaddon"
else
    GO_EXE="$GO_DIR/build/linux/gdaddon"
fi

if [ ! -x "$GO_EXE" ]; then
    echo "error: $GO_EXE not found or not executable — run 'make' first" >&2
    exit 1
fi

ln -sf "$GO_EXE" "$HOME/.local/bin/gdaddon"