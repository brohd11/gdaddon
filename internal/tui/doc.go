// Package tui is the thin top-level wiring for the interactive bubbletea
// front-end: Run builds the shared chrome state and the top-level tab set, then
// hands them to the router. The real code lives in sub-packages.
//
// # Package layout
//
//	core/        the framework: the *Shared chrome state, the Router (tea.Model)
//	             over a screen stack, navigation commands (Push/Pop/Replace/
//	             ResetToRoot/RootRefresh), the Screen interface plus the optional
//	             interfaces the router type-asserts (Filterer, OutputViewer,
//	             RootHandler), router-handled messages (MsgRootRefresh,
//	             MsgGlobalRefresh, InstallEvent), and generic list/help/style
//	             helpers (NewSelectList, RootHelp, RenderTitleBar, …).
//	components/  reusable, context-agnostic screens configured by closures — they
//	             name no domain type: PickerScreen, ConfirmScreen, LoadingScreen,
//	             and the generic streaming TaskScreen. A tab supplies the closures.
//	tabs/…       one package per top-level tab (the domain): its root screen, its
//	             flow screens, and the builders that wire components to features
//	             (e.g. a tab defines its own newInstallConfirm rather than confirm
//	             owning it).
//
// # Dependency direction
//
// core ← components ← tabs/* ← tui (this package). core names no concrete screen
// (the router reaches the browse root via the RootHandler interface); components
// name no domain type (loading/task/confirm take closures); tabs do not import
// each other. That acyclic layering is
// what lets the screens live in separate packages — Go forbids import cycles only
// between packages, and the closure + optional-interface inversions remove the
// concrete cross-references that would otherwise straddle a boundary.
//
// # Adding a tab
//
// Add a package under tabs/ whose root implements core.Screen (and RootHandler if
// it shows addon results), build its flow with the components, and register it in
// the tab set in Run.
package tui
