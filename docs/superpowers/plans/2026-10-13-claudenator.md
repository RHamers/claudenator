# claudenator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a self-contained Go binary, `claudenator`, that lists Claude Code sessions across every project directory on the machine in a vim-keybound TUI, and resumes the selected one in its original working directory by replacing the current process.

**Architecture:** A `session.Provider` interface isolates the Claude Code on-disk format from everything else (UI, trash, resume), so Codex/Gemini support later is a new provider package, not a rewrite. The Bubble Tea UI is a single `Model` built up incrementally (navigation/sort → filter → delete/undo → preview/resume) over several tasks, each independently testable via `Update()` state transitions — no real terminal or rendering needed in tests. `main.go` is thin glue: build providers, run the TUI, then `os.Chdir` + `syscall.Exec` on whatever plan the TUI handed back.

**Tech Stack:** Go (stdlib `encoding/json`, `os`, `path/filepath`, `syscall`, `time`), `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/bubbles` (table, viewport, textinput), `github.com/charmbracelet/lipgloss`, `github.com/sahilm/fuzzy`.

**Spec:** `docs/superpowers/specs/2026-10-13-claudenator-design.md`

## Global Constraints

- No CGO; `go build ./...` must produce a static binary (spec: "Build & distribution").
- Install path is `go install` onto the local `PATH` (spec: "Build & distribution").
- Keybindings are vim-style and exact: normal mode `h`/`j`/`k`/`l`, `gg`/`G`, `s` (sort cycle), `d` (delete), `u` (undo), `Enter` (resume), `/` or `f` (enter filter mode), `q`/`Ctrl+C` (quit); filter mode: typing filters live, `Enter` applies and returns to normal mode, `Esc` clears and returns to normal mode (spec: "TUI design").
- Trash lives at `~/.claudenator/trash/`; entries older than 30 days are pruned silently on startup (spec: "Soft-delete / trash").
- Undo is in-memory, scoped to the current run only — no persistence across restarts (spec: "Soft-delete / trash", confirmed with user).
- `session.Provider` is the only extension point for future Codex/Gemini support; no provider-specific code may leak into `internal/ui`, `internal/trash`, or `internal/resume` (spec: "Architecture").
- No CI/release pipeline in v1 (spec: "Build & distribution").

## Review Focus

- A session's directory no longer exists on disk — resuming it must show an inline error, not attempt the chdir/exec (spec: "Resume mechanics"). Covered in Task 4 and Task 8.
- A session's `.jsonl` file has a corrupt or truncated last line — the parser must skip it and still return a usable session (or cleanly omit it), never crash the whole list (spec: "Session data source", "Testing"). Covered in Task 2.
- Deleting a session that is currently running must be blocked with an inline message, not allowed to proceed (spec: "Soft-delete / trash"). Covered in Task 7.
- Resuming a session that is already running elsewhere must prompt for confirmation before attaching, to avoid corrupting the session file (spec: "Resume mechanics"). Covered in Task 4 and Task 8.
- Pressing undo (`u`) with nothing deleted this run must be a safe no-op, not a panic (spec: "Soft-delete / trash" — undo is per-run only, so an empty stack is the normal starting state). Covered in Task 1 and Task 7.

---

## Task 1: Session types + trash store/undo stack

**Files:**
- Create: `go.mod`
- Create: `internal/session/session.go`
- Create: `internal/session/provider.go`
- Create: `internal/trash/trash.go`
- Create: `internal/trash/undo.go`
- Test: `internal/trash/trash_test.go`
- Test: `internal/trash/undo_test.go`

**Interfaces:**
- Produces: `session.Session{ID, Provider, Dir, Branch, LastActive time.Time, LastMessage, Running bool, FilePath string}`
- Produces: `session.Provider` interface: `List() ([]Session, error)`, `ResumeArgv(s Session) []string`, `Delete(s Session) error`
- Produces: `trash.Store{Dir string}` with `Move(sess session.Session) error`, `Restore(sessionID string) error`, `Prune(maxAge time.Duration, now time.Time) error`
- Produces: `trash.TrashPath(dir, sessionID string) string`, `trash.MetaPath(dir, sessionID string) string`
- Produces: `trash.UndoStack` with `Push(s session.Session)`, `Pop() (session.Session, bool)`

- [ ] **Step 1: Scaffold the module**

```bash
cd /home/plcbo/Projects/claudenator
go mod init github.com/RHamers/claudenator
```

This creates `go.mod` with `module github.com/RHamers/claudenator` and the local Go toolchain's version line. No edits needed yet.

- [ ] **Step 2: Write `session.Session` and `session.Provider`**

`internal/session/session.go`:

```go
package session

import "time"

// Session represents a single coding-agent session discovered on disk.
type Session struct {
	ID          string    // unique session identifier (provider-specific format)
	Provider    string    // e.g. "claude-code"
	Dir         string    // working directory the session ran in
	Branch      string    // git branch checked out when the session was active
	LastActive  time.Time // timestamp of the most recent event in the session
	LastMessage string    // preview text of the last user message
	Running     bool      // true if a resume process for this session is currently active
	FilePath    string    // absolute path to the session's on-disk file
}
```

`internal/session/provider.go`:

```go
package session

// Provider discovers and manages sessions for one coding-agent tool.
// It is the only extension point for supporting tools beyond Claude Code.
type Provider interface {
	// List returns every session this provider can find.
	List() ([]Session, error)
	// ResumeArgv returns the argv that resumes this session as a new process.
	ResumeArgv(s Session) []string
	// Delete removes the session's on-disk file (expected to move it to a
	// trash location, not destroy it outright).
	Delete(s Session) error
}
```

These two files have no independent behavior to test — they compile as part of the `trash` package tests below, which is where their first real usage appears.

- [ ] **Step 3: Write failing tests for `trash.Store`**

`internal/trash/trash_test.go`:

```go
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
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test ./internal/trash/... -run TestStore -v`
Expected: FAIL — `Store`, `TrashPath`, `MetaPath`, `readMeta`, `writeMeta` undefined.

- [ ] **Step 5: Implement `trash.Store`**

`internal/trash/trash.go`:

