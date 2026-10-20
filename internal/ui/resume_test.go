package ui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/trash"
)

type stubProvider struct{}

func (stubProvider) List() ([]session.Session, error) { return nil, nil }
func (stubProvider) ResumeArgv(s session.Session) []string {
	return []string{"claude", "--resume", s.ID}
}
func (stubProvider) Delete(s session.Session) error { return nil }

func newResumeTestModel(t *testing.T, findRunning func(string) (int, bool)) Model {
	t.Helper()
	dir := t.TempDir()
	sessions := []session.Session{
		{ID: "abc", Provider: "stub", Dir: dir, Branch: "main"},
	}
	providers := map[string]session.Provider{"stub": stubProvider{}}
	return New(sessions, providers, trash.Store{}, findRunning, dir)
}

func TestEnterNoConflictSetsPendingResumeAndQuits(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 0, false })

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.PendingResume() == nil {
		t.Fatalf("expected pending resume plan")
	}
	if cmd == nil {
		t.Fatalf("expected tea.Quit command")
	}
}

func TestEnterConflictOpensConfirm(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 555, true })

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	if m.confirm == nil || m.confirm.kind != confirmResumeConflict {
		t.Fatalf("expected resume-conflict confirm pending")
	}
	if m.PendingResume() != nil {
		t.Fatalf("expected no pending resume before confirming")
	}
}

func TestEnterConflictYConfirmsResume(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 555, true })
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	m3, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = m3.(Model)

	if m.PendingResume() == nil {
		t.Fatalf("expected pending resume after confirming")
	}
	if cmd == nil {
		t.Fatalf("expected tea.Quit command")
	}
}

func TestEnterConflictNCancelsResume(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 555, true })
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = m3.(Model)

	if m.confirm != nil {
		t.Fatalf("expected confirm cleared")
	}
	if m.PendingResume() != nil {
		t.Fatalf("expected no pending resume")
	}
}

func TestEnterMissingDirShowsErrorNotQuit(t *testing.T) {
	missing := "/this/path/does/not/exist/claudenator-test"
	os.RemoveAll(missing) // ensure it really doesn't exist
	sessions := []session.Session{{ID: "abc", Provider: "stub", Dir: missing}}
	providers := map[string]session.Provider{"stub": stubProvider{}}
	m := New(sessions, providers, trash.Store{}, func(string) (int, bool) { return 0, false }, "/tmp")

	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	if m.err == "" {
		t.Fatalf("expected inline error for missing directory")
	}
	if cmd != nil {
		t.Fatalf("expected no quit command when directory is missing")
	}
}
