package trash

import (
	"testing"

	"github.com/RHamers/claudenator/internal/session"
)

func TestUndoStackPushPopOrder(t *testing.T) {
	var u UndoStack
	u.Push(session.Session{ID: "a"})
	u.Push(session.Session{ID: "b"})

	got, ok := u.Pop()
	if !ok || got.ID != "b" {
		t.Fatalf("expected b, got %+v (ok=%v)", got, ok)
	}
	got, ok = u.Pop()
	if !ok || got.ID != "a" {
		t.Fatalf("expected a, got %+v (ok=%v)", got, ok)
	}
}

func TestUndoStackPopEmptyIsSafeNoop(t *testing.T) {
	var u UndoStack
	_, ok := u.Pop()
	if ok {
		t.Fatalf("expected ok=false on empty stack")
	}
}
