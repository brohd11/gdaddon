#!/usr/bin/env bash
# Thin launcher: run the gdaddon binary's built-in installer from this dir.
# (macOS/Linux. Double-click install.command on macOS, or run this in a terminal.)
cd "$(dirname "${BASH_SOURCE[0]}")"
./gdaddon install
