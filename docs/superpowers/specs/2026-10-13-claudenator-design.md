# claudenator — design spec

Date: 2026-10-13

## Problem

Running `claude` from many different project directories over time makes it
easy to lose track of which Claude Code sessions exist, where they ran, and
what they were doing. Claude Code's own `--resume`/`/resume` only lists
sessions from the current working directory, so there is no single place to
see "all my sessions, across all my projects" and jump back into one.

## Goal

A single self-contained Go binary (`claudenator`) that:

- Lists every Claude Code session on the machine, across all project
  directories, in a terminal UI.
- Shows enough metadata per session to recognize it (directory, git branch,
  last-edited time, last message) without having to open it.
- Lets you fuzzy-filter and sort that list.
- Resumes the selected session, in its original working directory, by
  taking over the current terminal (no extra shell wrapper).
- Lets you soft-delete sessions you no longer want cluttering the list, with
  an in-session undo.

Non-goals (v1): Codex/Gemini CLI support (architected for, not built),
persistent/cross-restart undo, a restore UI beyond the trash folder, CI/
release automation.

## Why not use an existing tool

`ccr` (Rust) does exactly this, but ships no prebuilt binaries — every
GitHub release has an empty assets list — so using it would require
installing the Rust toolchain (cargo), which we specifically want to avoid.
Python alternatives (`ccsessions-tui`, `cc-session-control`) exist and avoid
a compiler, but we want a single dependency-free static binary we can drop
on a machine, which points at Go.

## Session data source

Claude Code stores one file per session at:

```
~/.claude/projects/<escaped-cwd>/<session-uuid>.jsonl
```

(`<escaped-cwd>` is the working directory path with `/` replaced by `-`.)

Each line is a JSON object. Relevant fields (confirmed by inspecting a real
session file):

- `cwd` — absolute working directory for that line/event
- `timestamp` — ISO8601 timestamp of that line
- `gitBranch` — branch checked out at that point
- `sessionId` — the session's UUID (matches the filename)
- message content (for the "last user message" preview)

A session's current `cwd`/`gitBranch` is read from the **last** line that
contains those fields (a session can change directory mid-run, though this
is rare); `timestamp` of the last line is "last edited"; the preview text is
the last line with a user-authored message.

Non-`.jsonl` entries under `~/.claude/projects/**` (e.g. a `memory/`
subdirectory) are ignored by the scanner.

## Architecture

```
claudenator/
  main.go                   — entry point, CLI flags, wires providers into the TUI
  internal/
    session/
      session.go             — Session struct: ID, Dir, Branch, LastActive,
                                LastMessage, Provider (string), Running (bool)
      provider.go             — Provider interface:
                                   List() ([]Session, error)
                                   ResumeArgv(s Session) []string   // e.g. ["claude", "--resume", id]
                                   Delete(s Session) error
      claudecode/
        claudecode.go          — Provider impl: walks ~/.claude/projects/*/*.jsonl,
                                  parses last relevant line per file
    trash/
      trash.go                 — soft-delete: move file into
                                  ~/.claudenator/trash/<session-id>.jsonl plus a
                                  sidecar <session-id>.meta.json (original path,
                                  deleted-at); prune entries older than 30 days
                                  on startup
    procscan/
      procscan.go               — reads /proc/*/cmdline on Linux to detect a
                                  running `claude --resume <id>` process; used
                                  for the running-indicator and the double-attach
                                  guard
    ui/
      model.go                  — Bubble Tea model: table, filter input, preview
                                  viewport, sort state, undo stack
      keys.go                    — keybindings
go.mod
```

`session.Provider` is the extension point for future Codex/Gemini support:
each is a separate package implementing the same interface; `main.go`
aggregates `[]Provider` into one merged, sorted list. Nothing in `ui/` or
`trash/` is Claude-Code-specific.

## TUI design

Single-screen layout: a sortable table (Dir / Branch / Last edited, with a
running-session indicator) above a preview pane showing the highlighted
row's last message.

### Modes

- **Normal mode** (default):
  - `j`/`k` or `↓`/`↑` — move selection
  - `gg` / `G` — jump to top/bottom
  - `h` / `l` — move focus between list pane and preview pane; with the
    preview pane focused, `j`/`k` scroll its text instead of moving the
    selection
  - `s` — cycle sort column (Last edited → Dir → Branch → …)
  - `d` — soft-delete highlighted session (inline y/n confirm); blocked
    with an inline message if that session is currently running
  - `u` — undo the most recent delete made this run (pops an in-memory
    stack; moves the file back from trash to its original path)
  - `Enter` — resume (see below)
  - `/` or `f` — enter filter mode
  - `q` / `Ctrl+C` — quit
- **Filter mode** (entered via `/` or `f`):
  - typed characters build a fuzzy query, matched live against
    dir/branch/last message
  - `Enter` — apply filter, return to normal mode (filter stays active)
  - `Esc` — clear filter, return to normal mode

### Resume mechanics

On `Enter`:

1. Scan `/proc/*/cmdline` for a `claude` process whose args include
   `--resume <session-id>`.
2. If found, show an inline confirm ("Session already running in `<dir>`
   (pid `<n>`) — attach anyway? y/n"); `n` returns to the list.
3. `os.Chdir(session.Dir)`, then `syscall.Exec` to replace the claudenator
   process with `claude --resume <session-id>`, inheriting the current TTY.
   claudenator's process ends; exiting Claude Code returns to the normal
   shell, not back into claudenator.

If `session.Dir` no longer exists on disk, show an inline error instead of
attempting the chdir/exec; the session stays listed (a missing directory is
itself useful information, not a reason to hide the row).

### Soft-delete / trash

- `d` → confirm → move `<session-id>.jsonl` from
  `~/.claude/projects/<escaped-cwd>/` to
  `~/.claudenator/trash/<session-id>.jsonl`, with a sidecar
  `<session-id>.meta.json` recording the original path and deletion time.
- Startup prunes trash entries older than 30 days, silently.
- Deleting a currently-running session is blocked (same process-scan as the
  resume guard) rather than allowed.
- `u` pops claudenator's in-memory undo stack (this run only) and moves the
  file back; no restore UI and no persistence across restarts in v1 — the
  trash folder itself is the fallback if you need something back from a
  previous run, before the 30-day prune.

## Testing

Go's standard `testing` package, table-driven:

- **Parser** (`session/claudecode`): fixture `.jsonl` files under
  `testdata/` covering happy path, missing `cwd`, truncated/corrupt last
  line, empty file — asserts the right `Session` or a clean error, never a
  crash.
- **Trash/undo** (`internal/trash`): exercised against a temp directory
  standing in for `$HOME`, not the real `~/.claude` — delete → undo →
  delete again must not lose track of the trash path.
- **TUI state transitions** (`internal/ui`): Bubble Tea models are plain
  structs with `Update(msg) (Model, Cmd)`; tests feed `tea.KeyMsg` sequences
  (e.g. `d`, `y`, `u`) and assert resulting model state — no rendering to a
  real terminal, no snapshot/golden-output testing (brittle, low value here).
- Resuming into a real `claude` process is not covered by automated tests —
  verified manually once built.

## Build & distribution

- `go build ./...`, no CGO — static binary.
- `go install` for local PATH installation, per user preference.
- No CI/release pipeline in v1 (personal tool; revisit if ever shared).

## Open questions / future extensions (explicitly deferred)

- Codex/Gemini CLI providers (interface supports it, not implemented).
- Persistent undo/restore UI across restarts.
- Any packaging beyond `go install` (Homebrew tap, release binaries, etc.).
