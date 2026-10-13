package claudecode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/RHamers/claudenator/internal/trash"
)

func TestListParsesHappyPath(t *testing.T) {
	p := New("testdata/happy", trash.Store{Dir: t.TempDir()})
	sessions, err := p.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	s := sessions[0]
	if s.ID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("ID = %q", s.ID)
	}
	if s.Dir != "/home/user/proj" {
		t.Errorf("Dir = %q", s.Dir)
	}
	if s.Branch != "main" {
		t.Errorf("Branch = %q", s.Branch)
	}
	if s.LastMessage != "now add tests for it" {
		t.Errorf("LastMessage = %q", s.LastMessage)
	}
	if s.Provider != ProviderName {
		t.Errorf("Provider = %q", s.Provider)
	}
}

func TestListSkipsSessionWithNoCWD(t *testing.T) {
	p := New("testdata/missing-cwd", trash.Store{Dir: t.TempDir()})
	sessions, err := p.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListToleratesCorruptLastLine(t *testing.T) {
	p := New("testdata/corrupt-last-line", trash.Store{Dir: t.TempDir()})
	sessions, err := p.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session despite corrupt last line, got %d", len(sessions))
	}
	if sessions[0].LastMessage != "last good message" {
		t.Errorf("LastMessage = %q, want last good line before the corrupt one", sessions[0].LastMessage)
	}
}

func TestListSkipsEmptyFile(t *testing.T) {
	p := New("testdata/empty", trash.Store{Dir: t.TempDir()})
	sessions, err := p.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions for empty file, got %d", len(sessions))
	}
}

func TestListIgnoresNonJSONLSiblings(t *testing.T) {
	p := New("testdata/with-nonjsonl-sibling", trash.Store{Dir: t.TempDir()})
	sessions, err := p.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
}

func TestDeleteMovesFileToTrash(t *testing.T) {
	root := t.TempDir()
	projDir := filepath.Join(root, "-home-user-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	srcPath := filepath.Join(projDir, "66666666-6666-6666-6666-666666666666.jsonl")
	if err := os.WriteFile(srcPath, []byte(`{"type":"system","cwd":"/home/user/proj","sessionId":"66666666-6666-6666-6666-666666666666","timestamp":"2026-10-01T10:00:00Z"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	trashDir := t.TempDir()
	p := New(root, trash.Store{Dir: trashDir})
	sessions, err := p.List()
	if err != nil || len(sessions) != 1 {
		t.Fatalf("List: %v, %d sessions", err, len(sessions))
	}

	if err := p.Delete(sessions[0]); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Fatalf("expected original file gone, stat err = %v", err)
	}
	if _, err := os.Stat(trash.TrashPath(trashDir, sessions[0].ID)); err != nil {
		t.Fatalf("expected file in trash: %v", err)
	}
}
