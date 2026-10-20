package ui

import (
	"testing"
	"time"

	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/trash"
	tea "github.com/charmbracelet/bubbletea"
)

func testSessions() []session.Session {
	now := time.Now()
	return []session.Session{
		{ID: "a", Provider: "claude-code", Dir: "/z-dir", Branch: "main", LastActive: now},
		{ID: "b", Provider: "claude-code", Dir: "/a-dir", Branch: "zzz", LastActive: now.Add(-time.Hour)},
	}
}

func newTestModel() Model {
	providers := map[string]session.Provider{}
	findRunning := func(string) (int, bool) { return 0, false }
	return New(testSessions(), providers, trash.Store{}, findRunning, "/tmp")
}

func pressRune(m Model, r rune) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	return updated.(Model)
}

func TestNewSortsByLastActiveDescendingByDefault(t *testing.T) {
	m := newTestModel()
	if m.visible[0].ID != "a" {
		t.Fatalf("expected most recent session (a) first, got %q", m.visible[0].ID)
	}
}

func TestNavigationDownMovesCursor(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, 'j')
	if m.table.Cursor() != 1 {
		t.Fatalf("expected cursor 1, got %d", m.table.Cursor())
	}
}

func TestNavigationUpMovesCursorBack(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, 'j')
	m = pressRune(m, 'k')
	if m.table.Cursor() != 0 {
		t.Fatalf("expected cursor 0, got %d", m.table.Cursor())
	}
}

func TestGGJumpsToTop(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, 'j') // move to 1
	m = pressRune(m, 'g')
	m = pressRune(m, 'g')
	if m.table.Cursor() != 0 {
		t.Fatalf("expected cursor 0 after gg, got %d", m.table.Cursor())
	}
}

func TestCapitalGJumpsToBottom(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = updated.(Model)
	if m.table.Cursor() != 1 {
		t.Fatalf("expected cursor 1 after G, got %d", m.table.Cursor())
	}
}

func TestSortCyclesToDirThenBranchThenBack(t *testing.T) {
	m := newTestModel()

	m = pressRune(m, 's') // Dir ascending
	if m.visible[0].Dir != "/a-dir" {
		t.Fatalf("after first sort, expected /a-dir first, got %q", m.visible[0].Dir)
	}

	m = pressRune(m, 's') // Branch ascending
	if m.visible[0].Branch != "main" {
		t.Fatalf("after second sort, expected main first, got %q", m.visible[0].Branch)
	}

	m = pressRune(m, 's') // back to LastActive descending
	if m.visible[0].ID != "a" {
		t.Fatalf("after third sort, expected a first again, got %q", m.visible[0].ID)
	}
}

func TestRowsForShowsRunningIndicatorOnlyForRunningSessions(t *testing.T) {
	sessions := []session.Session{
		{ID: "a", Dir: "/dir-a", Running: true},
		{ID: "b", Dir: "/dir-b", Running: false},
	}
	rows := rowsFor(sessions)
	if rows[0][0] == "" {
		t.Fatalf("expected running session to have a non-empty indicator, got row %+v", rows[0])
	}
	if rows[1][0] != "" {
		t.Fatalf("expected non-running session to have an empty indicator, got %q", rows[1][0])
	}
}

func TestQKeyQuitsWithoutSettingPendingResume(t *testing.T) {
	m := newTestModel()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = updated.(Model)

	if cmd == nil {
		t.Fatalf("expected a non-nil tea.Cmd (tea.Quit) when pressing q")
	}
	if m.PendingResume() != nil {
		t.Fatalf("expected quitting not to set a pending resume")
	}
}

func TestRowsForShowsFirstMessageSummaryTruncated(t *testing.T) {
	sessions := []session.Session{
		{ID: "a", Dir: "/dir-a", FirstMessage: "please refactor this very long function so it reads cleanly end to end"},
		{ID: "b", Dir: "/dir-b", FirstMessage: "short prompt"},
	}
	rows := rowsFor(sessions)
	if got := rows[0][4]; len([]rune(got)) != 40 || got[len(got)-3:] != "…" {
		t.Fatalf("expected long first message truncated to 40 runes with ellipsis, got %q (len %d)", got, len([]rune(got)))
	}
	if rows[1][4] != "short prompt" {
		t.Fatalf("expected short first message unchanged, got %q", rows[1][4])
	}
}

func TestNavigationOnEmptyListDoesNotPanic(t *testing.T) {
	providers := map[string]session.Provider{}
	findRunning := func(string) (int, bool) { return 0, false }
	m := New(nil, providers, trash.Store{}, findRunning, "/tmp")
	m = pressRune(m, 'j')
	m = pressRune(m, 'k')
	_ = m.selected() // must not panic
}
