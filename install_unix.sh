#!/usr/bin/env bash

mkdir -p "$HOME/.local/bin"

GO_DIR="$HOME/main/go/godot_addon_installer"
if [ "$(uname)" = "Darwin" ]; then
    GO_EXE="$GO_DIR/build/mac-arm64/gdutil"
else
    GO_EXE="$GO_DIR/build/linux/gdutil"
fi

ln -sf "$GO_EXE" "$HOME/.local/bin/gdutil"