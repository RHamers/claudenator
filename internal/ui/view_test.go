package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/trash"
)

func TestViewIncludesPreviewContentForSelectedSession(t *testing.T) {
	sessions := testSessions()
	sessions[0].LastMessage = "refactor the auth middleware"
	providers := map[string]session.Provider{}
	findRunning := func(string) (int, bool) { return 0, false }
	m := New(sessions, providers, trash.Store{}, findRunning)

	if !strings.Contains(m.View(), "refactor the auth middleware") {
		t.Fatalf("expected View() to contain preview content, got %q", m.View())
	}
}

func TestViewShowsFilterPromptWhenInFilterMode(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, '/')

	if !strings.Contains(m.View(), "Filter:") {
		t.Fatalf("expected View() to contain filter prompt, got %q", m.View())
	}
}

func TestViewShowsErrorLineWhenSet(t *testing.T) {
	m := newTestModel()
	m.err = "something went wrong"

	if !strings.Contains(m.View(), "something went wrong") {
		t.Fatalf("expected View() to contain error line, got %q", m.View())
	}
}

func TestViewShowsConfirmPromptForResumeConflict(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 555, true })

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if !strings.Contains(m.View(), "555") {
		t.Fatalf("expected View() to contain conflicting pid, got %q", m.View())
	}
}