```go
package trash

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RHamers/claudenator/internal/session"
)

// Store moves session files into and out of a trash directory, recording
// enough metadata (original path, deletion time) to restore or prune them.
type Store struct {
	Dir string
}

type metaRecord struct {
	OriginalPath string    `json:"originalPath"`
	DeletedAt    time.Time `json:"deletedAt"`
}

func TrashPath(dir, sessionID string) string {
	return filepath.Join(dir, sessionID+".jsonl")
}

func MetaPath(dir, sessionID string) string {
	return filepath.Join(dir, sessionID+".meta.json")
}

func readMeta(path string) (metaRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return metaRecord{}, err
	}
	var m metaRecord
	if err := json.Unmarshal(data, &m); err != nil {
		return metaRecord{}, err
	}
	return m, nil
}

func writeMeta(path string, m metaRecord) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Move relocates sess's on-disk file into the trash directory and records
// its original path so it can be restored later.
func (s Store) Move(sess session.Session) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	if err := os.Rename(sess.FilePath, TrashPath(s.Dir, sess.ID)); err != nil {
		return err
	}
	return writeMeta(MetaPath(s.Dir, sess.ID), metaRecord{
		OriginalPath: sess.FilePath,
		DeletedAt:    time.Now(),
	})
}

// Restore moves a trashed session's file back to its original path.
func (s Store) Restore(sessionID string) error {
	meta, err := readMeta(MetaPath(s.Dir, sessionID))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(meta.OriginalPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(TrashPath(s.Dir, sessionID), meta.OriginalPath); err != nil {
		return err
	}
	return os.Remove(MetaPath(s.Dir, sessionID))
}

// Prune permanently deletes trashed sessions older than maxAge.
func (s Store) Prune(maxAge time.Duration, now time.Time) error {
	matches, err := filepath.Glob(filepath.Join(s.Dir, "*.meta.json"))
	if err != nil {
		return err
	}
	for _, metaPath := range matches {
		meta, err := readMeta(metaPath)
		if err != nil {
			continue
		}
		if now.Sub(meta.DeletedAt) <= maxAge {
			continue
		}
		id := strings.TrimSuffix(filepath.Base(metaPath), ".meta.json")
		os.Remove(TrashPath(s.Dir, id))
		os.Remove(metaPath)
	}
	return nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/trash/... -run TestStore -v`
Expected: PASS

- [ ] **Step 7: Write failing tests for `trash.UndoStack`**

`internal/trash/undo_test.go`:

```go
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
```

- [ ] **Step 8: Run tests to verify they fail**

Run: `go test ./internal/trash/... -run TestUndoStack -v`
Expected: FAIL — `UndoStack` undefined.

- [ ] **Step 9: Implement `trash.UndoStack`**

`internal/trash/undo.go`:

```go
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
```

- [ ] **Step 10: Run all trash tests to verify they pass**

Run: `go test ./internal/trash/... -v`
Expected: PASS (all 4 tests)

- [ ] **Step 11: Commit**

```bash
git add go.mod internal/session internal/trash
git commit -m "feat: add session types and trash store/undo stack"
```

---

## Task 2: Claude Code session parser (provider)

**Files:**
- Create: `internal/session/claudecode/claudecode.go`
- Create: `internal/session/claudecode/testdata/happy/-home-user-proj/11111111-1111-1111-1111-111111111111.jsonl`
- Create: `internal/session/claudecode/testdata/missing-cwd/-home-user-proj/22222222-2222-2222-2222-222222222222.jsonl`
- Create: `internal/session/claudecode/testdata/corrupt-last-line/-home-user-proj/33333333-3333-3333-3333-333333333333.jsonl`
- Create: `internal/session/claudecode/testdata/empty/-home-user-proj/44444444-4444-4444-4444-444444444444.jsonl`
- Create: `internal/session/claudecode/testdata/with-nonjsonl-sibling/-home-user-proj/55555555-5555-5555-5555-555555555555.jsonl`
- Create: `internal/session/claudecode/testdata/with-nonjsonl-sibling/-home-user-proj/memory/notes.md`
- Test: `internal/session/claudecode/claudecode_test.go`

**Interfaces:**
- Consumes: `session.Session`, `session.Provider`, `trash.Store{Dir string}.Move(session.Session) error` (Task 1)
- Produces: `claudecode.ProviderName = "claude-code"`, `claudecode.New(projectsRoot string, trash trash.Store) *claudecode.Provider` implementing `session.Provider`

- [ ] **Step 1: Write fixture files**

`internal/session/claudecode/testdata/happy/-home-user-proj/11111111-1111-1111-1111-111111111111.jsonl`:

```
{"type":"system","cwd":"/home/user/proj","gitBranch":"main","sessionId":"11111111-1111-1111-1111-111111111111","timestamp":"2026-10-01T10:00:00Z"}
{"type":"user","message":{"role":"user","content":"refactor the auth middleware"},"cwd":"/home/user/proj","gitBranch":"main","sessionId":"11111111-1111-1111-1111-111111111111","timestamp":"2026-10-01T10:05:00Z"}
{"type":"user","message":{"role":"user","content":"now add tests for it"},"cwd":"/home/user/proj","gitBranch":"main","sessionId":"11111111-1111-1111-1111-111111111111","timestamp":"2026-10-01T10:10:00Z"}
```

`internal/session/claudecode/testdata/missing-cwd/-home-user-proj/22222222-2222-2222-2222-222222222222.jsonl`:

```
{"type":"system","sessionId":"22222222-2222-2222-2222-222222222222","timestamp":"2026-10-01T10:00:00Z"}
{"type":"user","message":{"role":"user","content":"hello"},"sessionId":"22222222-2222-2222-2222-222222222222","timestamp":"2026-10-01T10:05:00Z"}
```

`internal/session/claudecode/testdata/corrupt-last-line/-home-user-proj/33333333-3333-3333-3333-333333333333.jsonl`:

```
{"type":"system","cwd":"/home/user/proj","gitBranch":"main","sessionId":"33333333-3333-3333-3333-333333333333","timestamp":"2026-10-01T10:00:00Z"}
{"type":"user","message":{"role":"user","content":"last good message"},"cwd":"/home/user/proj","gitBranch":"main","sessionId":"33333333-3333-3333-3333-333333333333","timestamp":"2026-10-01T10:05:00Z"}
{"type":"user","message":{"role":"user","content":"trunc
```

(Note: the third line is deliberately invalid JSON — an unterminated string — to simulate a truncated write.)

`internal/session/claudecode/testdata/empty/-home-user-proj/44444444-4444-4444-4444-444444444444.jsonl`:

(zero-byte file — create with `touch` in the next step, not a text fixture)

`internal/session/claudecode/testdata/with-nonjsonl-sibling/-home-user-proj/55555555-5555-5555-5555-555555555555.jsonl`:

```
{"type":"system","cwd":"/home/user/proj","gitBranch":"main","sessionId":"55555555-5555-5555-5555-555555555555","timestamp":"2026-10-01T10:00:00Z"}
```

`internal/session/claudecode/testdata/with-nonjsonl-sibling/-home-user-proj/memory/notes.md`:

