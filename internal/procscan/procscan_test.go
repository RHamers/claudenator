package procscan

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCmdline(t *testing.T, procRoot, pid string, argv []string) {
	t.Helper()
	dir := filepath.Join(procRoot, pid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := ""
	for _, a := range argv {
		content += a + "\x00"
	}
	if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(content), 0o644); err != nil {
		t.Fatalf("write cmdline: %v", err)
	}
}

func TestFindResumeProcessFound(t *testing.T) {
	root := t.TempDir()
	writeCmdline(t, root, "123", []string{"claude", "--resume", "abc-123"})
	writeCmdline(t, root, "456", []string{"some-other-process"})

	pid, found := FindResumeProcess(root, "abc-123")
	if !found || pid != 123 {
		t.Fatalf("expected pid=123 found=true, got pid=%d found=%v", pid, found)
	}
}

func TestFindResumeProcessNotFound(t *testing.T) {
	root := t.TempDir()
	writeCmdline(t, root, "456", []string{"some-other-process"})

	_, found := FindResumeProcess(root, "abc-123")
	if found {
		t.Fatalf("expected found=false")
	}
}

func TestFindResumeProcessEmptyProcRoot(t *testing.T) {
	root := t.TempDir() // no entries at all
	_, found := FindResumeProcess(root, "abc-123")
	if found {
		t.Fatalf("expected found=false on empty procfs")
	}
}
