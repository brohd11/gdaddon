// Package tui implements the interactive bubbletea front-end for browsing
// addons, picking a remote version, and installing/updating. It renders state
// from the addon package and turns install progress into bubbletea messages.
//
// # Architecture
//
// The UI is a router (router.go) driving a stack of screens (screen.go). The
// router is the top-level tea.Model: it owns the persistent chrome — header, help
// bar, output/log pane, spinner, terminal size — in a single *shared (shared.go),
// and renders it around whatever screen is on top of the stack. Each screen
// renders only its own body, handles its own keys, and navigates by returning the
// stack commands in nav.go (push / pop / replace / resetToRoot). The router turns
// those into stack operations and routes the task-completion messages (install /
// install-all / archive / reload) to the root browse screen.
//
// Screens come in two kinds:
//
//   - Reusable components, configured by closures rather than subclassed:
//     pickerScreen  (screen_picker.go)  — a list + onSelect/onKey, esc pops
//     confirmScreen (screen_confirm.go) — a y/n box + render/onYes/onToggle
//     taskScreen    (screen_task.go)    — a streaming background task + log
//     loadingScreen (screen_loading.go) — a spinner awaiting a fetch result
//   - Flow screens that wire those components together for one feature:
//     browseScreen, actionsScreen, versionsScreen, newPluginForm, importScreen.
//
// # Adding a flow
//
// To add, say, a new Actions command that needs a submenu and then a confirm,
// reuse the components — don't add a case to an existing screen or a near-copy
// file:
//
//  1. Build the list items (items.go), then a pickerScreen via newPicker with an
//     onSelect closure that returns push(<the confirm or next screen>). See
//     newSubmenuScreen (screen_submenu.go) for the minimal wiring, and
//     archiveKeyHandler for an optional extra-key (onKey) handler.
//  2. Reuse confirmScreen for the confirmation; newInstallConfirm
//     (screen_confirm.go) shows the shape: crumb + render + onYes returning the
//     navigation/commit command.
//  3. From the originating screen, return push(<your picker>). Nothing else
//     changes — no new switch case, no new shared field.
//
// # Why one flat package
//
// The screen graph has back-edges: versionsScreen pushes loadingScreen (to fetch
// branches) and loadingScreen replaces itself with versionsScreen. Splitting
// screens into per-directory Go packages (Cobra-style) would therefore create
// import cycles. Reuse here comes from generic components, not directory nesting,
// so screens stay in package tui — one file per screen or flow.
package tui
