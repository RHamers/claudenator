package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/trash"
)

func typeString(m Model, s string) Model {
	for _, r := range s {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	return m
}

func TestFKeyOpensForkNameModalWithoutForkingYet(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 0, false })

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)

	if m.mode != modeForkName {
		t.Fatalf("expected modeForkName, got %v", m.mode)
	}
	if m.PendingResume() != nil {
		t.Fatalf("expected no pending resume before a name is submitted")
	}
	if cmd != nil {
		t.Fatalf("expected no quit command when opening the modal")
	}
}

func TestForkNameEmptySubmitShowsErrorAndStaysOpen(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 0, false })
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.mode != modeForkName {
		t.Fatalf("expected to stay in modeForkName, got %v", m.mode)
	}
	if m.forkNameErr == "" {
		t.Fatalf("expected a non-empty error")
	}
	if m.PendingResume() != nil || cmd != nil {
		t.Fatalf("expected no pending resume and no quit on empty submit")
	}
}

func TestForkNameWhitespaceOnlySubmitIsTreatedAsEmpty(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 0, false })
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)
	m = typeString(m, "   ")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.forkNameErr == "" {
		t.Fatalf("expected whitespace-only input to be rejected")
	}
	if m.PendingResume() != nil || cmd != nil {
		t.Fatalf("expected no pending resume and no quit")
	}
}

func TestForkNameValidSubmitSetsPendingResumeWithNameFlag(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 0, false })
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)
	m = typeString(m, "my-fork")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	plan := m.PendingResume()
	if plan == nil {
		t.Fatalf("expected a pending resume plan")
	}
	if cmd == nil {
		t.Fatalf("expected tea.Quit command")
	}
	joined := strings.Join(plan.Argv, " ")
	if !strings.Contains(joined, "--name my-fork") {
		t.Fatalf("expected argv to contain --name my-fork, got %v", plan.Argv)
	}
}

func TestForkNameSkipsConflictDialogEvenWhenSessionRunning(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 555, true })
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)
	m = typeString(m, "my-fork")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.confirm != nil {
		t.Fatalf("expected no confirm dialog when forking, got %v", m.confirm)
	}
	if m.PendingResume() == nil || cmd == nil {
		t.Fatalf("expected pending resume and quit")
	}
}

func TestForkNameEscCancelsAndClearsState(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 0, false })
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)
	m = typeString(m, "partial")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after esc, got %v", m.mode)
	}
	if m.PendingResume() != nil || cmd != nil {
		t.Fatalf("expected no pending resume and no quit after cancel")
	}

	// Reopening must not see the previous attempt's leftovers.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)
	if m.forkNameInput.Value() != "" {
		t.Fatalf("expected a blank input on reopen, got %q", m.forkNameInput.Value())
	}
	if m.forkNameErr != "" {
		t.Fatalf("expected no leftover error on reopen, got %q", m.forkNameErr)
	}
}

func TestForkNameMissingDirShowsErrorNotQuit(t *testing.T) {
	missing := "/this/path/does/not/exist/claudenator-test"
	sessions := []session.Session{{ID: "abc", Provider: "stub", Dir: missing}}
	providers := map[string]session.Provider{"stub": stubProvider{}}
	m := New(sessions, providers, trash.Store{}, func(string) (int, bool) { return 0, false }, "/tmp")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)
	m = typeString(m, "my-fork")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.err == "" {
		t.Fatalf("expected inline error for missing directory")
	}
	if m.mode != modeNormal {
		t.Fatalf("expected to fall back to modeNormal, got %v", m.mode)
	}
	if cmd != nil {
		t.Fatalf("expected no quit command when directory is missing")
	}
}

func TestFKeyOnEmptyListDoesNothing(t *testing.T) {
	providers := map[string]session.Provider{}
	findRunning := func(string) (int, bool) { return 0, false }
	m := New(nil, providers, trash.Store{}, findRunning, "/tmp")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected to stay in modeNormal with nothing selected, got %v", m.mode)
	}
	if cmd != nil {
		t.Fatalf("expected no command")
	}
}
