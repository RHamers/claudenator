# Fork Session Name Design

## Intent

`F` currently forks a session's history into a new Claude Code session with
no name (`claude --resume <id> --fork-session`). We're making a name
mandatory: pressing `F` opens a small bordered popup asking for it, and the
fork can't launch until something is typed.

## Goals

- `F` opens a popup box (rounded border, title "Fork session", a text
  input, an inline error when submitted empty) instead of forking
  immediately.
- Enter with a non-empty (trimmed) name launches
  `claude --resume <id> --fork-session --name <name>`, matching how the
  app already reads names back (`custom-title` lines in the session's
  jsonl, surfaced as `session.Session.Name`).
- Esc cancels the popup; no process is launched and no state leaks into
  the next time `F` is pressed.
- Built on branch `feat/fork-session` (already contains everything
  `feat/session-name-column` has, plus the fork key).

## Non-goals

- No rename-after-the-fact UI.
- No name validation beyond "non-empty after trimming whitespace" (no
  length limit, no character restrictions).
- No change to plain `enter`-resume behavior.

## Anti-goals

- Forking must still never trigger the "already running" conflict dialog
  that plain resume uses, regardless of whether a name is involved.

## Approaches considered

1. *True overlay, compositing the box over the live table.* Closest to a
   "real" modal, but bubbletea/lipgloss don't give cell-level compositing
   for free — it would mean hand-rolling a text-grid merge for a
   one-field form.
2. *Full-screen takeover, same pattern as the existing directory browser
   (`n`/`modeBrowse`).* `View()` returns just the lipgloss-rendered box
   while in this mode; nothing else drawn.

Going with **2**: it matches how `modeBrowse` already works, keeps the
change small, and the visual result is still the popup box (rounded
border, title, input, inline error). `lipgloss` is already pulled in
transitively via bubbles/bubbletea, so this promotes it to a direct
import — no new third-party dependency. The box is rendered top-left
(like `browseView()`), not centered against terminal dimensions, since
the app doesn't currently track window size and adding that is out of
scope for this change.

## Approach decided

- `session.Provider.ResumeArgv` gains a `name string` parameter:
  `ResumeArgv(s session.Session, fork bool, name string) []string`. The
  claude-code provider implementation appends `--name`, `name` only when
  `fork` is true *and* `name` is non-empty.
- `resume.Prepare` gains a matching `name string` parameter, passed
  straight through to `provider.ResumeArgv`. `Prepare` does not enforce
  "non-empty" — that's a UI-level rule, not a mechanical one.
- `ui.Model` gets a new mode, `modeForkName`, parallel to `modeFilter`/
  `modeBrowse`, plus a `forkNameInput textinput.Model`, a `forkTarget
  session.Session` (the session being forked, captured when `F` is
  pressed), and a `forkNameErr string`.
- `beginFork()` (triggered by `F`) no longer calls `resume.Prepare`
  directly. It validates the selection and provider (same guards as
  today), then opens `modeForkName` with a blank, focused input.
- `updateForkNameMode` handles the input: Enter trims the value; empty
  sets `forkNameErr = "name is required"` and stays in the modal;
  non-empty calls `resume.Prepare(target, provider, findRunning, true,
  name)` and quits on success, or shows the error inline (reverting to
  `modeNormal`) on failure (e.g. missing directory). Esc clears the input/
  error and returns to `modeNormal` without side effects.
- `forkNameView()` renders a `lipgloss` rounded-border box containing the
  title, the input line, the error line (if any), and an
  `enter: confirm   esc: cancel` hint — the same shape as the popup
  approved for this change.

## What's left to the builder

Nothing outside the above — this is a small, fully-specified change.
