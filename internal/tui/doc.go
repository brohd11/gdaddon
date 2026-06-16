// Package tui is the thin top-level wiring for the interactive bubbletea
// front-end: Run builds the shared chrome state and the top-level tab set, then
// hands them to the router. The real code lives in sub-packages.
//
// # Package layout
//
//	core/        the framework: the *Shared chrome state, the Router (tea.Model)
//	             over a screen stack, navigation commands (Push/Pop/Replace/
//	             ResetToRoot/RootRefresh/GlobalRefresh/ArchiveRefresh), the Screen
//	             interface plus the optional interfaces the router type-asserts
//	             (Filterer, OutputViewer, RootHandler, PopStopper), router-handled
//	             messages (MsgRefresh — one message targeting any tab's root —
//	             and InstallEvent), and generic list/help/style helpers (NewSelectList,
//	             RootHelp, RenderTitleBar, …).
//	components/  reusable, context-agnostic pieces configured by closures — they
//	             name no domain type: the Item list row (carries its own Pick
//	             closure) and the screens PickerScreen, ConfirmScreen, LoadingScreen,
//	             and the generic streaming TaskScreen. A tab supplies the closures.
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
// core ← components ← tabs/* ← tui (this package). core names no concrete screen
// (the router reaches the browse root via the RootHandler interface); components
// name no domain type (Item/loading/task/confirm take closures); tabs do not import
// each other. That acyclic layering is
// what lets the screens live in separate packages — Go forbids import cycles only
// between packages, and the closure + optional-interface inversions remove the
// concrete cross-references that would otherwise straddle a boundary.
//
// # Adding a tab
//
// Add a package under tabs/ whose root implements core.Screen (and RootHandler if
// it reloads on a refresh message), build its rows as components.Item values and
// its sub-flows from the components, and register it in the tab set in Run. A root
// that must rebuild after an out-of-band change (like Global/Archive after a
// remove) handles its own refresh message via RootHandler, raised with a
// core.*Refresh command — the router routes it to whichever root claims it.
package tui
