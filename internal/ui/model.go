package ui

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/trash"
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
	return m.updateNormalMode(keyMsg)
}

func (m Model) updateNormalMode(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	}
	return m, nil
}

func (m Model) View() string {
	return m.table.View()
}
