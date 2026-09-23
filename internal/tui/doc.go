// Package tui wires gdaddon's interactive front-end: Run passes bubblestack.Run the
// context (appctx), header and output chrome, and the tabs. Layers depend in one
// direction: appctx ← flows/* (screens shared by several tabs) ← tabs/* (one package per
// tab) ← tui.
package tui