```
# not a session file
```

Create the empty fixture and directories:

```bash
mkdir -p internal/session/claudecode/testdata/empty/-home-user-proj
touch internal/session/claudecode/testdata/empty/-home-user-proj/44444444-4444-4444-4444-444444444444.jsonl
```

- [ ] **Step 2: Write failing tests**

`internal/session/claudecode/claudecode_test.go`:

```go
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
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/session/claudecode/... -v`
Expected: FAIL — `New`, `ProviderName` undefined.

- [ ] **Step 4: Implement the provider**

`internal/session/claudecode/claudecode.go`:

```go
package claudecode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/trash"
)

const ProviderName = "claude-code"

// Provider discovers Claude Code sessions under a ~/.claude/projects-style
// directory tree: <projectsRoot>/<escaped-cwd>/<session-id>.jsonl.
type Provider struct {
	ProjectsRoot string
	Trash        trash.Store
}

func New(projectsRoot string, t trash.Store) *Provider {
	return &Provider{ProjectsRoot: projectsRoot, Trash: t}
}

type rawLine struct {
	CWD       string      `json:"cwd"`
	Timestamp string      `json:"timestamp"`
	GitBranch string      `json:"gitBranch"`
	SessionID string      `json:"sessionId"`
	Message   *rawMessage `json:"message"`
}

type rawMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type rawContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString
	}
	var blocks []rawContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

func parseFile(path string) (session.Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return session.Session{}, err
	}
	trimmed := strings.TrimRight(string(data), "\n")
	if trimmed == "" {
		return session.Session{}, fmt.Errorf("claudecode: %s is empty", path)
	}

	var (
		dir, branch, lastMsg, sessionID string
		lastActive                      time.Time
		sawCWD                          bool
	)

	for _, line := range strings.Split(trimmed, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var raw rawLine
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue // skip corrupt/truncated line, keep scanning
		}
		if raw.SessionID != "" {
			sessionID = raw.SessionID
		}
		if raw.CWD != "" {
			dir = raw.CWD
			sawCWD = true
		}
		if raw.GitBranch != "" {
			branch = raw.GitBranch
		}
		if raw.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339, raw.Timestamp); err == nil {
				lastActive = t
			}
		}
		if raw.Message != nil && raw.Message.Role == "user" {
			if text := extractText(raw.Message.Content); text != "" {
				lastMsg = text
			}
		}
	}

	if !sawCWD {
		return session.Session{}, fmt.Errorf("claudecode: no cwd found in %s", path)
	}
	if sessionID == "" {
		sessionID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}

	return session.Session{
		ID:          sessionID,
		Provider:    ProviderName,
		Dir:         dir,
		Branch:      branch,
		LastActive:  lastActive,
		LastMessage: lastMsg,
		FilePath:    path,
	}, nil
}

func (p *Provider) List() ([]session.Session, error) {
	matches, err := filepath.Glob(filepath.Join(p.ProjectsRoot, "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	var sessions []session.Session
	for _, path := range matches {
		s, err := parseFile(path)
		if err != nil {
			continue // unreadable, empty, or missing cwd — not listable
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (p *Provider) ResumeArgv(s session.Session) []string {
	return []string{"claude", "--resume", s.ID}
}

func (p *Provider) Delete(s session.Session) error {
	return p.Trash.Move(s)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/session/claudecode/... -v`
Expected: PASS (all 6 tests)

- [ ] **Step 6: Commit**

```bash
git add internal/session/claudecode
git commit -m "feat: add Claude Code session provider with List/ResumeArgv/Delete"
```

---

## Task 3: Process scanner (double-attach detection)

**Files:**
- Create: `internal/procscan/procscan.go`
- Test: `internal/procscan/procscan_test.go`

**Interfaces:**
- Produces: `procscan.FindResumeProcess(procRoot, sessionID string) (pid int, found bool)`

- [ ] **Step 1: Write failing tests**

`internal/procscan/procscan_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/procscan/... -v`
Expected: FAIL — `FindResumeProcess` undefined.

- [ ] **Step 3: Implement the scanner**

`internal/procscan/procscan.go`:

```go
// Package procscan detects a running `claude --resume <id>` process by
// reading /proc/*/cmdline, so the UI can warn before double-attaching to
// a session that's already open elsewhere.
package procscan

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FindResumeProcess scans procRoot (normally "/proc") for a process whose
// argv contains both "--resume" and sessionID. It returns found=false,
// rather than an error, if procRoot can't be read — the guard is a
// best-effort safety check, not a hard dependency.
func FindResumeProcess(procRoot, sessionID string) (pid int, found bool) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return 0, false
	}
	for _, e := range entries {
		candidatePID, err := strconv.Atoi(e.Name())
		if err != nil {
			continue // not a PID directory
		}
		data, err := os.ReadFile(filepath.Join(procRoot, e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		args := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
		hasResume := false
		hasSessionID := false
		for _, a := range args {
			if a == "--resume" {
				hasResume = true
			}
			if a == sessionID {
				hasSessionID = true
			}
		}
		if hasResume && hasSessionID {
			return candidatePID, true
		}
	}
	return 0, false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/procscan/... -v`
Expected: PASS (all 3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/procscan
git commit -m "feat: add /proc-based scanner to detect running resume processes"
```

---

## Task 4: Resume plan preparation

**Files:**
- Create: `internal/resume/resume.go`
- Test: `internal/resume/resume_test.go`

**Interfaces:**
- Consumes: `session.Session`, `session.Provider` (Task 1)
- Produces: `resume.Plan{Argv []string, Dir string, ConflictPID int}`, `resume.Prepare(s session.Session, provider session.Provider, findRunning func(sessionID string) (pid int, found bool)) (*Plan, error)`

- [ ] **Step 1: Write failing tests**

`internal/resume/resume_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/resume/... -v`
Expected: FAIL — `Prepare`, `Plan` undefined.

- [ ] **Step 3: Implement `resume.Prepare`**

`internal/resume/resume.go`:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/resume/... -v`
Expected: PASS (all 3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/resume
git commit -m "feat: add resume plan preparation with dir-check and conflict detection"
```

---

## Task 5: TUI model — list, navigation, sort

**Files:**
- Create: `internal/ui/model.go`
- Create: `internal/ui/sort.go`
- Create: `internal/ui/format.go`
- Test: `internal/ui/model_test.go`

**Interfaces:**
- Consumes: `session.Session` (Task 1)
- Produces: `ui.New(sessions []session.Session, providers map[string]session.Provider, trashStore trash.Store, findRunning func(string) (int, bool)) Model`, `Model.Update(tea.Msg) (tea.Model, tea.Cmd)`, `Model.View() string`, `Model.selected() session.Session` (unexported, used by later tasks in this package)

- [ ] **Step 1: Add TUI dependencies**

```bash
go get github.com/charmbracelet/bubbletea@latest
go get github.com/charmbracelet/bubbles@latest
go get github.com/charmbracelet/lipgloss@latest
```

- [ ] **Step 2: Write failing tests**

`internal/ui/model_test.go`:

```go
package ui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/trash"
)

func testSessions() []session.Session {
	now := time.Now()
	return []session.Session{
		{ID: "a", Provider: "claude-code", Dir: "/z-dir", Branch: "main", LastActive: now},
		{ID: "b", Provider: "claude-code", Dir: "/a-dir", Branch: "zzz", LastActive: now.Add(-time.Hour)},
	}
}

func newTestModel() Model {
	providers := map[string]session.Provider{}
	findRunning := func(string) (int, bool) { return 0, false }
	return New(testSessions(), providers, trash.Store{}, findRunning)
}

func pressRune(m Model, r rune) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	return updated.(Model)
}

