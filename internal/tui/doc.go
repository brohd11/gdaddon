// Package tui is the thin top-level wiring for the interactive bubbletea
// front-end: Run hands bubblestack.Run gdaddon's context, the header + output chrome
// (from appctx / components), and the top-level tab set — each tab a constructor the
// router calls lazily. The real code lives in sub-packages, plus the bubblestack
// framework module.
//
// # Package layout
//
// The framework lives in its own module, github.com/brohd11/bubblestack (developed
// in-tree under ./bubblestack via a replace directive), so it can be reused by
// another tool — it imports no gdaddon package:
//
//	bubblestack/core/        domain-agnostic: the *Shared state (carrying the
//	             consumer's own context in App, recovered typed via App[T], the
//	             spinner/help/task channel, and the optional *Chrome — header closure,
//	             status line, and pluggable Output pane, each independently toggleable
//	             and gateable per-screen), the Router (tea.Model) over a screen stack,
//	             navigation commands (Push/Pop/Replace/ResetToRoot/Refresh), the Screen
//	             interface plus the optional interfaces the router type-asserts
//	             (Filterer, RootHandler, PopStopper, ChromeMasker — a screen suppresses
//	             chrome elements while on top, e.g. FullscreenMask), router-handled
//	             messages (MsgRefresh — one message whose
//	             opaque Target the router only routes, never interprets —, MsgThemeChanged,
//	             and the streaming TaskEvent with an opaque Payload), themes (SetTheme/
//	             ApplyTheme/ThemeNames/CurrentTheme), and generic list/help/style
//	             helpers (NewSelectList, ShortHelp, RenderTitleBar, HeaderBox,
//	             TruncLeft, …). A TabEntry is {Title, New func(*Shared) Screen}: the
//	             router builds each root via New after the theme is applied (so it bakes
//	             the right palette) and re-invokes New on MsgThemeChanged to repaint.
//	bubblestack/components/  reusable, context-agnostic pieces configured by closures
//	             — they name no domain type: the Item list row (carries its own Pick
//	             closure), the screens PickerScreen, ConfirmScreen, LoadingScreen, and
//	             the generic streaming TaskScreen, and LogPane (the default core.Output
//	             chrome). A tab supplies the closures.
//
// The rest is gdaddon's domain front-end, under internal/tui:
//
//	appctx/      the one domain↔framework seam: gdaddon's Ctx (ManifestPath/
//	             ProjectRoot/…) stored on Shared.App (read with appctx.Of), the
//	             Header that renders it, and the RefreshTarget identifiers
//	             (Project/Global/Archive) the tab roots claim. A leaf package so the
//	             tui package and the tabs both read the context without a cycle.
//	flows/…      shared, domain-aware flow screens composed by more than one tab
//	             (so they can't live in any single tab without a cross-tab import):
//	             e.g. flows/newplugin, the Add Plugin form+confirm used by both the
//	             Actions and Search tabs. Unlike components these DO name domain
//	             types; they sit between components and tabs.
//	tabs/…       one package per top-level tab (the domain): its root screen, its
//	             flow screens, and the builders that wire components to features
//	             (e.g. a tab defines its own newInstallConfirm rather than confirm
//	             owning it).
//
// # Self-dispatching list rows (components.Item)
//
// Lists are built from components.Item values, each carrying its own Pick closure
// (and optional Keys). On enter a PickerScreen runs the selected row's Pick, so a
// menu of mixed commands needs no per-row kind enum, no switch, and — for a pushed
// screen — no Update method at all: building the rows is the whole flow. An inert
// row (a placeholder, or a disabled/non-installable entry) is just an Item with a
// nil Pick. A tab root still writes Update (it owns quit-on-q, refresh, the output
// pane), but its enter handler is the same one-liner: it.(components.Item).Pick(sh).
//
// Domain values that are *carried* through a flow rather than rendered (e.g.
// project.versionItem, global.globalItem) stay plain payload structs — they are
// captured inside an Item's Pick closure, not used as the list row themselves.
//
// # Dependency direction
//
// core ← components ← appctx ← flows/* ← tabs/* ← tui (this package). core and
// components name no domain type (the context lives behind Shared.App; Item/loading/
// task/confirm take closures); appctx is the single leaf that binds gdaddon's domain
// to the framework; flows hold domain-aware screens shared by several tabs; tabs do
// not import each other. That acyclic layering is
// what lets the screens live in separate packages — Go forbids import cycles only
// between packages, and the closure + optional-interface inversions remove the
// concrete cross-references that would otherwise straddle a boundary.
//
// # Adding a tab
//
// Add a package under tabs/ whose root implements core.Screen (and RootHandler if
// it reloads on a refresh message), build its rows as components.Item values and
// its sub-flows from the components, and register a {Title, New} TabEntry in the tab
// set in Run — New is a func(*core.Shared) core.Screen that builds the root from its
// own state (read context via appctx.Of(sh)). The router calls New lazily and rebuilds
// it on a theme change, so a root must construct cleanly from sh alone and hold no
// state it can't reproduce. A root that must rebuild after an out-of-band change
// (like Global/Archive after a remove) handles its own refresh message via
// RootHandler, raised with a core.Refresh(target, …) command (target being an appctx
// identifier) — the router routes it to whichever root claims that target.
package tui
