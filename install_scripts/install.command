#!/usr/bin/env bash
# Double-clickable launcher (macOS): Finder opens this in Terminal and runs the
# gdaddon binary's built-in installer from this folder.
cd "$(dirname "$0")"
./gdaddon install