func TestNewSortsByLastActiveDescendingByDefault(t *testing.T) {
	m := newTestModel()
	if m.visible[0].ID != "a" {
		t.Fatalf("expected most recent session (a) first, got %q", m.visible[0].ID)
	}
}

func TestNavigationDownMovesCursor(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, 'j')
	if m.table.Cursor() != 1 {
		t.Fatalf("expected cursor 1, got %d", m.table.Cursor())
	}
}

func TestNavigationUpMovesCursorBack(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, 'j')
	m = pressRune(m, 'k')
	if m.table.Cursor() != 0 {
		t.Fatalf("expected cursor 0, got %d", m.table.Cursor())
	}
}

func TestGGJumpsToTop(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, 'j') // move to 1
	m = pressRune(m, 'g')
	m = pressRune(m, 'g')
	if m.table.Cursor() != 0 {
		t.Fatalf("expected cursor 0 after gg, got %d", m.table.Cursor())
	}
}

func TestCapitalGJumpsToBottom(t *testing.T) {
	m := newTestModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = updated.(Model)
	if m.table.Cursor() != 1 {
		t.Fatalf("expected cursor 1 after G, got %d", m.table.Cursor())
	}
}

func TestSortCyclesToDirThenBranchThenBack(t *testing.T) {
	m := newTestModel()

	m = pressRune(m, 's') // Dir ascending
	if m.visible[0].Dir != "/a-dir" {
		t.Fatalf("after first sort, expected /a-dir first, got %q", m.visible[0].Dir)
	}

	m = pressRune(m, 's') // Branch ascending
	if m.visible[0].Branch != "main" {
		t.Fatalf("after second sort, expected main first, got %q", m.visible[0].Branch)
	}

	m = pressRune(m, 's') // back to LastActive descending
	if m.visible[0].ID != "a" {
		t.Fatalf("after third sort, expected a first again, got %q", m.visible[0].ID)
	}
}

