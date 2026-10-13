package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RHamers/claudenator/internal/session"
)

func pressRunes(m Model, s string) Model {
	for _, r := range s {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	return m
}

func pressKey(m Model, t tea.KeyType) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: t})
	return updated.(Model)
}

func TestApplyFilterNarrowsToMatches(t *testing.T) {
	all := []session.Session{
		{ID: "a", Dir: "/home/user/claudenator", Branch: "main"},
		{ID: "b", Dir: "/home/user/other-project", Branch: "main"},
	}
	result := applyFilter(all, "claudenator")
	if len(result) != 1 || result[0].ID != "a" {
		t.Fatalf("expected only session a, got %+v", result)
	}
}

func TestApplyFilterEmptyQueryReturnsAll(t *testing.T) {
	all := []session.Session{{ID: "a"}, {ID: "b"}}
	result := applyFilter(all, "")
	if len(result) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(result))
	}
}

func TestSlashEntersFilterModeAndNarrowsLive(t *testing.T) {
	m := newTestModel() // sessions a(/z-dir) and b(/a-dir), from Task 5's helper
	m = pressRune(m, '/')
	m = pressRunes(m, "a-dir")
	if len(m.visible) != 1 || m.visible[0].Dir != "/a-dir" {
		t.Fatalf("expected only /a-dir visible, got %+v", m.visible)
	}
}

func TestFEntersFilterModeToo(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, 'f')
	m = pressRunes(m, "a-dir")
	if len(m.visible) != 1 || m.visible[0].Dir != "/a-dir" {
		t.Fatalf("expected only /a-dir visible, got %+v", m.visible)
	}
}

func TestEnterInFilterModeKeepsFilterAndReturnsToNormalMode(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, '/')
	m = pressRunes(m, "a-dir")
	m = pressKey(m, tea.KeyEnter)
	if m.mode != modeNormal {
		t.Fatalf("expected normal mode after Enter")
	}
	if len(m.visible) != 1 {
		t.Fatalf("expected filter to remain applied, got %d visible", len(m.visible))
	}
	// Normal-mode keys work again.
	m = pressRune(m, 'j')
}

func TestEscInFilterModeClearsFilterAndReturnsToNormalMode(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, '/')
	m = pressRunes(m, "a-dir")
	m = pressKey(m, tea.KeyEsc)
	if m.mode != modeNormal {
		t.Fatalf("expected normal mode after Esc")
	}
	if len(m.visible) != 2 {
		t.Fatalf("expected filter cleared (both sessions visible), got %d", len(m.visible))
	}
}
