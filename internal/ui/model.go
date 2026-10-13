package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/RHamers/claudenator/internal/resume"
	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/trash"
)

type mode int

const (
	modeNormal mode = iota
	modeFilter
)

type focusPane int

const (
	focusList focusPane = iota
	focusPreview
)

type Model struct {
	all           []session.Session
	visible       []session.Session
	table         table.Model
	sortCol       SortColumn
	pendingG      bool
	providers     map[string]session.Provider
	trashStore    trash.Store
	findRunning   func(string) (int, bool)
	mode          mode
	filterInput   textinput.Model
	confirm       *confirmState
	err           string
	undoStack     trash.UndoStack
	preview       viewport.Model
	focus         focusPane
	pendingResume *resume.Plan
}

func columns() []table.Column {
	return []table.Column{
		{Title: "Dir", Width: 40},
		{Title: "Branch", Width: 16},
		{Title: "Last edited", Width: 16},
	}
}

func rowsFor(sessions []session.Session) []table.Row {
	rows := make([]table.Row, len(sessions))
	for i, s := range sessions {
		rows[i] = table.Row{s.Dir, s.Branch, humanize(s.LastActive)}
	}
	return rows
}

func New(sessions []session.Session, providers map[string]session.Provider, trashStore trash.Store, findRunning func(string) (int, bool)) Model {
	m := Model{
		all:         append([]session.Session(nil), sessions...),
		sortCol:     SortByLastActive,
		providers:   providers,
		trashStore:  trashStore,
		findRunning: findRunning,
	}
	m.visible = append([]session.Session(nil), m.all...)
	sortSessions(m.visible, m.sortCol)
	m.table = table.New(
		table.WithColumns(columns()),
		table.WithRows(rowsFor(m.visible)),
		table.WithFocused(true),
	)
	m.filterInput = textinput.New()
	m.filterInput.Prompt = "Filter: "
	m.preview = viewport.New(80, 6)
	m.refreshPreview()
	return m
}

func (m Model) selected() session.Session {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.visible) {
		return session.Session{}
	}
	return m.visible[i]
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.mode == modeFilter {
		return m.updateFilterMode(keyMsg)
	}
	return m.updateNormalMode(keyMsg)
}

func (m *Model) refreshVisible() {
	m.visible = applyFilter(m.all, m.filterInput.Value())
	sortSessions(m.visible, m.sortCol)
	m.table.SetRows(rowsFor(m.visible))
	m.refreshPreview()
}

func (m *Model) refreshPreview() {
	m.preview.SetContent(m.selected().LastMessage)
}

func (m Model) updateFilterMode(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.Type {
	case tea.KeyEnter:
		m.mode = modeNormal
		return m, nil
	case tea.KeyEsc:
		m.filterInput.SetValue("")
		m.mode = modeNormal
		m.refreshVisible()
		return m, nil
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(keyMsg)
	m.refreshVisible()
	return m, cmd
}

func (m Model) updateNormalMode(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirm != nil {
		return m.updateConfirm(keyMsg)
	}

	key := keyMsg.String()
	wasPendingG := m.pendingG
	m.pendingG = false

	switch key {
	case "j", "down":
		if m.focus == focusPreview {
			m.preview.LineDown(1)
		} else {
			m.table.MoveDown(1)
			m.refreshPreview()
		}
	case "k", "up":
		if m.focus == focusPreview {
			m.preview.LineUp(1)
		} else {
			m.table.MoveUp(1)
			m.refreshPreview()
		}
	case "g":
		if wasPendingG {
			m.table.GotoTop()
			m.refreshPreview()
		} else {
			m.pendingG = true
		}
	case "G":
		m.table.GotoBottom()
		m.refreshPreview()
	case "h":
		m.focus = focusList
	case "l":
		m.focus = focusPreview
	case "s":
		m.sortCol = m.sortCol.Next()
		sortSessions(m.visible, m.sortCol)
		m.table.SetRows(rowsFor(m.visible))
		m.refreshPreview()
	case "/", "f":
		m.mode = modeFilter
		m.filterInput.Focus()
	case "d":
		return m.beginDelete()
	case "u":
		return m.undo()
	case "enter":
		return m.beginResume()
	}
	return m, nil
}

func (m Model) beginResume() (tea.Model, tea.Cmd) {
	sel := m.selected()
	if sel.ID == "" {
		return m, nil
	}
	provider, ok := m.providers[sel.Provider]
	if !ok {
		m.err = fmt.Sprintf("no provider registered for %q", sel.Provider)
		return m, nil
	}
	plan, err := resume.Prepare(sel, provider, m.findRunning)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.err = ""
	if plan.ConflictPID != 0 {
		m.confirm = &confirmState{kind: confirmResumeConflict, target: sel, pid: plan.ConflictPID, plan: plan}
		return m, nil
	}
	m.pendingResume = plan
	return m, tea.Quit
}

func (m Model) PendingResume() *resume.Plan {
	return m.pendingResume
}

func (m Model) beginDelete() (tea.Model, tea.Cmd) {
	sel := m.selected()
	if sel.ID == "" {
		return m, nil // empty list, nothing to delete
	}
	if pid, running := m.findRunning(sel.ID); running {
		m.err = fmt.Sprintf("session %s is running (pid %d) — cannot delete", sel.ID, pid)
		return m, nil
	}
	m.err = ""
	m.confirm = &confirmState{kind: confirmDelete, target: sel}
	return m, nil
}

func (m Model) undo() (tea.Model, tea.Cmd) {
	s, ok := m.undoStack.Pop()
	if !ok {
		return m, nil
	}
	if err := m.trashStore.Restore(s.ID); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.all = append(m.all, s)
	m.refreshVisible()
	return m, nil
}

func (m Model) updateConfirm(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.String() {
	case "y":
		c := *m.confirm
		m.confirm = nil
		switch c.kind {
		case confirmDelete:
			return m.performDelete(c.target)
		case confirmResumeConflict:
			m.pendingResume = c.plan
			return m, tea.Quit
		}
	case "n":
		m.confirm = nil
	}
	return m, nil
}

func (m Model) performDelete(s session.Session) (tea.Model, tea.Cmd) {
	provider, ok := m.providers[s.Provider]
	if !ok {
		m.err = fmt.Sprintf("no provider registered for %q", s.Provider)
		return m, nil
	}
	if err := provider.Delete(s); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.all = removeByID(m.all, s.ID)
	m.undoStack.Push(s)
	m.refreshVisible()
	return m, nil
}

func removeByID(sessions []session.Session, id string) []session.Session {
	result := make([]session.Session, 0, len(sessions))
	for _, s := range sessions {
		if s.ID != id {
			result = append(result, s)
		}
	}
	return result
}

func (m Model) View() string {
	view := m.table.View() + "\n" + m.preview.View()

	if m.mode == modeFilter {
		view += "\n" + m.filterInput.View()
	}

	if m.err != "" {
		view += "\n" + m.err
	}

	if m.confirm != nil {
		switch m.confirm.kind {
		case confirmDelete:
			view += "\n" + fmt.Sprintf("Delete session in %s? (y/n)", m.confirm.target.Dir)
		case confirmResumeConflict:
			view += "\n" + fmt.Sprintf("Session already running (pid %d) — attach anyway? (y/n)", m.confirm.pid)
		}
	}

	return view
}
