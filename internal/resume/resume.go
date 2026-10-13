// Package resume turns a selected session into a concrete plan for
// replacing the current process, without performing any of the actual
// chdir/exec side effects itself.
package resume

import (
	"fmt"
	"os"

	"github.com/RHamers/claudenator/internal/session"
)

// Plan is the argv and directory to exec into, plus the PID of a
// conflicting already-running resume process, if any (0 means no conflict).
type Plan struct {
	Argv        []string
	Dir         string
	ConflictPID int
}

// Prepare validates that s.Dir still exists and checks findRunning for a
// process already attached to this session. It performs no side effects.
func Prepare(s session.Session, provider session.Provider, findRunning func(sessionID string) (pid int, found bool)) (*Plan, error) {
	if _, err := os.Stat(s.Dir); err != nil {
		return nil, fmt.Errorf("claudenator: directory no longer exists: %s", s.Dir)
	}
	plan := &Plan{
		Argv: provider.ResumeArgv(s),
		Dir:  s.Dir,
	}
	if pid, found := findRunning(s.ID); found {
		plan.ConflictPID = pid
	}
	return plan, nil
}
