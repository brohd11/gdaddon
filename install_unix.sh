#!/usr/bin/env bash

cd "$(dirname "${BASH_SOURCE[0]}")"

mkdir -p "$HOME/.local/bin"

# Absolute path to the repo so the symlink target doesn't dangle.
GO_DIR="$(pwd)"
# The makefile lays builds out as build/<GOOS>-<GOARCH>/, and `make` builds the host
# target only -- so ask the toolchain rather than assuming darwin means arm64.
GO_EXE="$GO_DIR/build/$(go env GOOS)-$(go env GOARCH)/gdaddon"

if [ ! -x "$GO_EXE" ]; then
    echo "error: $GO_EXE not found or not executable — run 'make' first" >&2
    exit 1
fi

ln -sf "$GO_EXE" "$HOME/.local/bin/gdaddon"