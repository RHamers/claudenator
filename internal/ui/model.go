package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/trash"
)

type mode int

const (
	modeNormal mode = iota
	modeFilter
)

type Model struct {
	all         []session.Session
	visible     []session.Session
	table       table.Model
	sortCol     SortColumn
	pendingG    bool
	providers   map[string]session.Provider
	trashStore  trash.Store
	findRunning func(string) (int, bool)
	mode        mode
	filterInput textinput.Model
	confirm     *confirmState
	err         string
	undoStack   trash.UndoStack
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
		m.table.MoveDown(1)
	case "k", "up":
		m.table.MoveUp(1)
	case "g":
		if wasPendingG {
			m.table.GotoTop()
		} else {
			m.pendingG = true
		}
	case "G":
		m.table.GotoBottom()
	case "s":
		m.sortCol = m.sortCol.Next()
		sortSessions(m.visible, m.sortCol)
		m.table.SetRows(rowsFor(m.visible))
	case "/", "f":
		m.mode = modeFilter
		m.filterInput.Focus()
	case "d":
		return m.beginDelete()
	case "u":
		return m.undo()
	}
	return m, nil
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
		if c.kind == confirmDelete {
			return m.performDelete(c.target)
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
	return m.table.View()
}
