# Fork Session Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an `F` keybinding that forks the selected session (`claude --resume <id> --fork-session`) alongside the existing `enter` keybinding that resumes it in place.

**Architecture:** Thread a `fork bool` parameter down the existing resume path — `session.Provider.ResumeArgv` → `resume.Prepare` → `ui.Model` — instead of adding a parallel set of fork-specific types. `F` reuses `resume.Prepare` and the same `PendingResume`/exec handoff in `main.go`, skipping the running-process conflict check since a fork always gets a new session ID.

**Tech Stack:** Go, bubbletea/bubbles TUI, existing `internal/session`, `internal/resume`, `internal/ui` packages.

**Spec:** `docs/superpowers/specs/2026-11-18-fork-session-design.md`

## Global Constraints

- `session.Provider.ResumeArgv` signature becomes `ResumeArgv(s session.Session, fork bool) []string` — every implementation and fake in the repo must be updated to match.
- The claude-code provider appends exactly `--fork-session` (one argv element) when `fork` is true, after the existing `{"claude", "--resume", s.ID}`.
- Forking must never set `Plan.ConflictPID` and must never route through the `confirmResumeConflict` dialog.
- `enter`'s existing behavior (including the conflict dialog) must be unchanged.

## Review Focus

- Forking a session whose directory no longer exists must surface the same inline error `beginResume` already produces for this case (`resume.Prepare` returns an error either way) — covered by Task 2/3 tests mirroring `TestPrepareMissingDirReturnsError` / `TestEnterMissingDirShowsErrorNotQuit`.
- Forking a session that *is* currently running (an "enter" conflict case) must skip the confirm dialog and go straight to `PendingResume` + quit — covered by Task 3's conflict test.
- The footer hint text must mention the new key so it's discoverable — covered by Task 3's footer test.
- No provider is registered for the selected session (same failure path `beginResume` already handles) — already generically covered by `beginResume`'s existing `m.err` branch; Task 3 adds the equivalent fork test.
- Argv ordering/content for the fork case must be exact (`--fork-session` present and last) — covered by Task 1's test.

---

## Task 1: `ResumeArgv` gains a `fork` parameter

**Files:**
- Modify: `internal/session/provider.go`
- Modify: `internal/session/claudecode/claudecode.go:158-160`
- Modify: `internal/session/claudecode/claudecode_test.go` (add test)
- Modify: `main_test.go:15-17` (fake signature)
- Modify: `internal/resume/resume_test.go:12-14` (fake signature)
- Modify: `internal/ui/resume_test.go:16-18` (fake signature)

**Interfaces:**
- Produces: `session.Provider.ResumeArgv(s session.Session, fork bool) []string` — the signature every later task's fakes and callers use.

- [ ] **Step 1: Write the failing test** in `internal/session/claudecode/claudecode_test.go`:

```go
func TestResumeArgvPlainResume(t *testing.T) {
	p := New("", trash.Store{})
	got := p.ResumeArgv(session.Session{ID: "abc"}, false)
	want := []string{"claude", "--resume", "abc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResumeArgv(fork=false) = %v, want %v", got, want)
	}
}

func TestResumeArgvForkAppendsForkSessionFlag(t *testing.T) {
	p := New("", trash.Store{})
	got := p.ResumeArgv(session.Session{ID: "abc"}, true)
	want := []string{"claude", "--resume", "abc", "--fork-session"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResumeArgv(fork=true) = %v, want %v", got, want)
	}
}
```

