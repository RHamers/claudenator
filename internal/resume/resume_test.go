package resume

import (
	"testing"

	"github.com/RHamers/claudenator/internal/session"
)

type fakeProvider struct{}

func (fakeProvider) List() ([]session.Session, error) { return nil, nil }
func (fakeProvider) ResumeArgv(s session.Session) []string {
	return []string{"claude", "--resume", s.ID}
}
func (fakeProvider) Delete(s session.Session) error { return nil }

func TestPrepareNoConflict(t *testing.T) {
	s := session.Session{ID: "abc", Dir: t.TempDir()}
	findRunning := func(string) (int, bool) { return 0, false }

	plan, err := Prepare(s, fakeProvider{}, findRunning)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if plan.ConflictPID != 0 {
		t.Errorf("ConflictPID = %d, want 0", plan.ConflictPID)
	}
	wantArgv := []string{"claude", "--resume", "abc"}
	if len(plan.Argv) != len(wantArgv) {
		t.Fatalf("Argv = %v", plan.Argv)
	}
	for i := range wantArgv {
		if plan.Argv[i] != wantArgv[i] {
			t.Fatalf("Argv = %v, want %v", plan.Argv, wantArgv)
		}
	}
}

func TestPrepareConflict(t *testing.T) {
	s := session.Session{ID: "abc", Dir: t.TempDir()}
	findRunning := func(id string) (int, bool) {
		if id == "abc" {
			return 999, true
		}
		return 0, false
	}

	plan, err := Prepare(s, fakeProvider{}, findRunning)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if plan.ConflictPID != 999 {
		t.Errorf("ConflictPID = %d, want 999", plan.ConflictPID)
	}
}

func TestPrepareMissingDirReturnsError(t *testing.T) {
	s := session.Session{ID: "abc", Dir: "/this/path/does/not/exist/claudenator-test"}
	findRunning := func(string) (int, bool) { return 0, false }

	_, err := Prepare(s, fakeProvider{}, findRunning)
	if err == nil {
		t.Fatalf("expected error for missing directory")
	}
}
