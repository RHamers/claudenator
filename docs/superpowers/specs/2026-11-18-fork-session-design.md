# Fork Session Design

## Intent

Today the TUI only lets you resume a session with `enter`, which runs
`claude --resume <id>` and attaches to (or re-execs into) that same
session ID. The user also wants to *fork* a selected session: start a new
Claude Code process that branches off the session's history under a new
session ID, leaving the original session untouched. This maps directly to
Claude Code's own `--fork-session` flag, used together with `--resume`.

## Goals

- Add a second keybinding that forks the selected session instead of
  resuming it in place.
- Reuse the existing resume plumbing (`resume.Prepare`, `Plan`, the
  `PendingResume`/exec handoff in `main.go`) rather than duplicating it.

## Non-goals

- No change to the existing `enter` behavior.
- No UI for picking a custom name/target for the fork — it's a single key
  that forks with Claude Code's default behavior.

## Anti-goals

- Don't make fork path trip the "already running" conflict dialog that
  plain resume uses — a fork always gets a new session ID from Claude
  Code, so it can never collide with a process already attached to the
  original ID.

## Approach decided

- `session.Provider.ResumeArgv` gains a `fork bool` parameter:
  `ResumeArgv(s session.Session, fork bool) []string`. This mirrors the
  CLI's own design (fork is a modifier on resume, not a separate verb).
  The claude-code provider implementation appends `--fork-session` when
  `fork` is true.
- `resume.Prepare` gains a `fork bool` parameter. When `fork` is true, it
  skips the `findRunning` conflict check entirely (no `ConflictPID` is
  ever set) and builds `Plan.Argv` via `provider.ResumeArgv(s, true)`.
- `ui.Model` gets a new key handler bound to `F` (shift+f) that calls a
  `beginFork()` method, parallel to `beginResume()`/`beginResume`'s `enter`
  handler, but calling `resume.Prepare(sel, provider, m.findRunning, true)`
  and going straight to `tea.Quit` (no conflict confirm state, since fork
  can't conflict).
- Footer help text gains `F: fork` next to `enter: resume`.

Keybinding alternative considered: `ctrl+Return` / `shift+Return`. Ruled
out — bubbletea 1.3.10 has no `KeyShiftEnter`/`KeyCtrlEnter` (unlike its
shift/ctrl variants for arrows, Tab, Home/End), because plain terminal
input delivers `Enter` as the same byte regardless of Ctrl/Shift unless
the terminal and app both opt into the Kitty keyboard protocol or
xterm's `modifyOtherKeys`, which bubbletea doesn't implement. A plain
letter key works identically on every terminal.

## What's left to the builder

Nothing outside the above — this is a small, fully-specified change.
