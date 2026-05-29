#!/usr/bin/env bash

GO_DIR="$HOME/dotfiles/.misc/scripts/go/godot_addon_installer"
if [ "$(uname)" = "Darwin" ]; then
    GO_EXE="$GO_DIR/build/mac-arm64/addon_installer"
else
    GO_EXE="$GO_DIR/build/linux/addon_installer"
fi

YAML_FILE="./addon_manifest.yml"
GIT_ROOT=$(git rev-parse --show-toplevel 2>/dev/null)
if [ -z "$GIT_ROOT" ]; then
    echo "Error: Not inside a git repository."
    exit 1
fi

#echo "$GO_EXE" "$YAML_FILE" "$GIT_ROOT"
"$GO_EXE" "$YAML_FILE" "$GIT_ROOT"
