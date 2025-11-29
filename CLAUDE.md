# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

```bash
# Build the application
go build -o claude-squad

# Run tests
go test ./...

# Run a single test
go test ./session/tmux -run TestSessionName

# Lint
gofmt -w .

# Run the application
./claude-squad
# or after installation:
cs
```

## Architecture Overview

Claude Squad is a TUI application that manages multiple AI coding agents (Claude Code, Aider, Codex, Gemini) in isolated workspaces. Each agent runs in its own tmux session with a dedicated git worktree.

### Core Components

**Entry Point** (`main.go`): Uses Cobra for CLI commands (`reset`, `debug`, `version`). The root command launches the TUI via `app.Run()`.

**TUI Layer** (`app/`): Built with Bubble Tea (bubbletea). `home` struct in `app.go` is the main model managing:
- State machine: `stateDefault`, `stateNew`, `statePrompt`, `stateHelp`, `stateConfirm`
- UI components: `List`, `TabbedWindow` (preview/diff), `Menu`, overlay dialogs
- Instance lifecycle through `session.Storage`

**Session Management** (`session/`):
- `Instance` (`instance.go`): Represents a running agent session. Coordinates tmux session + git worktree. States: `Running`, `Ready`, `Loading`, `Paused`
- `Storage` (`storage.go`): JSON persistence in `~/.claude-squad/`

**Tmux Integration** (`session/tmux/`):
- `TmuxSession` manages tmux sessions with prefix `claudesquad_`
- Uses PTY (`creack/pty`) for terminal emulation and window sizing
- `statusMonitor` tracks output changes via SHA256 hashing
- Auto-accepts trust prompts for claude/aider/gemini on startup

**Git Worktree** (`session/git/`):
- `GitWorktree` creates isolated worktrees in `~/.claude-squad/worktrees/`
- Branch naming: `{username}/{session-name}` (configurable via `branch_prefix`)
- Supports pause/resume by removing worktree but preserving branch

**Configuration** (`config/`):
- Config file: `~/.claude-squad/config.json`
- State file: `~/.claude-squad/state.json`
- Key settings: `default_program`, `auto_yes`, `branch_prefix`

### Key Patterns

- **Dependency injection**: `cmd.Executor` interface wraps `exec.Cmd` for testability
- **Platform-specific code**: `*_unix.go` / `*_windows.go` files for OS-specific behavior
- **Overlay system**: `ui/overlay/` provides modal dialogs (text input, confirmation, help)

### Prerequisites

- tmux must be installed and available in PATH
- gh (GitHub CLI) for push operations
- Must run from within a git repository
