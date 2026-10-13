package ui

import (
	"testing"

	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/trash"
)

func TestHLTogglesFocusBetweenListAndPreview(t *testing.T) {
	m := newTestModel()
	if m.focus != focusList {
		t.Fatalf("expected initial focus on list")
	}
	m = pressRune(m, 'l')
	if m.focus != focusPreview {
		t.Fatalf("expected focus on preview after l")
	}
	m = pressRune(m, 'h')
	if m.focus != focusList {
		t.Fatalf("expected focus back on list after h")
	}
}

func TestJKMovesSelectionWhenListFocused(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, 'j')
	if m.table.Cursor() != 1 {
		t.Fatalf("expected cursor 1, got %d", m.table.Cursor())
	}
}

func TestJKScrollsPreviewWhenPreviewFocused(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, 'l') // focus preview
	cursorBefore := m.table.Cursor()
	m = pressRune(m, 'j')
	if m.table.Cursor() != cursorBefore {
		t.Fatalf("expected list cursor unchanged while preview focused, got %d want %d", m.table.Cursor(), cursorBefore)
	}
}

func TestPreviewContentMatchesSelectedSessionLastMessage(t *testing.T) {
	sessions := testSessions()
	sessions[0].LastMessage = "refactor the auth middleware"
	providers := map[string]session.Provider{}
	findRunning := func(string) (int, bool) { return 0, false }
	m := New(sessions, providers, trash.Store{}, findRunning)

	if m.preview.View() == "" {
		t.Fatalf("expected non-empty preview for selected session")
	}
}
