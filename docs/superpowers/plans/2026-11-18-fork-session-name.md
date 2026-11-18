# Fork Session Name Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make forking a session (`F`) require typing a name in a popup before it launches, passing that name to Claude Code via `--name`.

**Architecture:** Thread a `name string` parameter through `session.Provider.ResumeArgv` and `resume.Prepare` down to the forked process's argv. Add a new `modeForkName` UI mode (same shape as `modeBrowse`) that fully replaces the view with a `lipgloss`-bordered popup while active; `F` opens it, Enter validates and submits, Esc cancels.

**Tech Stack:** Go, bubbletea/bubbles (textinput), lipgloss (promoted from indirect to direct dependency).

**Spec:** `docs/superpowers/specs/2026-11-18-fork-session-name-design.md`

## Global Constraints

- Branch: built on `feat/fork-session` (already contains `feat/session-name-column`'s commits plus the existing unnamed fork feature).
- `--name <name>` is appended to the fork argv only when `fork` is true **and** `name` is non-empty; never for plain resume.
- "Name required" is enforced by trimming whitespace and checking for emptiness in the UI layer (`updateForkNameMode`), not in `resume.Prepare`.
- The popup is a full-screen view replacement (like `modeBrowse`), rendered top-left via a `lipgloss.RoundedBorder()` box — no terminal-size tracking or centering.
- Forking must never trigger the existing "already running" conflict confirm, named or not.
- No new third-party dependency: `lipgloss` is already in `go.sum` transitively; this only promotes it to a direct `go.mod` require.

## Review Focus

- Whitespace-only input (`"   "`): must be treated as empty and rejected, not accepted as a literal name.
- Re-opening the popup after a prior cancel/submit: input value and error from the previous attempt must not leak into the next `F` press.
- The session's directory disappearing between pressing `F` and submitting a name: the existing "directory no longer exists" error must still surface inline, without quitting, and without leaving the UI stuck in the popup.
- A session already running (conflict case): submitting a valid name while forking that session must still skip the conflict dialog entirely and launch directly.
- Pressing `F` with an empty session list (nothing selected): must do nothing, not open an empty/broken popup.

---

### Task 1: Thread `name` through `ResumeArgv` and `Prepare`

**Files:**
- Modify: `internal/session/provider.go`
- Modify: `internal/session/claudecode/claudecode.go`
- Modify: `internal/session/claudecode/claudecode_test.go`
- Modify: `internal/resume/resume.go`
- Modify: `internal/resume/resume_test.go`
- Modify: `internal/ui/model.go` (call-site signature fix only — pass `""` for both `beginResume` and the existing `beginFork`; behavior change comes in Task 2)
- Modify: `internal/ui/resume_test.go` (fix `stubProvider.ResumeArgv` signature only)
- Modify: `main_test.go` (fix `fakeAggregateProvider.ResumeArgv` signature only)

**Interfaces:**
- Produces: `session.Provider.ResumeArgv(s Session, fork bool, name string) []string`
- Produces: `resume.Prepare(s session.Session, provider session.Provider, findRunning func(string) (int, bool), fork bool, name string) (*resume.Plan, error)`

- [ ] **Step 1: Write the failing tests in `internal/session/claudecode/claudecode_test.go`**

```go
func TestResumeArgvForkWithNameAppendsNameFlag(t *testing.T) {
	p := New("", trash.Store{})
	got := p.ResumeArgv(session.Session{ID: "abc"}, true, "my-fork")
	want := []string{"claude", "--resume", "abc", "--fork-session", "--name", "my-fork"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResumeArgv(fork=true, name set) = %v, want %v", got, want)
	}
}

func TestResumeArgvForkWithEmptyNameOmitsNameFlag(t *testing.T) {
	p := New("", trash.Store{})
	got := p.ResumeArgv(session.Session{ID: "abc"}, true, "")
	want := []string{"claude", "--resume", "abc", "--fork-session"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResumeArgv(fork=true, name empty) = %v, want %v", got, want)
	}
}

func TestResumeArgvPlainResumeIgnoresName(t *testing.T) {
	p := New("", trash.Store{})
	got := p.ResumeArgv(session.Session{ID: "abc"}, false, "should-be-ignored")
	want := []string{"claude", "--resume", "abc"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResumeArgv(fork=false, name set) = %v, want %v", got, want)
	}
}
```

Also update the two existing calls in this file (`TestResumeArgvPlainResume`, `TestResumeArgvForkAppendsForkSessionFlag`) to pass a third `""` argument so the file compiles against the new signature.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/claudecode/... -run TestResumeArgv -v`
Expected: compile error (`ResumeArgv` still takes 2 args) or FAIL once the signature is updated but the body isn't.

- [ ] **Step 3: Implement `ResumeArgv(s session.Session, fork bool, name string) []string` in `internal/session/claudecode/claudecode.go`**

Append `"--name", name` only when `fork` is true and `name != ""`, after the existing `--fork-session` append.

- [ ] **Step 4: Update `session.Provider` interface in `internal/session/provider.go`**

Change the `ResumeArgv` method signature to `ResumeArgv(s Session, fork bool, name string) []string` and extend its doc comment to mention `name`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/session/claudecode/... -v`
Expected: PASS

- [ ] **Step 6: Write the failing tests in `internal/resume/resume_test.go`**

```go
func TestPrepareForkPassesNameToProvider(t *testing.T) {
	s := session.Session{ID: "abc", Dir: t.TempDir()}
	findRunning := func(string) (int, bool) { return 0, false }

	plan, err := Prepare(s, fakeProvider{}, findRunning, true, "my-fork")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	want := []string{"claude", "--resume", "abc", "--fork-session", "--name", "my-fork"}
	if !reflect.DeepEqual(plan.Argv, want) {
		t.Fatalf("Argv = %v, want %v", plan.Argv, want)
	}
}
```

Update `fakeProvider.ResumeArgv` to the 3-arg signature (mirroring `claudecode.Provider`: append `--fork-session` when `fork`, then `--name`, name when `fork && name != ""`), and add a trailing `""` or the relevant name argument to every existing `Prepare(...)` call in this file so it compiles.

- [ ] **Step 7: Run tests to verify they fail, then pass**

Run: `go test ./internal/resume/... -v`
Expected: first a compile/FAIL, then implement Step 8 and re-run for PASS.

- [ ] **Step 8: Implement the `name` parameter in `resume.Prepare` in `internal/resume/resume.go`**

Add `name string` to the signature and pass it straight through as the third argument to `provider.ResumeArgv(s, fork, name)`. No other logic changes.

- [ ] **Step 9: Fix remaining call sites so the whole repo compiles**

- `internal/ui/model.go`: `beginResume` calls `resume.Prepare(sel, provider, m.findRunning, false, "")`; the existing `beginFork` calls `resume.Prepare(sel, provider, m.findRunning, true, "")` (still a placeholder — Task 2 replaces this body).
- `internal/ui/resume_test.go`: `stubProvider.ResumeArgv(s session.Session, fork bool, name string) []string`. The current body ignores `fork` entirely (`return []string{"claude", "--resume", s.ID}` unconditionally) — Task 2's `TestForkNameValidSubmitSetsPendingResumeWithNameFlag` needs the resulting plan's argv to actually contain `--name <name>`, so give it the same fork/name-aware body as `claudecode.Provider.ResumeArgv` and `fakeProvider.ResumeArgv` (Step 6): append `--fork-session` when `fork`, then `--name`, `name` when `fork && name != ""`.
- `main_test.go`: `fakeAggregateProvider.ResumeArgv(s session.Session, fork bool, name string) []string` (body unchanged).

- [ ] **Step 10: Run the full test suite**

Run: `go test ./...`
Expected: PASS (existing `TestShiftF*` tests in `internal/ui/resume_test.go` still pass unchanged, since `beginFork`'s behavior hasn't changed yet — only its call signature).

- [ ] **Step 11: Commit**

```bash
git add internal/session/provider.go internal/session/claudecode/claudecode.go internal/session/claudecode/claudecode_test.go internal/resume/resume.go internal/resume/resume_test.go internal/ui/model.go internal/ui/resume_test.go main_test.go
git commit -m "feat: thread an optional --name through fork's resume argv"
```

---

### Task 2: Fork-name popup

**Files:**
- Create: `internal/ui/forkname.go` (mirrors `internal/ui/browse.go`: `beginFork`, `updateForkNameMode`, `forkNameView`)
- Create: `internal/ui/forkname_test.go`
- Modify: `internal/ui/model.go` (add `modeForkName` const, `forkNameInput`/`forkTarget`/`forkNameErr` fields, initialize `forkNameInput` in `New`, dispatch in `Update`/`View`, remove the old `beginFork` body — it moves to `forkname.go`)
- Modify: `internal/ui/resume_test.go` (rewrite the three `TestShiftF*` tests for the new two-step flow)
- Modify: `internal/ui/view_test.go` (add a popup-rendering test)
- Modify: `go.mod` / `go.sum` (promote `lipgloss` to a direct dependency via `go mod tidy` after adding the import)

**Interfaces:**
- Consumes: `resume.Prepare(s, provider, findRunning, fork bool, name string) (*resume.Plan, error)` (Task 1)
- Produces: `ui.Model.beginFork() (tea.Model, tea.Cmd)` (same name/signature as before, new behavior — opens the popup instead of forking directly)
- Produces: `ui.Model.updateForkNameMode(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd)`
- Produces: `ui.Model.forkNameView() string`

- [ ] **Step 1: Add the new mode, fields, and initialization in `internal/ui/model.go`**

Add `modeForkName` to the `mode` const block (after `modeBrowse`). Add to `Model`: `forkNameInput textinput.Model`, `forkTarget session.Session`, `forkNameErr string`. In `New()`, alongside the existing `m.filterInput` setup, add:

```go
m.forkNameInput = textinput.New()
m.forkNameInput.Prompt = "Name: "
```

In `Update()`, add a `case modeForkName: return m.updateForkNameMode(keyMsg)` branch. In `View()`, add `if m.mode == modeForkName { return m.forkNameView() }` alongside the existing `modeBrowse` check. Remove the current `beginFork` method body from `model.go` (it's rewritten in `forkname.go` in Step 4); leave the `"F": return m.beginFork()` case in `updateNormalMode` untouched.

- [ ] **Step 2: Write the failing tests in `internal/ui/forkname_test.go`**

```go
package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func typeString(m Model, s string) Model {
	for _, r := range s {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(Model)
	}
	return m
}

func TestFKeyOpensForkNameModalWithoutForkingYet(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 0, false })

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)

	if m.mode != modeForkName {
		t.Fatalf("expected modeForkName, got %v", m.mode)
	}
	if m.PendingResume() != nil {
		t.Fatalf("expected no pending resume before a name is submitted")
	}
	if cmd != nil {
		t.Fatalf("expected no quit command when opening the modal")
	}
}

func TestForkNameEmptySubmitShowsErrorAndStaysOpen(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 0, false })
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.mode != modeForkName {
		t.Fatalf("expected to stay in modeForkName, got %v", m.mode)
	}
	if m.forkNameErr == "" {
		t.Fatalf("expected a non-empty error")
	}
	if m.PendingResume() != nil || cmd != nil {
		t.Fatalf("expected no pending resume and no quit on empty submit")
	}
}

func TestForkNameWhitespaceOnlySubmitIsTreatedAsEmpty(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 0, false })
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)
	m = typeString(m, "   ")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.forkNameErr == "" {
		t.Fatalf("expected whitespace-only input to be rejected")
	}
	if m.PendingResume() != nil || cmd != nil {
		t.Fatalf("expected no pending resume and no quit")
	}
}

func TestForkNameValidSubmitSetsPendingResumeWithNameFlag(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 0, false })
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)
	m = typeString(m, "my-fork")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	plan := m.PendingResume()
	if plan == nil {
		t.Fatalf("expected a pending resume plan")
	}
	if cmd == nil {
		t.Fatalf("expected tea.Quit command")
	}
	joined := strings.Join(plan.Argv, " ")
	if !strings.Contains(joined, "--name my-fork") {
		t.Fatalf("expected argv to contain --name my-fork, got %v", plan.Argv)
	}
}

func TestForkNameSkipsConflictDialogEvenWhenSessionRunning(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 555, true })
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)
	m = typeString(m, "my-fork")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.confirm != nil {
		t.Fatalf("expected no confirm dialog when forking, got %v", m.confirm)
	}
	if m.PendingResume() == nil || cmd == nil {
		t.Fatalf("expected pending resume and quit")
	}
}

func TestForkNameEscCancelsAndClearsState(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 0, false })
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)
	m = typeString(m, "partial")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after esc, got %v", m.mode)
	}
	if m.PendingResume() != nil || cmd != nil {
		t.Fatalf("expected no pending resume and no quit after cancel")
	}

	// Reopening must not see the previous attempt's leftovers.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)
	if m.forkNameInput.Value() != "" {
		t.Fatalf("expected a blank input on reopen, got %q", m.forkNameInput.Value())
	}
	if m.forkNameErr != "" {
		t.Fatalf("expected no leftover error on reopen, got %q", m.forkNameErr)
	}
}

func TestForkNameMissingDirShowsErrorNotQuit(t *testing.T) {
	missing := "/this/path/does/not/exist/claudenator-test"
	sessions := []session.Session{{ID: "abc", Provider: "stub", Dir: missing}}
	providers := map[string]session.Provider{"stub": stubProvider{}}
	m := New(sessions, providers, trash.Store{}, func(string) (int, bool) { return 0, false }, "/tmp")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)
	m = typeString(m, "my-fork")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	if m.err == "" {
		t.Fatalf("expected inline error for missing directory")
	}
	if m.mode != modeNormal {
		t.Fatalf("expected to fall back to modeNormal, got %v", m.mode)
	}
	if cmd != nil {
		t.Fatalf("expected no quit command when directory is missing")
	}
}

func TestFKeyOnEmptyListDoesNothing(t *testing.T) {
	providers := map[string]session.Provider{}
	findRunning := func(string) (int, bool) { return 0, false }
	m := New(nil, providers, trash.Store{}, findRunning, "/tmp")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected to stay in modeNormal with nothing selected, got %v", m.mode)
	}
	if cmd != nil {
		t.Fatalf("expected no command")
	}
}
```

Add the missing `"github.com/RHamers/claudenator/internal/session"` and `"github.com/RHamers/claudenator/internal/trash"` imports that `TestForkNameMissingDirShowsErrorNotQuit` and `TestFKeyOnEmptyListDoesNothing` need (same imports `resume_test.go` already uses).

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/ui/... -v`
Expected: compile errors (`modeForkName`, `forkNameErr`, `forkNameInput` undefined) until Step 1 and Step 4 are both in place; once those compile, these new tests FAIL against the still-unchanged `beginFork`.

- [ ] **Step 4: Implement `internal/ui/forkname.go`**

```go
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/RHamers/claudenator/internal/resume"
)

func (m Model) beginFork() (tea.Model, tea.Cmd) {
	sel := m.selected()
	if sel.ID == "" {
		return m, nil
	}
	if _, ok := m.providers[sel.Provider]; !ok {
		m.err = fmt.Sprintf("no provider registered for %q", sel.Provider)
		return m, nil
	}
	m.err = ""
	m.forkTarget = sel
	m.forkNameErr = ""
	m.forkNameInput.SetValue("")
	m.forkNameInput.Focus()
	m.mode = modeForkName
	return m, nil
}

func (m Model) updateForkNameMode(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.Type {
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyEsc:
		m.mode = modeNormal
		m.forkNameInput.SetValue("")
		m.forkNameErr = ""
		return m, nil
	case tea.KeyEnter:
		name := strings.TrimSpace(m.forkNameInput.Value())
		if name == "" {
			m.forkNameErr = "name is required"
			return m, nil
		}
		provider := m.providers[m.forkTarget.Provider]
		plan, err := resume.Prepare(m.forkTarget, provider, m.findRunning, true, name)
		if err != nil {
			m.mode = modeNormal
			m.err = err.Error()
			return m, nil
		}
		m.pendingResume = plan
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.forkNameInput, cmd = m.forkNameInput.Update(keyMsg)
	return m, cmd
}

func (m Model) forkNameView() string {
	body := "Fork session\n\n" + m.forkNameInput.View()
	if m.forkNameErr != "" {
		body += "\n" + m.forkNameErr
	}
	body += "\n\nenter: confirm   esc: cancel"
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Render(body)
}
```

- [ ] **Step 5: Run `go mod tidy` and verify `lipgloss` becomes a direct dependency**

Run: `go mod tidy && git diff go.mod`
Expected: `github.com/charmbracelet/lipgloss v1.1.0` moves out of the `// indirect` block into the main `require` block.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/ui/... -v`
Expected: the new tests in `forkname_test.go` PASS; the old `TestShiftF*` tests in `resume_test.go` now FAIL (they assert the old immediate-fork behavior) — fix them in Step 7.

- [ ] **Step 7: Remove the stale tests from `internal/ui/resume_test.go`**

Replace `TestShiftFNoConflictSetsPendingResumeAndQuits`, `TestShiftFSkipsConflictDialogEvenWhenSessionRunning`, and `TestShiftFMissingDirShowsErrorNotQuit` — their new-behavior equivalents now live in `forkname_test.go` (Step 2), so delete these three from `resume_test.go` rather than patching them.

- [ ] **Step 8: Add a view test in `internal/ui/view_test.go`**

```go
func TestViewShowsForkNamePopupTitleAndEmptyNameError(t *testing.T) {
	m := newResumeTestModel(t, func(string) (int, bool) { return 0, false })
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)

	view := m.View()
	if !strings.Contains(view, "Fork session") {
		t.Fatalf("expected View() to contain the popup title, got %q", view)
	}
	if !strings.Contains(view, "name is required") {
		t.Fatalf("expected View() to contain the empty-name error, got %q", view)
	}
}
```

- [ ] **Step 9: Run the full test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add internal/ui/forkname.go internal/ui/forkname_test.go internal/ui/model.go internal/ui/resume_test.go internal/ui/view_test.go go.mod go.sum
git commit -m "feat: require a name in a popup before forking a session"
```