(Add `"reflect"` and `"github.com/RHamers/claudenator/internal/session"` to the file's imports if not already present.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/session/claudecode/... -run TestResumeArgv -v`
Expected: FAIL (compile error — `ResumeArgv` takes 1 arg, called with 2)

- [ ] **Step 3: Update `session.Provider` and the claude-code implementation**

In `internal/session/provider.go`, change the interface method to:
```go
// ResumeArgv returns the argv that resumes this session as a new process.
// If fork is true, the returned argv starts a new session ID instead of
// reusing the original.
ResumeArgv(s Session, fork bool) []string
```

In `internal/session/claudecode/claudecode.go:158-160`, change to:
```go
func (p *Provider) ResumeArgv(s session.Session, fork bool) []string {
	argv := []string{"claude", "--resume", s.ID}
	if fork {
		argv = append(argv, "--fork-session")
	}
	return argv
}
```

- [ ] **Step 4: Update every fake `ResumeArgv` to the new signature**

In `main_test.go:15-17`, `internal/resume/resume_test.go:12-14`, and `internal/ui/resume_test.go:16-18`, change each fake's method to:
```go
func (p fakeAggregateProvider) ResumeArgv(s session.Session, fork bool) []string {
	return []string{"claude", "--resume", s.ID}
}
```
(adjust the receiver type name per file: `fakeProvider`, `stubProvider`). The `fork` parameter is unused in these fakes — that's fine, it's a stand-in for providers that only care about the plain-resume case; Task 1's own tests exercise the real fork behavior.

- [ ] **Step 5: Run test to verify it passes**

Run: `go build ./... && go test ./... -v`
Expected: PASS (full build succeeds and all packages' tests pass, confirming every call site compiles against the new signature)

- [ ] **Step 6: Commit**

```bash
git add internal/session/provider.go internal/session/claudecode/claudecode.go internal/session/claudecode/claudecode_test.go main_test.go internal/resume/resume_test.go internal/ui/resume_test.go
git commit -m "feat: add fork parameter to Provider.ResumeArgv"
```

---

## Task 2: `resume.Prepare` gains a `fork` parameter and skips the conflict check

**Files:**
- Modify: `internal/resume/resume.go`
- Modify: `internal/resume/resume_test.go` (add tests, update existing call sites)

**Interfaces:**
- Consumes: `session.Provider.ResumeArgv(s session.Session, fork bool) []string` (Task 1).
- Produces: `resume.Prepare(s session.Session, provider session.Provider, findRunning func(string) (int, bool), fork bool) (*Plan, error)` — the signature Task 3's `beginFork`/`beginResume` call.

- [ ] **Step 1: Write the failing tests** in `internal/resume/resume_test.go`:

```go
func TestPrepareForkSkipsConflictCheck(t *testing.T) {
	s := session.Session{ID: "abc", Dir: t.TempDir()}
	findRunning := func(id string) (int, bool) {
		if id == "abc" {
			return 999, true // would be a conflict for plain resume
		}
		return 0, false
	}

	plan, err := Prepare(s, fakeProvider{}, findRunning, true)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if plan.ConflictPID != 0 {
		t.Errorf("ConflictPID = %d, want 0 (fork should never conflict)", plan.ConflictPID)
	}
}

func TestPrepareForkBuildsForkArgv(t *testing.T) {
	s := session.Session{ID: "abc", Dir: t.TempDir()}
	findRunning := func(string) (int, bool) { return 0, false }

	plan, err := Prepare(s, fakeProvider{}, findRunning, true)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	want := []string{"claude", "--resume", "abc", "--fork-session"}
	if !reflect.DeepEqual(plan.Argv, want) {
		t.Fatalf("Argv = %v, want %v", plan.Argv, want)
	}
}
```

Update `fakeProvider.ResumeArgv` in this file (if not already done as part of Task 1 Step 4) to actually honor `fork`, matching the real provider:
```go
func (fakeProvider) ResumeArgv(s session.Session, fork bool) []string {
	argv := []string{"claude", "--resume", s.ID}
	if fork {
		argv = append(argv, "--fork-session")
	}
	return argv
}
```

Update the three existing calls to `Prepare(...)` in `TestPrepareNoConflict`, `TestPrepareConflict`, and `TestPrepareMissingDirReturnsError` to pass `false` as the new last argument.

(Add `"reflect"` to imports if not already present.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/resume/... -v`
Expected: FAIL (compile error — `Prepare` called with wrong arg count)

- [ ] **Step 3: Implement the `fork` parameter in `Prepare`**

In `internal/resume/resume.go`:
```go
func Prepare(s session.Session, provider session.Provider, findRunning func(sessionID string) (pid int, found bool), fork bool) (*Plan, error) {
	if _, err := os.Stat(s.Dir); err != nil {
		return nil, fmt.Errorf("claudenator: directory no longer exists: %s", s.Dir)
	}
	plan := &Plan{
		Argv: provider.ResumeArgv(s, fork),
		Dir:  s.Dir,
	}
	if fork {
		return plan, nil
	}
	if pid, found := findRunning(s.ID); found {
		plan.ConflictPID = pid
	}
	return plan, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/resume/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/resume/resume.go internal/resume/resume_test.go
git commit -m "feat: add fork parameter to resume.Prepare, skip conflict check when forking"
```

---

## Task 3: Wire `F` to fork in the TUI

**Files:**
- Modify: `internal/ui/model.go`
- Modify: `internal/ui/format.go:34` (footer text)
- Modify: `internal/ui/resume_test.go` (add tests)
- Modify: `internal/ui/view_test.go` (footer test)

**Interfaces:**
- Consumes: `resume.Prepare(s session.Session, provider session.Provider, findRunning func(string) (int, bool), fork bool) (*Plan, error)` (Task 2).
- Produces: `Model.beginFork() (tea.Model, tea.Cmd)` — no other task depends on this name, but keep it parallel to `beginResume` for readability.

- [ ] **Step 1: Write the failing tests** in `internal/ui/resume_test.go`:

```go
func TestShiftFNoConflictSetsPendingResumeAndQuits(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 0, false })

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)

	if m.PendingResume() == nil {
		t.Fatalf("expected pending resume plan")
	}
	if cmd == nil {
		t.Fatalf("expected tea.Quit command")
	}
}

func TestShiftFSkipsConflictDialogEvenWhenSessionRunning(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 555, true })

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)

	if m.confirm != nil {
		t.Fatalf("expected no confirm dialog when forking, got %v", m.confirm)
	}
	if m.PendingResume() == nil {
		t.Fatalf("expected pending resume plan")
	}
	if cmd == nil {
		t.Fatalf("expected tea.Quit command")
	}
}