func TestNavigationOnEmptyListDoesNotPanic(t *testing.T) {
	providers := map[string]session.Provider{}
	findRunning := func(string) (int, bool) { return 0, false }
	m := New(nil, providers, trash.Store{}, findRunning)
	m = pressRune(m, 'j')
	m = pressRune(m, 'k')
	_ = m.selected() // must not panic
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/ui/... -v`
Expected: FAIL — `New`, `Model` undefined.

- [ ] **Step 4: Implement `format.go` and `sort.go`**

`internal/ui/format.go`:

```go
package ui

import (
	"fmt"
	"time"
)

func humanize(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
```

`internal/ui/sort.go`:

```go
package ui

import (
	"sort"

	"github.com/RHamers/claudenator/internal/session"
)

type SortColumn int

const (
	SortByLastActive SortColumn = iota
	SortByDir
	SortByBranch
)

func (c SortColumn) Next() SortColumn {
	return (c + 1) % 3
}

func (c SortColumn) String() string {
	switch c {
	case SortByDir:
		return "Dir"
	case SortByBranch:
		return "Branch"
	default:
		return "Last edited"
	}
}

func sortSessions(sessions []session.Session, col SortColumn) {
	sort.SliceStable(sessions, func(i, j int) bool {
		switch col {
		case SortByDir:
			return sessions[i].Dir < sessions[j].Dir
		case SortByBranch:
			return sessions[i].Branch < sessions[j].Branch
		default:
			return sessions[i].LastActive.After(sessions[j].LastActive)
		}
	})
}
```

- [ ] **Step 5: Implement `model.go`**

`internal/ui/model.go`:

```go
package ui

import (
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/trash"
)

type Model struct {
	all         []session.Session
	visible     []session.Session
	table       table.Model
	sortCol     SortColumn
	pendingG    bool
	providers   map[string]session.Provider
	trashStore  trash.Store
	findRunning func(string) (int, bool)
}

func columns() []table.Column {
	return []table.Column{
		{Title: "Dir", Width: 40},
		{Title: "Branch", Width: 16},
		{Title: "Last edited", Width: 16},
	}
}

func rowsFor(sessions []session.Session) []table.Row {
	rows := make([]table.Row, len(sessions))
	for i, s := range sessions {
		rows[i] = table.Row{s.Dir, s.Branch, humanize(s.LastActive)}
	}
	return rows
}

func New(sessions []session.Session, providers map[string]session.Provider, trashStore trash.Store, findRunning func(string) (int, bool)) Model {
	m := Model{
		all:         append([]session.Session(nil), sessions...),
		sortCol:     SortByLastActive,
		providers:   providers,
		trashStore:  trashStore,
		findRunning: findRunning,
	}
	m.visible = append([]session.Session(nil), m.all...)
	sortSessions(m.visible, m.sortCol)
	m.table = table.New(
		table.WithColumns(columns()),
		table.WithRows(rowsFor(m.visible)),
		table.WithFocused(true),
	)
	return m
}

func (m Model) selected() session.Session {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.visible) {
		return session.Session{}
	}
	return m.visible[i]
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	return m.updateNormalMode(keyMsg)
}

func (m Model) updateNormalMode(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := keyMsg.String()
	wasPendingG := m.pendingG
	m.pendingG = false

	switch key {
	case "j", "down":
		m.table.MoveDown(1)
	case "k", "up":
		m.table.MoveUp(1)
	case "g":
		if wasPendingG {
			m.table.GotoTop()
		} else {
			m.pendingG = true
		}
	case "G":
		m.table.GotoBottom()
	case "s":
		m.sortCol = m.sortCol.Next()
		sortSessions(m.visible, m.sortCol)
		m.table.SetRows(rowsFor(m.visible))
	}
	return m, nil
}

func (m Model) View() string {
	return m.table.View()
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/ui/... -v`
Expected: PASS (all 7 tests)

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/ui
git commit -m "feat: add TUI model with vim-style navigation and column sort"
```

---

## Task 6: Filter mode (fuzzy)

**Files:**
- Create: `internal/ui/filter.go`
- Modify: `internal/ui/model.go`
- Test: `internal/ui/filter_test.go`

**Interfaces:**
- Consumes: `session.Session` (Task 1), `Model` fields from Task 5
- Produces: `applyFilter(all []session.Session, query string) []session.Session`; `Model` gains filter-mode key handling (`/`, `f`, `Enter`, `Esc`)

- [ ] **Step 1: Add fuzzy matching dependency**

```bash
go get github.com/sahilm/fuzzy@latest
go get github.com/charmbracelet/bubbles@latest # ensure textinput is available (already a transitive dep from Task 5)
```

- [ ] **Step 2: Write failing tests**

`internal/ui/filter_test.go`:

```go
package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/RHamers/claudenator/internal/session"
)

func pressRunes(m Model, s string) Model {
	for _, r := range s {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	return m
}

func pressKey(m Model, t tea.KeyType) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: t})
	return updated.(Model)
}

func TestApplyFilterNarrowsToMatches(t *testing.T) {
	all := []session.Session{
		{ID: "a", Dir: "/home/user/claudenator", Branch: "main"},
		{ID: "b", Dir: "/home/user/other-project", Branch: "main"},
	}
	result := applyFilter(all, "claudenator")
	if len(result) != 1 || result[0].ID != "a" {
		t.Fatalf("expected only session a, got %+v", result)
	}
}

func TestApplyFilterEmptyQueryReturnsAll(t *testing.T) {
	all := []session.Session{{ID: "a"}, {ID: "b"}}
	result := applyFilter(all, "")
	if len(result) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(result))
	}
}

func TestSlashEntersFilterModeAndNarrowsLive(t *testing.T) {
	m := newTestModel() // sessions a(/z-dir) and b(/a-dir), from Task 5's helper
	m = pressRune(m, '/')
	m = pressRunes(m, "a-dir")
	if len(m.visible) != 1 || m.visible[0].Dir != "/a-dir" {
		t.Fatalf("expected only /a-dir visible, got %+v", m.visible)
	}
}

func TestFEntersFilterModeToo(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, 'f')
	m = pressRunes(m, "a-dir")
	if len(m.visible) != 1 || m.visible[0].Dir != "/a-dir" {
		t.Fatalf("expected only /a-dir visible, got %+v", m.visible)
	}
}

func TestEnterInFilterModeKeepsFilterAndReturnsToNormalMode(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, '/')
	m = pressRunes(m, "a-dir")
	m = pressKey(m, tea.KeyEnter)
	if m.mode != modeNormal {
		t.Fatalf("expected normal mode after Enter")
	}
	if len(m.visible) != 1 {
		t.Fatalf("expected filter to remain applied, got %d visible", len(m.visible))
	}
	// Normal-mode keys work again.
	m = pressRune(m, 'j')
}

func TestEscInFilterModeClearsFilterAndReturnsToNormalMode(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, '/')
	m = pressRunes(m, "a-dir")
	m = pressKey(m, tea.KeyEsc)
	if m.mode != modeNormal {
		t.Fatalf("expected normal mode after Esc")
	}
	if len(m.visible) != 2 {
		t.Fatalf("expected filter cleared (both sessions visible), got %d", len(m.visible))
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/ui/... -v`
Expected: FAIL — `applyFilter`, `modeNormal` undefined.

- [ ] **Step 4: Implement `filter.go`**

`internal/ui/filter.go`:

```go
package ui

import (
	"github.com/sahilm/fuzzy"

	"github.com/RHamers/claudenator/internal/session"
)

func applyFilter(all []session.Session, query string) []session.Session {
	if query == "" {
		return append([]session.Session(nil), all...)
	}
	sources := make([]string, len(all))
	for i, s := range all {
		sources[i] = s.Dir + " " + s.Branch + " " + s.LastMessage
	}
	matches := fuzzy.Find(query, sources)
	result := make([]session.Session, len(matches))
	for i, match := range matches {
		result[i] = all[match.Index]
	}
	return result
}
```

- [ ] **Step 5: Modify `model.go` to add filter mode**

Add imports `"github.com/charmbracelet/bubbles/textinput"` to `internal/ui/model.go`, add fields to `Model`, initialize `filterInput` in `New`, and split `Update` into mode dispatch:

```go
type mode int

const (
	modeNormal mode = iota
	modeFilter
)
```

Add to the `Model` struct: `mode mode` and `filterInput textinput.Model`.

In `New`, after building `m.table`, add:

```go
	m.filterInput = textinput.New()
	m.filterInput.Prompt = "Filter: "
```

Replace the body of `Update` with:

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if m.mode == modeFilter {
		return m.updateFilterMode(keyMsg)
	}
	return m.updateNormalMode(keyMsg)
}

func (m *Model) refreshVisible() {
	m.visible = applyFilter(m.all, m.filterInput.Value())
	sortSessions(m.visible, m.sortCol)
	m.table.SetRows(rowsFor(m.visible))
}

func (m Model) updateFilterMode(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.Type {
	case tea.KeyEnter:
		m.mode = modeNormal
		return m, nil
	case tea.KeyEsc:
		m.filterInput.SetValue("")
		m.mode = modeNormal
		m.refreshVisible()
		return m, nil
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(keyMsg)
	m.refreshVisible()
	return m, cmd
}
```

Add `"/", "f":` as a new case in `updateNormalMode`'s switch:

```go
	case "/", "f":
		m.mode = modeFilter
		m.filterInput.Focus()
```

Note: `refreshVisible` must take a **pointer** receiver (`func (m *Model) refreshVisible()`), not a value receiver. `Update`'s local variable `m` is an addressable `Model` value, so calling `m.refreshVisible()` on it auto-takes its address and the mutation lands directly on that local variable — which is what the function then returns. A value-receiver version would mutate a throwaway copy and silently leave the caller's `m` unchanged, so `go vet`/manual review should treat a value receiver here as a bug, not a style choice.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/ui/... -v`
Expected: PASS (all 13 tests across Task 5 + Task 6)

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/ui
git commit -m "feat: add vim-style fuzzy filter mode (/ or f)"
```

---

## Task 7: Delete + undo wiring

**Files:**
- Modify: `internal/ui/model.go`
- Create: `internal/ui/confirm.go`
- Test: `internal/ui/delete_test.go`

**Interfaces:**
- Consumes: `session.Provider.Delete` (Task 1/2), `trash.Store.Restore`, `trash.UndoStack` (Task 1)
- Produces: `Model` gains `d` (delete with confirm, blocked if running) and `u` (undo) handling

- [ ] **Step 1: Write failing tests**

`internal/ui/delete_test.go`:

```go
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
```

Also update `newTestModel` in `internal/ui/model_test.go` (from Task 5) to pass `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}` via the existing `pressRune` helper — no change needed there, but confirm `pressRune`'s signature already matches (it does, from Task 5).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/... -v`
Expected: FAIL — `m.confirm`, `m.err` undefined.

- [ ] **Step 3: Implement `confirm.go`**

`internal/ui/confirm.go`:

```go
package ui

import "github.com/RHamers/claudenator/internal/session"

type confirmKind int

const (
	confirmDelete confirmKind = iota
	confirmResumeConflict
)

type confirmState struct {
	kind   confirmKind
	target session.Session
	pid    int
}
```

- [ ] **Step 4: Modify `model.go`**

Add fields to `Model`: `confirm *confirmState` and `err string`.

Add to `updateNormalMode`, as the first thing in the function body (before the `wasPendingG` logic), a check that routes to confirm handling when a confirm is pending:

```go
func (m Model) updateNormalMode(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirm != nil {
		return m.updateConfirm(keyMsg)
	}

	key := keyMsg.String()
	wasPendingG := m.pendingG
	m.pendingG = false

	switch key {
	case "j", "down":
		m.table.MoveDown(1)
	case "k", "up":
		m.table.MoveUp(1)
	case "g":
		if wasPendingG {
			m.table.GotoTop()
		} else {
			m.pendingG = true
		}
	case "G":
		m.table.GotoBottom()
	case "s":
		m.sortCol = m.sortCol.Next()
		sortSessions(m.visible, m.sortCol)
		m.table.SetRows(rowsFor(m.visible))
	case "/", "f":
		m.mode = modeFilter
		m.filterInput.Focus()
	case "d":
		return m.beginDelete()
	case "u":
		return m.undo()
	}
	return m, nil
}

func (m Model) beginDelete() (tea.Model, tea.Cmd) {
	sel := m.selected()
	if sel.ID == "" {
		return m, nil // empty list, nothing to delete
	}
	if pid, running := m.findRunning(sel.ID); running {
		m.err = fmt.Sprintf("session %s is running (pid %d) — cannot delete", sel.ID, pid)
		return m, nil
	}
	m.err = ""
	m.confirm = &confirmState{kind: confirmDelete, target: sel}
	return m, nil
}

func (m Model) undo() (tea.Model, tea.Cmd) {
	s, ok := m.undoStack.Pop()
	if !ok {
		return m, nil
	}
	if err := m.trashStore.Restore(s.ID); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.all = append(m.all, s)
	m.refreshVisible()
	return m, nil
}

func (m Model) updateConfirm(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.String() {
	case "y":
		c := *m.confirm
		m.confirm = nil
		if c.kind == confirmDelete {
			return m.performDelete(c.target)
		}
	case "n":
		m.confirm = nil
	}
	return m, nil
}

func (m Model) performDelete(s session.Session) (tea.Model, tea.Cmd) {
	provider, ok := m.providers[s.Provider]
	if !ok {
		m.err = fmt.Sprintf("no provider registered for %q", s.Provider)
		return m, nil
	}
	if err := provider.Delete(s); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.all = removeByID(m.all, s.ID)
	m.undoStack.Push(s)
	m.refreshVisible()
	return m, nil
}

func removeByID(sessions []session.Session, id string) []session.Session {
	result := make([]session.Session, 0, len(sessions))
	for _, s := range sessions {
		if s.ID != id {
			result = append(result, s)
		}
	}
	return result
}
```

Add `"fmt"` to the import block and `undoStack trash.UndoStack` to the `Model` struct. Task 7's `updateConfirm` only handles `confirmDelete` — Task 8 extends this same `switch` to add a `confirmResumeConflict` case once `confirmState.plan` exists. Do not add `pendingResume` or the `resume` import yet; those belong to Task 8.

Update `New` to initialize `undoStack` implicitly (zero value `trash.UndoStack{}` is already usable — no explicit init needed).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/ui/... -v`
Expected: PASS (all tests from Tasks 5–7)

- [ ] **Step 6: Commit**

```bash
git add internal/ui
git commit -m "feat: wire delete (d) with confirm/running-guard and undo (u)"
```

---

## Task 8: Preview pane + resume wiring

**Files:**
- Modify: `internal/ui/model.go`
- Modify: `internal/ui/confirm.go`
- Test: `internal/ui/preview_test.go`
- Test: `internal/ui/resume_test.go`

**Interfaces:**
- Consumes: `resume.Prepare`, `resume.Plan` (Task 4)
- Produces: `Model` gains `h`/`l` pane focus toggle, preview scrolling, `Enter` resume handling, `Model.PendingResume() *resume.Plan`

- [ ] **Step 1: Write failing tests**

`internal/ui/preview_test.go`:

```go
package ui

import (
	"testing"
)

func TestHLTogglesFocusBetweenListAndPreview(t *testing.T) {
	m := newTestModel()
	if m.focus != focusList {
		t.Fatalf("expected initial focus on list")
	}
	m = pressRune(m, 'l')
	if m.focus != focusPreview {
		t.Fatalf("expected focus on preview after l")
	}
	m = pressRune(m, 'h')
	if m.focus != focusList {
		t.Fatalf("expected focus back on list after h")
	}
}

func TestJKMovesSelectionWhenListFocused(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, 'j')
	if m.table.Cursor() != 1 {
		t.Fatalf("expected cursor 1, got %d", m.table.Cursor())
	}
}

