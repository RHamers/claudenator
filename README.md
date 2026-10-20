# claudenator

A terminal UI for managing [Claude Code](https://claude.com/claude-code)
sessions across every project directory on your machine — list them, resume
one, start a fresh one in a directory you pick, and clean up the ones you
don't need anymore.

Claude Code's own `--resume`/`/resume` only sees sessions from the current
working directory. claudenator scans `~/.claude/projects/**` so you get one
list across all of them.

## Install

**Prebuilt binary** — grab the latest Linux (amd64) or macOS (arm64) build
from the [Releases page](../../releases/latest), make it executable, and put
it on your `PATH`:

```sh
chmod +x claudenator-<platform>
mv claudenator-<platform> ~/go/bin/claudenator   # or anywhere on your PATH
```

**With Go installed:**

```sh
go install github.com/RHamers/claudenator@latest
```

Make sure `go env GOBIN` (or `$(go env GOPATH)/bin` if `GOBIN` is unset) is on
your `PATH`.

**From a clone:**

```sh
./install.sh
```

## Usage

Run `claudenator` from anywhere; it lists every Claude Code session found on
the machine, most recently active first.

| Key | Action |
|---|---|
| `j` / `k` / `↓` / `↑` | Move selection |
| `gg` / `G` | Jump to top / bottom |
| `h` / `l` | Move focus between the list and the preview pane |
| `s` | Cycle sort column |
| `/` or `f` | Filter (fuzzy match on dir/branch/last message) |
| `enter` | Resume the selected session, in its original directory |
| `n` | Start a new session — browse down from the directory claudenator was launched in, `enter` to launch there |
| `d` | Soft-delete the selected session (`y`/`n` to confirm) |
| `u` | Undo the last delete |
| `q` / `ctrl+c` | Quit |

Run `claudenator --version` (or `-v`) to print the build's commit SHA
without launching the TUI — release binaries are stamped with the commit
they were built from.

Resuming (or starting a new session) replaces the claudenator process with
`claude`, inheriting the current terminal — quitting Claude Code drops you
back into your shell, not back into claudenator.

### New-session browser (`n`)

- `j`/`k` move the selection, `l` descends into the highlighted subdirectory,
  `h` goes back up a level — never above the directory claudenator was
  started from.
- `enter` launches a new `claude` session in whatever directory is currently
  being viewed.
- `esc` cancels back to the session list.

### Soft delete

`d` moves a session's file to `~/.claudenator/trash/` instead of deleting it
outright; entries older than 30 days are pruned on startup. `u` restores the
most recently deleted session (this run only).

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

See `docs/superpowers/specs/2026-10-13-claudenator-design.md` for the full
design rationale.