func TestShiftFMissingDirShowsErrorNotQuit(t *testing.T) {
	missing := "/this/path/does/not/exist/claudenator-test"
	os.RemoveAll(missing)
	sessions := []session.Session{{ID: "abc", Provider: "stub", Dir: missing}}
	providers := map[string]session.Provider{"stub": stubProvider{}}
	m := New(sessions, providers, trash.Store{}, func(string) (int, bool) { return 0, false }, "/tmp")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)

	if m.err == "" {
		t.Fatalf("expected inline error for missing directory")
	}
	if cmd != nil {
		t.Fatalf("expected no quit command when directory is missing")
	}
}
```

- [ ] **Step 2: Write the failing footer test** in `internal/ui/view_test.go`, extending `TestViewShowsKeybindFooterInNormalMode`:

```go
	if !strings.Contains(view, "F: fork") {
		t.Fatalf("expected View() to contain fork keybind hint, got %q", view)
	}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/ui/... -v`
Expected: FAIL (`F` currently falls through to the default case and does nothing; footer text doesn't mention fork)

- [ ] **Step 4: Implement `beginFork` and wire the `F` key in `internal/ui/model.go`**

Add `"F"` as a new case in `updateNormalMode`'s switch (next to `case "enter":`):
```go
	case "F":
		return m.beginFork()
```

Add `beginFork`, parallel to `beginResume` (`internal/ui/model.go:226-248`) but passing `fork: true` to `resume.Prepare` and never entering the confirm state:
```go
func (m Model) beginFork() (tea.Model, tea.Cmd) {
	sel := m.selected()
	if sel.ID == "" {
		return m, nil
	}
	provider, ok := m.providers[sel.Provider]
	if !ok {
		m.err = fmt.Sprintf("no provider registered for %q", sel.Provider)
		return m, nil
	}
	plan, err := resume.Prepare(sel, provider, m.findRunning, true)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.err = ""
	m.pendingResume = plan
	return m, tea.Quit
}
```

Update the existing `beginResume` call to `resume.Prepare` (`internal/ui/model.go:236`) to pass `false` as the new last argument.

- [ ] **Step 5: Update the footer text in `internal/ui/format.go:34`**

Change the default-mode footer string to insert fork next to resume:
```go
return "j/k: move   gg/G: top/bottom   h/l: focus list/preview   s: sort   /: filter   enter: resume   F: fork   n: new session   d: delete   u: undo   q: quit"
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./... -v`
Expected: PASS (all packages, including the new fork tests and footer test)

- [ ] **Step 7: Commit**

```bash
git add internal/ui/model.go internal/ui/format.go internal/ui/resume_test.go internal/ui/view_test.go
git commit -m "feat: bind F to fork the selected session"
```