func TestJKScrollsPreviewWhenPreviewFocused(t *testing.T) {
	m := newTestModel()
	m = pressRune(m, 'l') // focus preview
	cursorBefore := m.table.Cursor()
	m = pressRune(m, 'j')
	if m.table.Cursor() != cursorBefore {
		t.Fatalf("expected list cursor unchanged while preview focused, got %d want %d", m.table.Cursor(), cursorBefore)
	}
}

func TestPreviewContentMatchesSelectedSessionLastMessage(t *testing.T) {
	sessions := testSessions()
	sessions[0].LastMessage = "refactor the auth middleware"
	providers := map[string]session.Provider{}
	findRunning := func(string) (int, bool) { return 0, false }
	m := New(sessions, providers, trash.Store{}, findRunning)

	if m.preview.View() == "" {
		t.Fatalf("expected non-empty preview for selected session")
	}
}
```

Note: `testSessions`, `session`, and `trash` are already imported/available via `model_test.go` in the same package — add `"github.com/RHamers/claudenator/internal/session"` and `"github.com/RHamers/claudenator/internal/trash"` imports to `preview_test.go` only if the Go compiler flags them as unused in this file specifically (package-level test files share nothing but the package name, each file needs its own imports). Add both imports to `preview_test.go`.

`internal/ui/resume_test.go`:

```go
package ui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/trash"
)

