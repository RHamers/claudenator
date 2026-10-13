package trash

import "github.com/RHamers/claudenator/internal/session"

// UndoStack holds sessions deleted during the current run, most recent last.
// It intentionally does not persist to disk: undo is per-run only.
type UndoStack struct {
	items []session.Session
}

func (u *UndoStack) Push(s session.Session) {
	u.items = append(u.items, s)
}

// Pop removes and returns the most recently pushed session. ok is false
// if the stack is empty.
func (u *UndoStack) Pop() (session.Session, bool) {
	if len(u.items) == 0 {
		return session.Session{}, false
	}
	last := u.items[len(u.items)-1]
	u.items = u.items[:len(u.items)-1]
	return last, true
}
