package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/RHamers/claudenator/internal/resume"
)

func (m Model) beginFork() (tea.Model, tea.Cmd) {
	sel := m.selected()
	if sel.ID == "" {
		return m, nil
	}
	if _, ok := m.providers[sel.Provider]; !ok {
		m.err = fmt.Sprintf("no provider registered for %q", sel.Provider)
		return m, nil
	}
	m.err = ""
	m.forkTarget = sel
	m.forkNameErr = ""
	m.forkNameInput.SetValue("")
	m.forkNameInput.Focus()
	m.mode = modeForkName
	return m, nil
}

func (m Model) updateForkNameMode(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.mode = modeNormal
		m.forkNameInput.SetValue("")
		m.forkNameErr = ""
		return m, nil
	case tea.KeyEnter:
		name := strings.TrimSpace(m.forkNameInput.Value())
		if name == "" {
			m.forkNameErr = "name is required"
			return m, nil
		}
		provider := m.providers[m.forkTarget.Provider]
		plan, err := resume.Prepare(m.forkTarget, provider, m.findRunning, true, name)
		if err != nil {
			m.mode = modeNormal
			m.err = err.Error()
			return m, nil
		}
		m.pendingResume = plan
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.forkNameInput, cmd = m.forkNameInput.Update(keyMsg)
	return m, cmd
}

func (m Model) forkNameView() string {
	body := "Fork session\n\n" + m.forkNameInput.View()
	if m.forkNameErr != "" {
		body += "\n" + m.forkNameErr
	}
	body += "\n\nenter: confirm   esc: cancel"
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Render(body)
}
