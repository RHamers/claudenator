package trash

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RHamers/claudenator/internal/session"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestStoreMoveAndRestore(t *testing.T) {
	projectsDir := t.TempDir()
	trashDir := filepath.Join(t.TempDir(), "trash")
	origPath := filepath.Join(projectsDir, "abc123.jsonl")
	writeFile(t, origPath, `{"sessionId":"abc123"}`)

	store := Store{Dir: trashDir}
	sess := session.Session{ID: "abc123", FilePath: origPath}

	if err := store.Move(sess); err != nil {
		t.Fatalf("Move: %v", err)
	}
	if _, err := os.Stat(origPath); !os.IsNotExist(err) {
		t.Fatalf("expected original file gone, stat err = %v", err)
	}
	if _, err := os.Stat(TrashPath(trashDir, "abc123")); err != nil {
		t.Fatalf("expected trashed file present: %v", err)
	}

	if err := store.Restore("abc123"); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if _, err := os.Stat(origPath); err != nil {
		t.Fatalf("expected original file restored: %v", err)
	}
	if _, err := os.Stat(TrashPath(trashDir, "abc123")); !os.IsNotExist(err) {
		t.Fatalf("expected trashed file gone after restore, stat err = %v", err)
	}
}

func TestStorePrunePrunesOldKeepsRecent(t *testing.T) {
	trashDir := t.TempDir()
	store := Store{Dir: trashDir}
	now := time.Now()

	old := session.Session{ID: "old", FilePath: filepath.Join(t.TempDir(), "old.jsonl")}
	writeFile(t, old.FilePath, `{}`)
	recent := session.Session{ID: "recent", FilePath: filepath.Join(t.TempDir(), "recent.jsonl")}
	writeFile(t, recent.FilePath, `{}`)

	if err := store.Move(old); err != nil {
		t.Fatalf("Move old: %v", err)
	}
	if err := store.Move(recent); err != nil {
		t.Fatalf("Move recent: %v", err)
	}

	// Backdate "old"'s deletion by rewriting its meta sidecar.
	meta, err := readMeta(MetaPath(trashDir, "old"))
	if err != nil {
		t.Fatalf("readMeta: %v", err)
	}
	meta.DeletedAt = now.Add(-31 * 24 * time.Hour)
	if err := writeMeta(MetaPath(trashDir, "old"), meta); err != nil {
		t.Fatalf("writeMeta: %v", err)
	}

	if err := store.Prune(30*24*time.Hour, now); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	if _, err := os.Stat(TrashPath(trashDir, "old")); !os.IsNotExist(err) {
		t.Fatalf("expected old trashed file pruned, stat err = %v", err)
	}
	if _, err := os.Stat(TrashPath(trashDir, "recent")); err != nil {
		t.Fatalf("expected recent trashed file kept: %v", err)
	}
}
