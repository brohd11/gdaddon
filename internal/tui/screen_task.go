package tui

import (
	"fmt"
	"strings"

	"gdaddon/internal/addon"
	"gdaddon/internal/archive"
	"gdaddon/internal/source"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type taskKind int

const (
	taskInstall taskKind = iota
	taskInstallAll
	taskArchive
)

// taskScreen runs a streaming background task (install / install-all / archive),
// piping progress into the shared output pane. Install and install-all unwind to
// browse on completion; archive stays on screen until dismissed, then re-lists
// the versions screen.
type taskScreen struct {
	kind       taskKind
	label      string
	done       bool
	bodyHeight int

	// install payload
	selected      addon.Addon
	selectedLocal string
	pick          versionItem

	// archive payload
	repoID string
	tag    string
	assets []source.Asset
}

var _ outputViewer = (*taskScreen)(nil)

func newInstallTask(selected addon.Addon, local string, pick versionItem) *taskScreen {
	return &taskScreen{kind: taskInstall, label: "installing " + selected.Name + "…", selected: selected, selectedLocal: local, pick: pick}
}

func newInstallAllTask() *taskScreen {
	return &taskScreen{kind: taskInstallAll, label: "installing all addons…"}
}

func newArchiveTask(selected addon.Addon, tag, repoID string, assets []source.Asset) *taskScreen {
	return &taskScreen{kind: taskArchive, label: "archiving " + tag + "…", selected: selected, repoID: repoID, tag: tag, assets: assets}
}

func (s *taskScreen) wantsOutput() bool { return true }

func (s *taskScreen) Init(sh *shared) tea.Cmd {
	switch s.kind {
	case taskInstall:
		target := addon.Addon{Name: s.selected.Name, URL: s.pick.asset.URL, Path: s.selected.Path}
		root := sh.projectRoot
		return startTask(sh, func(report addon.Reporter, done chan<- installEvent) {
			res, err := addon.Install(target, root, report)
			done <- installEvent{done: true, err: err, path: res.Path, version: res.Version}
		})
	case taskInstallAll:
		mp, pr := sh.manifestPath, sh.projectRoot
		return startTask(sh, func(report addon.Reporter, done chan<- installEvent) {
			statuses, err := addon.Inspect(mp, pr)
			if err != nil {
				report("error: %v", err)
			} else {
				_ = addon.InstallAll(mp, statuses, pr, report)
			}
			done <- installEvent{done: true}
		})
	case taskArchive:
		repoID, tag, assets := s.repoID, s.tag, s.assets
		return startTask(sh, func(report addon.Reporter, done chan<- installEvent) {
			for _, a := range assets {
				report("downloading %s …", strings.TrimSuffix(a.Name, " - archived"))
				if err := archive.Archive(repoID, tag, a); err != nil {
					done <- installEvent{done: true, err: err}
					return
				}
			}
			done <- installEvent{done: true}
		})
	}
	return nil
}

func (s *taskScreen) Update(sh *shared, msg tea.Msg) (screen, tea.Cmd) {
	switch msg := msg.(type) {
	case installEvent:
		if !msg.done {
			sh.appendLog(msg.line)
			return s, waitForEvent(sh.events)
		}
		switch s.kind {
		case taskArchive:
			// Stay on the log screen so the result (or error) is readable; the user
			// dismisses with esc (handled below).
			s.done = true
			if msg.err != nil {
				sh.appendLog("archive failed: " + msg.err.Error())
			} else {
				sh.appendLog("archived " + s.tag)
			}
			return s, nil
		case taskInstallAll:
			return s, finishInstallAllCmd(sh)
		case taskInstall:
			if msg.err != nil {
				sh.appendLog(fmt.Sprintf("[%s] error: %v", s.selected.Name, msg.err))
				sh.statusMsg = "install failed"
				return s, resetToRoot()
			}
			sh.appendLog(fmt.Sprintf("[%s] installed", s.selected.Name))
			return s, finishInstallCmd(sh, s.selected, s.pick, msg.path, msg.version)
		}

	case tea.KeyMsg:
		if s.kind == taskArchive && s.done {
			switch msg.String() {
			case "esc", "enter", "q":
				sh.statusMsg = ""
				return s, archiveFinished()
			}
		}
	}
	return s, nil
}

func (s *taskScreen) View(sh *shared) string {
	glyph := sh.spinner.View()
	if s.done {
		glyph = "•"
	}
	label := s.label
	if s.kind == taskArchive && s.done {
		label = "done — esc to go back"
	}
	progress := fmt.Sprintf("\n  %s %s", glyph, label)
	if len(sh.logs) == 0 {
		return progress
	}
	out := sh.outputView()
	filler := s.bodyHeight - lipgloss.Height(progress) - lipgloss.Height(out)
	if filler < 1 {
		filler = 1
	}
	return lipgloss.JoinVertical(lipgloss.Left, progress, blanks(filler), out)
}

func (s *taskScreen) HelpView(sh *shared) string {
	if s.kind == taskArchive && s.done {
		return sh.bindingHelp([]key.Binding{
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		})
	}
	return sh.noteHelp("non-interactive · working…")
}

func (s *taskScreen) SetSize(sh *shared, width, bodyHeight int) { s.bodyHeight = bodyHeight }
