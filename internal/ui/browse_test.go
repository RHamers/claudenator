package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/trash"
)

// newBrowseTestModel builds a model rooted (for browsing purposes) at a
// temp directory containing a child "sub" which itself contains "nested".
func newBrowseTestModel(t *testing.T) (Model, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	providers := map[string]session.Provider{}
	findRunning := func(string) (int, bool) { return 0, false }
	m := New(nil, providers, trash.Store{}, findRunning, root)
	return m, root
}

func TestNKeyEntersBrowseModeAtLaunchDir(t *testing.T) {
	m, root := newBrowseTestModel(t)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(Model)

	if m.mode != modeBrowse {
		t.Fatalf("expected modeBrowse, got %v", m.mode)
	}
	if m.browseCurrent != root {
		t.Fatalf("expected browseCurrent %q, got %q", root, m.browseCurrent)
	}
	if len(m.browseEntries) != 1 || m.browseEntries[0] != "sub" {
		t.Fatalf("expected entries [sub], got %v", m.browseEntries)
	}
}

func TestLDescendsIntoHighlightedSubdir(t *testing.T) {
	m, root := newBrowseTestModel(t)
	m = pressRune(m, 'n')
	m = pressRune(m, 'l')

	want := filepath.Join(root, "sub")
	if m.browseCurrent != want {
		t.Fatalf("expected browseCurrent %q, got %q", want, m.browseCurrent)
	}
	if len(m.browseEntries) != 1 || m.browseEntries[0] != "nested" {
		t.Fatalf("expected entries [nested], got %v", m.browseEntries)
	}
}

func TestHGoesBackUpOneLevel(t *testing.T) {
	m, root := newBrowseTestModel(t)
	m = pressRune(m, 'n')
	m = pressRune(m, 'l')
	m = pressRune(m, 'h')

	if m.browseCurrent != root {
		t.Fatalf("expected browseCurrent back at root %q, got %q", root, m.browseCurrent)
	}
}

func TestHAtRootDoesNotGoAboveLaunchDir(t *testing.T) {
	m, root := newBrowseTestModel(t)
	m = pressRune(m, 'n')
	m = pressRune(m, 'h')

	if m.browseCurrent != root {
		t.Fatalf("expected browseCurrent to stay at root %q, got %q", root, m.browseCurrent)
	}
}

func TestEnterInBrowseModeSetsPendingResumeAndQuits(t *testing.T) {
	m, root := newBrowseTestModel(t)
	m = pressRune(m, 'n')
	m = pressRune(m, 'l') // descend into "sub"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if cmd == nil {
		t.Fatalf("expected tea.Quit command")
	}
	plan := m.PendingResume()
	if plan == nil {
		t.Fatalf("expected pending resume plan")
	}
	wantDir := filepath.Join(root, "sub")
	if plan.Dir != wantDir {
		t.Fatalf("expected plan.Dir %q, got %q", wantDir, plan.Dir)
	}
	if len(plan.Argv) == 0 || plan.Argv[0] != "claude" {
		t.Fatalf("expected argv to start with claude, got %v", plan.Argv)
	}
}

func TestEscCancelsBrowseModeBackToNormal(t *testing.T) {
	m, _ := newBrowseTestModel(t)
	m = pressRune(m, 'n')

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after esc, got %v", m.mode)
	}
	if m.PendingResume() != nil {
		t.Fatalf("expected no pending resume after cancelling")
	}
}