type stubProvider struct{}

func (stubProvider) List() ([]session.Session, error) { return nil, nil }
func (stubProvider) ResumeArgv(s session.Session) []string {
	return []string{"claude", "--resume", s.ID}
}
func (stubProvider) Delete(s session.Session) error { return nil }

func newResumeTestModel(t *testing.T, findRunning func(string) (int, bool)) Model {
	t.Helper()
	dir := t.TempDir()
	sessions := []session.Session{
		{ID: "abc", Provider: "stub", Dir: dir, Branch: "main"},
	}
	providers := map[string]session.Provider{"stub": stubProvider{}}
	return New(sessions, providers, trash.Store{}, findRunning)
}

func TestEnterNoConflictSetsPendingResumeAndQuits(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 0, false })

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.PendingResume() == nil {
		t.Fatalf("expected pending resume plan")
	}
	if cmd == nil {
		t.Fatalf("expected tea.Quit command")
	}
}

func TestEnterConflictOpensConfirm(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 555, true })

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	if m.confirm == nil || m.confirm.kind != confirmResumeConflict {
		t.Fatalf("expected resume-conflict confirm pending")
	}
	if m.PendingResume() != nil {
		t.Fatalf("expected no pending resume before confirming")
	}
}

func TestEnterConflictYConfirmsResume(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 555, true })
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	m3, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = m3.(Model)

	if m.PendingResume() == nil {
		t.Fatalf("expected pending resume after confirming")
	}
	if cmd == nil {
		t.Fatalf("expected tea.Quit command")
	}
}

func TestEnterConflictNCancelsResume(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 555, true })
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = m3.(Model)

	if m.confirm != nil {
		t.Fatalf("expected confirm cleared")
	}
	if m.PendingResume() != nil {
		t.Fatalf("expected no pending resume")
	}
}

