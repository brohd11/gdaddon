#!/usr/bin/env bash

mkdir -p "$HOME/.local/bin"

GO_DIR="$HOME/main/go/godot_addon_installer"
if [ "$(uname)" = "Darwin" ]; then
    GO_EXE="$GO_DIR/build/mac-arm64/gdaddon"
else
    GO_EXE="$GO_DIR/build/linux/gdaddon"
fi

ln -sf "$GO_EXE" "$HOME/.local/bin/gdaddon"