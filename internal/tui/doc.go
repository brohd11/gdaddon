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
//	             navigation commands that return a core.Action (Push/Pop/PopTo —
//	             unwind to the nearest PopStopper — /Replace/ResetToRoot/ShowTab, plus
//	             Seq to issue several at once), the Screen
//	             interface plus the optional interfaces the router type-asserts
//	             (Filterer, Receiver, PopStopper, ChromeMasker — a screen suppresses
//	             chrome elements while on top, e.g. FullscreenMask; Crumber — a screen
//	             contributes one segment to the router-drawn breadcrumb bar under the
//	             tab strip, built fresh each frame from the live stack root→top, see
//	             RenderBreadcrumb; Overlayer — a popup the router draws *over* the
//	             screen below it, see "Overlays" below). A Screen's Update
//	             returns (Screen, core.Action): the Action bundles a control message
//	             the router applies to the stack synchronously this same tick (no command
//	             round-trip) and a genuine async tea.Cmd for bubbletea (Async wraps a
//	             cmd-only Action; the zero Action does nothing).
//	             Router-handled messages include the PropagateAll broadcast — one message
//	             whose opaque payload the router only routes to every Receiver (tab roots,
//	             deeper screens, and the consumer's App), never interprets — and the
//	             streaming TaskEvent with an opaque Payload. A theme switch rides this
//	             broadcast: ApplyTheme raises PropagateAll(MsgThemeChanged{}), the App's
//	             Receive returns RefreshRoots(), and the router rebuilds the cached roots.
//	             Themes (SetTheme/
//	             ApplyTheme/ThemeNames/CurrentTheme), and generic list/help/style
//	             helpers (NewSelectList, ShortHelp, RenderTitleBar, HeaderBox,
//	             TruncLeft, …). A TabEntry is {Title, New func(*Shared) Screen}: the
//	             router builds each root via New after the theme is applied (so it bakes
//	             the right palette) and re-invokes New on RefreshRoots to repaint.
//	bubblestack/components/  reusable, context-agnostic pieces configured by closures
//	             — they name no domain type: the Item list row (carries its own Pick
//	             closure), the screens PickerScreen, DialogScreen (a confirm box, or a
//	             modal overlay when its Overlay flag is set — see "Overlays" below),
//	             LoadingScreen, the field-focused FormScreen, and the generic
//	             streaming TaskScreen, and LogPane (the default core.Output chrome). A tab
//	             supplies the closures. The two screens that run background work —
//	             TaskScreen (a streaming task) and LoadingScreen (a fetch spinner) — each
//	             own a context.WithCancel and let esc abort the in-flight work: the work
//	             closure (TaskScreen's RunFunc, LoadingScreen's Run) takes that ctx as its
//	             first arg, so a cancellable closure threads it into its network/process
//	             call; the screen then unwinds (TaskScreen lingers on an "aborted" log,
//	             LoadingScreen pops back).
//
// The rest is gdaddon's domain front-end, under internal/tui:
//
//	appctx/      the one domain↔framework seam: gdaddon's Ctx (ManifestPath/
//	             ProjectRoot/…) stored on Shared.App (read with appctx.Of), the
//	             Header that renders it, the tab titles, and the Dirty notification
//	             payloads (ProjectDirty/GlobalDirty/ArchiveDirty) the tab roots react
//	             to. A leaf package so the tui package and the tabs both read the
//	             context without a cycle.
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
// (and optional Keys). Pick returns a core.Action: a control message the router
// applies synchronously (e.g. core.Push) and/or an async cmd. On enter a PickerScreen
// runs the selected row's Pick, so a menu of mixed commands needs no per-row kind enum,
// no switch, and — for a pushed screen — no Update method at all: building the rows is
// the whole flow. An inert row (a placeholder, or a disabled/non-installable entry) is
// just an Item with a nil Pick. A tab root still writes Update (it owns quit-on-q,
// notifications, the output pane), but it just forwards components.RootUpdate's pair.
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
// # Overlays (popups / modals)
//
// Most screens replace the one below them; a screen that implements core.Overlayer
// is instead drawn *on top of* it. The router renders the below-screen's full frame
// (chrome + body) as the background, then core.Composite splices the overlay's box
// (View) centered over it — so the underlying screen stays visible around the box.
// Composite is display-cell aware (it slices via x/ansi, bracketing the box with
// resets) so it doesn't corrupt the background's ANSI styling. Input is unchanged:
// the router still dispatches only to the top screen, so an overlay is naturally
// modal, and the background is kept sized (for resize) while it shows through. The
// router honors core.Overlayer's bool each frame, so a screen can opt in yet still
// render full-screen by returning false. components.DialogScreen does exactly that:
// the same closure-configured box serves a full-screen confirm and, when its Overlay
// flag is set, a modal (core.PopupBox renders it in the theme accent, with its key
// hints inside the box since the router keeps the background's help bar).
// See components.DialogScreen (the Overlay flag) and components.CreatePopup for
// the popup machinery.
//
// # Adding a tab
//
// Add a package under tabs/ whose root implements core.Screen (and Receiver if
// it reloads on a notification), build its rows as components.Item values and
// its sub-flows from the components, and register a {Title, New} TabEntry in the tab
// set in Run — New is a func(*core.Shared) core.Screen that builds the root from its
// own state (read context via appctx.Of(sh)). The router calls New lazily and rebuilds
// it on a theme change, so a root must construct cleanly from sh alone and hold no
// state it can't reproduce. A root that must rebuild after an out-of-band change
// (like Global/Archive after a remove) implements core.Receiver: core.PropagateAll(payload)
// broadcasts an appctx Dirty payload (a bare reload marker) to every root, and the root
// that recognizes it reloads itself. The visible outcome — the status line and any focus
// switch — is composed at the call site instead of riding the payload: wrap core.SetStatus,
// the core.PropagateAll, core.ShowTab(title), and any async cmd in one core.Seq, which the
// router applies in order (its seqMsg drains both the control and async lanes of every
// child Action). The router interprets neither the payload nor the composed outcome.
package tui
