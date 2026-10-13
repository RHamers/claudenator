package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/session/claudecode"
	"github.com/RHamers/claudenator/internal/trash"
)

func newDeleteTestModel(t *testing.T) (Model, string, string) {
	t.Helper()
	projectsRoot := t.TempDir()
	projDir := filepath.Join(projectsRoot, "-home-user-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sessionPath := filepath.Join(projDir, "77777777-7777-7777-7777-777777777777.jsonl")
	content := `{"type":"system","cwd":"/home/user/proj","gitBranch":"main","sessionId":"77777777-7777-7777-7777-777777777777","timestamp":"2026-10-01T10:00:00Z"}`
	if err := os.WriteFile(sessionPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	trashDir := t.TempDir()
	trashStore := trash.Store{Dir: trashDir}
	provider := claudecode.New(projectsRoot, trashStore)
	sessions, err := provider.List()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("List: %v, %d sessions", err, len(sessions))
	}

	providers := map[string]session.Provider{claudecode.ProviderName: provider}
	findRunning := func(string) (int, bool) { return 0, false }
	m := New(sessions, providers, trashStore, findRunning)
	return m, sessionPath, trashDir
}

func TestDeleteWithConfirmMovesToTrashAndRemovesFromVisible(t *testing.T) {
	m, sessionPath, trashDir := newDeleteTestModel(t)

	m = pressRune(m, 'd')
	if m.confirm == nil {
		t.Fatalf("expected delete confirm to be pending")
	}
	m = pressRune(m, 'y')

	if len(m.visible) != 0 {
		t.Fatalf("expected session removed from visible list, got %d", len(m.visible))
	}
	if _, err := os.Stat(sessionPath); !os.IsNotExist(err) {
		t.Fatalf("expected original file gone, stat err = %v", err)
	}
	if _, err := os.Stat(trash.TrashPath(trashDir, "77777777-7777-7777-7777-777777777777")); err != nil {
		t.Fatalf("expected file in trash: %v", err)
	}
}

func TestDeleteConfirmNDoesNothing(t *testing.T) {
	m, sessionPath, _ := newDeleteTestModel(t)

	m = pressRune(m, 'd')
	m = pressRune(m, 'n')

	if m.confirm != nil {
		t.Fatalf("expected confirm cleared")
	}
	if len(m.visible) != 1 {
		t.Fatalf("expected session still visible")
	}
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("expected original file untouched: %v", err)
	}
}

func TestDeleteConfirmEscDoesNothing(t *testing.T) {
	m, sessionPath, _ := newDeleteTestModel(t)

	m = pressRune(m, 'd')
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.confirm != nil {
		t.Fatalf("expected confirm cleared by Esc")
	}
	if len(m.visible) != 1 {
		t.Fatalf("expected session still visible")
	}
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("expected original file untouched: %v", err)
	}
}

func TestDeleteBlockedWhenSessionRunning(t *testing.T) {
	m, sessionPath, _ := newDeleteTestModel(t)
	m.findRunning = func(id string) (int, bool) { return 4242, true }

	m = pressRune(m, 'd')

	if m.confirm != nil {
		t.Fatalf("expected no confirm when session is running")
	}
	if m.err == "" {
		t.Fatalf("expected an inline error explaining the block")
	}
	if len(m.visible) != 1 {
		t.Fatalf("expected session still visible")
	}
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("expected original file untouched: %v", err)
	}
}

func TestUndoRestoresDeletedSession(t *testing.T) {
	m, sessionPath, _ := newDeleteTestModel(t)

	m = pressRune(m, 'd')
	m = pressRune(m, 'y')
	m = pressRune(m, 'u')

	if len(m.visible) != 1 {
		t.Fatalf("expected session restored to visible list, got %d", len(m.visible))
	}
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("expected original file restored: %v", err)
	}
}

func TestUnrelatedKeyClearsStaleError(t *testing.T) {
	m, _, _ := newDeleteTestModel(t)
	m.findRunning = func(id string) (int, bool) { return 4242, true }

	m = pressRune(m, 'd') // blocked delete, sets m.err
	if m.err == "" {
		t.Fatalf("expected error to be set by blocked delete")
	}

	m = pressRune(m, 'j') // unrelated navigation key
	if m.err != "" {
		t.Fatalf("expected stale error to be cleared by unrelated keypress, got %q", m.err)
	}
}

func TestDeleteUndoDeleteCycleKeepsTrashAndMetaConsistent(t *testing.T) {
	m, sessionPath, trashDir := newDeleteTestModel(t)
	const sessionID = "77777777-7777-7777-7777-777777777777"

	// First delete.
	m = pressRune(m, 'd')
	m = pressRune(m, 'y')
	if len(m.visible) != 0 {
		t.Fatalf("expected session removed from visible after first delete, got %d", len(m.visible))
	}
	if _, err := os.Stat(trash.TrashPath(trashDir, sessionID)); err != nil {
		t.Fatalf("expected file in trash after first delete: %v", err)
	}

	// Undo.
	m = pressRune(m, 'u')
	if len(m.visible) != 1 {
		t.Fatalf("expected session restored to visible after undo, got %d", len(m.visible))
	}
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("expected original file restored after undo: %v", err)
	}

	// Delete again.
	m = pressRune(m, 'd')
	m = pressRune(m, 'y')
	if len(m.visible) != 0 {
		t.Fatalf("expected session removed from visible after second delete, got %d", len(m.visible))
	}
	if _, err := os.Stat(sessionPath); !os.IsNotExist(err) {
		t.Fatalf("expected original file gone after second delete, stat err = %v", err)
	}
	if _, err := os.Stat(trash.TrashPath(trashDir, sessionID)); err != nil {
		t.Fatalf("expected file in trash after second delete: %v", err)
	}

	// Sanity check: the meta sidecar still points at the correct
	// original path, by restoring it once more via the trash store
	// directly and confirming it lands back at sessionPath.
	trashStore := trash.Store{Dir: trashDir}
	if err := trashStore.Restore(sessionID); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("expected final restore to put file back at original path: %v", err)
	}
}

func TestUndoWithNothingDeletedIsSafeNoop(t *testing.T) {
	m, _, _ := newDeleteTestModel(t)
	m = pressRune(m, 'u')
	if len(m.visible) != 1 {
		t.Fatalf("expected no change, got %d visible", len(m.visible))
	}
	if m.err != "" {
		t.Fatalf("expected no error, got %q", m.err)
	}
}