func TestEnterMissingDirShowsErrorNotQuit(t *testing.T) {
	missing := "/this/path/does/not/exist/claudenator-test"
	os.RemoveAll(missing) // ensure it really doesn't exist
	sessions := []session.Session{{ID: "abc", Provider: "stub", Dir: missing}}
	providers := map[string]session.Provider{"stub": stubProvider{}}
	m := New(sessions, providers, trash.Store{}, func(string) (int, bool) { return 0, false })

	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	if m.err == "" {
		t.Fatalf("expected inline error for missing directory")
	}
	if cmd != nil {
		t.Fatalf("expected no quit command when directory is missing")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ui/... -v`
Expected: FAIL — `m.focus`, `m.preview`, `focusList`, `m.PendingResume` undefined.

- [ ] **Step 3: Extend `confirm.go`**

Add a `plan *resume.Plan` field to `confirmState` and the matching import:

```go
package ui

import (
	"github.com/RHamers/claudenator/internal/resume"
	"github.com/RHamers/claudenator/internal/session"
)

type confirmKind int

const (
	confirmDelete confirmKind = iota
	confirmResumeConflict
)

type confirmState struct {
	kind   confirmKind
	target session.Session
	pid    int
	plan   *resume.Plan
}
```

- [ ] **Step 4: Modify `model.go`**

Add imports `"github.com/charmbracelet/bubbles/viewport"` and `"github.com/RHamers/claudenator/internal/resume"`.

Add to `Model`: `preview viewport.Model`, `focus focusPane`, `pendingResume *resume.Plan`.

Add the `focusPane` type near the `mode` type:

```go
type focusPane int

const (
	focusList focusPane = iota
	focusPreview
)
```

In `New`, after setting up `m.filterInput`, add:

```go
	m.preview = viewport.New(80, 6)
	m.refreshPreview()
```

Add `refreshPreview` and update `refreshVisible` to call it. Both take a **pointer** receiver, for the same reason noted in Task 6: a value receiver would mutate a throwaway copy and the caller's `m` would silently see no change.

```go
func (m *Model) refreshPreview() {
	m.preview.SetContent(m.selected().LastMessage)
}
```

Update `refreshVisible` (from Task 6) to also call `m.refreshPreview()` as its last line (it already has a pointer receiver, so this is a direct call, not a re-addressing), and update `updateNormalMode`'s `j`/`k`/`g`/`gg`/`G` cases to refresh the preview and respect focus. Replace the whole `updateNormalMode` function with:

```go
func (m Model) updateNormalMode(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.confirm != nil {
		return m.updateConfirm(keyMsg)
	}

	key := keyMsg.String()
	wasPendingG := m.pendingG
	m.pendingG = false

	switch key {
	case "j", "down":
		if m.focus == focusPreview {
			m.preview.LineDown(1)
		} else {
			m.table.MoveDown(1)
			m.refreshPreview()
		}
	case "k", "up":
		if m.focus == focusPreview {
			m.preview.LineUp(1)
		} else {
			m.table.MoveUp(1)
			m.refreshPreview()
		}
	case "g":
		if wasPendingG {
			m.table.GotoTop()
			m.refreshPreview()
		} else {
			m.pendingG = true
		}
	case "G":
		m.table.GotoBottom()
		m.refreshPreview()
	case "h":
		m.focus = focusList
	case "l":
		m.focus = focusPreview
	case "s":
		m.sortCol = m.sortCol.Next()
		sortSessions(m.visible, m.sortCol)
		m.table.SetRows(rowsFor(m.visible))
		m.refreshPreview()
	case "/", "f":
		m.mode = modeFilter
		m.filterInput.Focus()
	case "d":
		return m.beginDelete()
	case "u":
		return m.undo()
	case "enter":
		return m.beginResume()
	}
	return m, nil
}

func (m Model) beginResume() (tea.Model, tea.Cmd) {
	sel := m.selected()
	if sel.ID == "" {
		return m, nil
	}
	provider, ok := m.providers[sel.Provider]
	if !ok {
		m.err = fmt.Sprintf("no provider registered for %q", sel.Provider)
		return m, nil
	}
	plan, err := resume.Prepare(sel, provider, m.findRunning)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.err = ""
	if plan.ConflictPID != 0 {
		m.confirm = &confirmState{kind: confirmResumeConflict, target: sel, pid: plan.ConflictPID, plan: plan}
		return m, nil
	}
	m.pendingResume = plan
	return m, tea.Quit
}

func (m Model) PendingResume() *resume.Plan {
	return m.pendingResume
}
```

Update `updateConfirm` to also handle the resume-conflict branch:

```go
func (m Model) updateConfirm(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.String() {
	case "y":
		c := *m.confirm
		m.confirm = nil
		switch c.kind {
		case confirmDelete:
			return m.performDelete(c.target)
		case confirmResumeConflict:
			m.pendingResume = c.plan
			return m, tea.Quit
		}
	case "n":
		m.confirm = nil
	}
	return m, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/ui/... -v`
Expected: PASS (all tests from Tasks 5–8)

- [ ] **Step 6: Commit**

```bash
git add internal/ui
git commit -m "feat: add preview pane (h/l focus) and Enter-to-resume with conflict confirm"
```

---

## Task 9: main.go wiring

**Files:**
- Create: `main.go`
- Test: `main_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1–8
- Produces: the `claudenator` binary's entry point

- [ ] **Step 1: Write failing tests**

`main_test.go`:

```go
package main

import (
	"testing"

	"github.com/RHamers/claudenator/internal/session"
)

type fakeAggregateProvider struct {
	sessions []session.Session
	err      error
}

func (p fakeAggregateProvider) List() ([]session.Session, error) { return p.sessions, p.err }
func (p fakeAggregateProvider) ResumeArgv(s session.Session) []string {
	return []string{"claude", "--resume", s.ID}
}
func (p fakeAggregateProvider) Delete(s session.Session) error { return nil }

func TestAggregateSessionsMergesProviders(t *testing.T) {
	providers := map[string]session.Provider{
		"a": fakeAggregateProvider{sessions: []session.Session{{ID: "1"}}},
		"b": fakeAggregateProvider{sessions: []session.Session{{ID: "2"}}},
	}
	sessions, err := aggregateSessions(providers)
	if err != nil {
		t.Fatalf("aggregateSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestAggregateSessionsReturnsPartialResultsOnProviderError(t *testing.T) {
	providers := map[string]session.Provider{
		"a": fakeAggregateProvider{sessions: []session.Session{{ID: "1"}}},
		"b": fakeAggregateProvider{err: errTestProvider},
	}
	sessions, err := aggregateSessions(providers)
	if err == nil {
		t.Fatalf("expected an error from the failing provider")
	}
	if len(sessions) != 1 {
		t.Fatalf("expected the successful provider's session to still be returned, got %d", len(sessions))
	}
}

func TestBuildProvidersRegistersClaudeCode(t *testing.T) {
	providers, _ := buildProviders(t.TempDir())
	if _, ok := providers["claude-code"]; !ok {
		t.Fatalf("expected claude-code provider registered")
	}
}
```

Add a small sentinel error for the second test, in the same test file:

```go
var errTestProvider = fmtErrorf("boom")

func fmtErrorf(msg string) error {
	return &testErr{msg}
}

type testErr struct{ msg string }

func (e *testErr) Error() string { return e.msg }
```

(This avoids importing `fmt` or `errors` into the test just for one sentinel value; either is fine — using a tiny local type keeps the test file dependency-free.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -v`
Expected: FAIL — `aggregateSessions`, `buildProviders` undefined.

- [ ] **Step 3: Implement `main.go`**

```go
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/RHamers/claudenator/internal/procscan"
	"github.com/RHamers/claudenator/internal/session"
	"github.com/RHamers/claudenator/internal/session/claudecode"
	"github.com/RHamers/claudenator/internal/trash"
	"github.com/RHamers/claudenator/internal/ui"
)

func buildProviders(home string) (map[string]session.Provider, trash.Store) {
	trashStore := trash.Store{Dir: filepath.Join(home, ".claudenator", "trash")}
	projectsRoot := filepath.Join(home, ".claude", "projects")
	providers := map[string]session.Provider{
		claudecode.ProviderName: claudecode.New(projectsRoot, trashStore),
	}
	return providers, trashStore
}

func aggregateSessions(providers map[string]session.Provider) ([]session.Session, error) {
	var all []session.Session
	var errs []error
	for _, p := range providers {
		sessions, err := p.List()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		all = append(all, sessions...)
	}
	if len(errs) > 0 {
		return all, errors.Join(errs...)
	}
	return all, nil
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	providers, trashStore := buildProviders(home)

	if err := trashStore.Prune(30*24*time.Hour, time.Now()); err != nil {
		fmt.Fprintln(os.Stderr, "warning: trash prune failed:", err)
	}

	sessions, err := aggregateSessions(providers)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: some sessions could not be listed:", err)
	}

	findRunning := func(id string) (int, bool) {
		return procscan.FindResumeProcess("/proc", id)
	}

	m := ui.New(sessions, providers, trashStore, findRunning)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	final, ok := finalModel.(ui.Model)
	if !ok {
		return
	}
	plan := final.PendingResume()
	if plan == nil {
		return
	}

	if err := os.Chdir(plan.Dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	bin, err := exec.LookPath(plan.Argv[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := syscall.Exec(bin, plan.Argv, os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test . -v`
Expected: PASS (all 3 tests)

- [ ] **Step 5: Build the binary and sanity-check it starts**

```bash
go build ./...
go vet ./...
go install .
```

Expected: both commands succeed with no output; `claudenator` (built from `main.go`, module `github.com/RHamers/claudenator`) is now on `$GOPATH/bin` (or `$GOBIN`).

- [ ] **Step 6: Manual smoke test**

Run `claudenator` for real, against your actual `~/.claude/projects` sessions:

```bash
claudenator
```

Verify by hand (this is the one thing no automated test covers, per the spec's Testing section): the list shows your real sessions with correct directories; `j`/`k`/`gg`/`G` navigate; `s` cycles sort; `/` or `f` filters live; `h`/`l` move focus and the preview scrolls; `d`→`y` deletes a throwaway/test session and `u` brings it back; `Enter` on a real session actually resumes Claude Code in that directory (confirm you land in the right folder and the conversation continues).

- [ ] **Step 7: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: wire main.go entry point (provider setup, TUI run, resume exec)"
```

---

## Final step: push

Once all 9 tasks are committed and the manual smoke test in Task 9 Step 6 passes, push to the remote the user already created:

```bash
git push -u origin master
```

(Ask the user to confirm before running this — pushing is the first action visible outside the local machine.)
