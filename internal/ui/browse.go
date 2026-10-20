package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RHamers/claudenator/internal/browse"
	"github.com/RHamers/claudenator/internal/resume"
)

// beginBrowse enters browse mode, rooted at the directory claudenator was
// launched from.
func (m Model) beginBrowse() (tea.Model, tea.Cmd) {
	if err := m.loadBrowseEntries(m.launchDir); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.err = ""
	m.mode = modeBrowse
	m.browseRoot = m.launchDir
	return m, nil
}

// loadBrowseEntries lists dir's subdirectories into the model, resetting
// the cursor. It leaves the model untouched on error.
func (m *Model) loadBrowseEntries(dir string) error {
	entries, err := browse.Subdirs(dir)
	if err != nil {
		return err
	}
	m.browseCurrent = dir
	m.browseEntries = entries
	m.browseCursor = 0
	return nil
}

func (m Model) updateBrowseMode(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.mode = modeNormal
		return m, nil
	case "j", "down":
		if m.browseCursor < len(m.browseEntries)-1 {
			m.browseCursor++
		}
	case "k", "up":
		if m.browseCursor > 0 {
			m.browseCursor--
		}
	case "l", "right":
		if m.browseCursor < len(m.browseEntries) {
			next := filepath.Join(m.browseCurrent, m.browseEntries[m.browseCursor])
			if err := m.loadBrowseEntries(next); err != nil {
				m.err = err.Error()
			} else {
				m.err = ""
			}
		}
	case "h", "left":
		if m.browseCurrent != m.browseRoot {
			parent := filepath.Dir(m.browseCurrent)
			if err := m.loadBrowseEntries(parent); err != nil {
				m.err = err.Error()
			} else {
				m.err = ""
			}
		}
	case "enter":
		m.pendingResume = &resume.Plan{Argv: []string{"claude"}, Dir: m.browseCurrent}
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) browseView() string {
	var b strings.Builder
	fmt.Fprintf(&b, "New session in: %s\n\n", m.browseCurrent)
	if len(m.browseEntries) == 0 {
		b.WriteString("(no subdirectories)\n")
	}
	for i, name := range m.browseEntries {
		cursor := "  "
		if i == m.browseCursor {
			cursor = "> "
		}
		fmt.Fprintf(&b, "%s%s\n", cursor, name)
	}
	b.WriteString("\nenter: launch here   h/l: up/down a level   j/k: move   esc: cancel\n")
	if m.err != "" {
		b.WriteString("\n" + m.err)
	}
	return b.String()
}
