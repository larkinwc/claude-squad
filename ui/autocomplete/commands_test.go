package autocomplete

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeCommandsAutocompleter(t *testing.T) {
	t.Run("returns empty suggestions when directory doesn't exist", func(t *testing.T) {
		tempDir := t.TempDir()

		ac := NewClaudeCommandsAutocompleter(tempDir)

		suggestions := ac.GetSuggestions("/")
		assert.Len(t, suggestions, 0)
	})

	t.Run("scans .claude/commands for .md files", func(t *testing.T) {
		tempDir := t.TempDir()
		commandsDir := filepath.Join(tempDir, ".claude", "commands")
		err := os.MkdirAll(commandsDir, 0755)
		require.NoError(t, err)

		// Create some command files
		err = os.WriteFile(filepath.Join(commandsDir, "0-fix-issue.md"), []byte("# Fix issue"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(commandsDir, "commit.md"), []byte("# Commit"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(commandsDir, "review-pr.md"), []byte("# Review PR"), 0644)
		require.NoError(t, err)

		ac := NewClaudeCommandsAutocompleter(tempDir)

		suggestions := ac.GetSuggestions("")
		assert.Len(t, suggestions, 3)
	})

	t.Run("ignores non-.md files", func(t *testing.T) {
		tempDir := t.TempDir()
		commandsDir := filepath.Join(tempDir, ".claude", "commands")
		err := os.MkdirAll(commandsDir, 0755)
		require.NoError(t, err)

		// Create a command file and a non-command file
		err = os.WriteFile(filepath.Join(commandsDir, "valid.md"), []byte("# Valid"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(commandsDir, "readme.txt"), []byte("readme"), 0644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(commandsDir, "script.sh"), []byte("#!/bin/bash"), 0644)
		require.NoError(t, err)

		ac := NewClaudeCommandsAutocompleter(tempDir)

		suggestions := ac.GetSuggestions("")
		assert.Len(t, suggestions, 1)
		assert.Equal(t, "/valid", suggestions[0].Value)
	})

	t.Run("ignores directories", func(t *testing.T) {
		tempDir := t.TempDir()
		commandsDir := filepath.Join(tempDir, ".claude", "commands")
		err := os.MkdirAll(commandsDir, 0755)
		require.NoError(t, err)

		// Create a command file and a subdirectory
		err = os.WriteFile(filepath.Join(commandsDir, "valid.md"), []byte("# Valid"), 0644)
		require.NoError(t, err)
		err = os.MkdirAll(filepath.Join(commandsDir, "subdir"), 0755)
		require.NoError(t, err)

		ac := NewClaudeCommandsAutocompleter(tempDir)

		suggestions := ac.GetSuggestions("")
		assert.Len(t, suggestions, 1)
	})

	t.Run("filters suggestions by prefix", func(t *testing.T) {
		tempDir := t.TempDir()
		commandsDir := filepath.Join(tempDir, ".claude", "commands")
		err := os.MkdirAll(commandsDir, 0755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(commandsDir, "0-fix-issue.md"), []byte(""), 0644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(commandsDir, "0-fix-lint.md"), []byte(""), 0644)
		require.NoError(t, err)
		err = os.WriteFile(filepath.Join(commandsDir, "commit.md"), []byte(""), 0644)
		require.NoError(t, err)

		ac := NewClaudeCommandsAutocompleter(tempDir)

		// All suggestions
		suggestions := ac.GetSuggestions("")
		assert.Len(t, suggestions, 3)

		// Filter by /0-fix
		suggestions = ac.GetSuggestions("/0-fix")
		assert.Len(t, suggestions, 2)

		// Filter by /commit
		suggestions = ac.GetSuggestions("/commit")
		assert.Len(t, suggestions, 1)
		assert.Equal(t, "/commit", suggestions[0].Value)

		// No matches
		suggestions = ac.GetSuggestions("/nonexistent")
		assert.Len(t, suggestions, 0)
	})

	t.Run("filters case-insensitively", func(t *testing.T) {
		tempDir := t.TempDir()
		commandsDir := filepath.Join(tempDir, ".claude", "commands")
		err := os.MkdirAll(commandsDir, 0755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(commandsDir, "Fix-Issue.md"), []byte(""), 0644)
		require.NoError(t, err)

		ac := NewClaudeCommandsAutocompleter(tempDir)

		// Lowercase search
		suggestions := ac.GetSuggestions("/fix")
		assert.Len(t, suggestions, 1)

		// Uppercase search
		suggestions = ac.GetSuggestions("/FIX")
		assert.Len(t, suggestions, 1)
	})

	t.Run("suggestion has correct Value and Display", func(t *testing.T) {
		tempDir := t.TempDir()
		commandsDir := filepath.Join(tempDir, ".claude", "commands")
		err := os.MkdirAll(commandsDir, 0755)
		require.NoError(t, err)

		err = os.WriteFile(filepath.Join(commandsDir, "my-command.md"), []byte(""), 0644)
		require.NoError(t, err)

		ac := NewClaudeCommandsAutocompleter(tempDir)

		suggestions := ac.GetSuggestions("")
		require.Len(t, suggestions, 1)
		assert.Equal(t, "/my-command", suggestions[0].Value)
		assert.Equal(t, "my-command", suggestions[0].Display)
	})

	t.Run("Reload refreshes commands", func(t *testing.T) {
		tempDir := t.TempDir()
		commandsDir := filepath.Join(tempDir, ".claude", "commands")
		err := os.MkdirAll(commandsDir, 0755)
		require.NoError(t, err)

		// Create initial command
		err = os.WriteFile(filepath.Join(commandsDir, "first.md"), []byte(""), 0644)
		require.NoError(t, err)

		ac := NewClaudeCommandsAutocompleter(tempDir)
		assert.Len(t, ac.GetSuggestions(""), 1)

		// Add another command
		err = os.WriteFile(filepath.Join(commandsDir, "second.md"), []byte(""), 0644)
		require.NoError(t, err)

		// Before reload, still sees 1
		assert.Len(t, ac.GetSuggestions(""), 1)

		// After reload, sees 2
		err = ac.Reload()
		require.NoError(t, err)
		assert.Len(t, ac.GetSuggestions(""), 2)
	})
}
