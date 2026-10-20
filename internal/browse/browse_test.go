package browse

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestSubdirsListsOnlyDirectoriesSortedAlphabetically(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "zebra"))
	mustMkdir(t, filepath.Join(root, "alpha"))
	if err := os.WriteFile(filepath.Join(root, "file.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Subdirs(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"alpha", "zebra"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSubdirsExcludesDotdirs(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, ".git"))
	mustMkdir(t, filepath.Join(root, "src"))

	got, err := Subdirs(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"src"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSubdirsOnMissingDirReturnsError(t *testing.T) {
	_, err := Subdirs(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatalf("expected error for a missing directory")
	}
}
