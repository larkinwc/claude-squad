package autocomplete

import (
	"os"
	"path/filepath"
	"strings"
)

// ClaudeCommandsAutocompleter scans .claude/commands/ for available commands
type ClaudeCommandsAutocompleter struct {
	basePath string
	commands []Suggestion
}

// NewClaudeCommandsAutocompleter creates a new autocompleter that scans
// the .claude/commands/ directory in the given base path for .md files.
func NewClaudeCommandsAutocompleter(basePath string) *ClaudeCommandsAutocompleter {
	a := &ClaudeCommandsAutocompleter{
		basePath: basePath,
		commands: make([]Suggestion, 0),
	}
	_ = a.Reload() // Ignore errors, just start with empty commands
	return a
}

// GetSuggestions returns suggestions that match the given prefix (case-insensitive).
func (a *ClaudeCommandsAutocompleter) GetSuggestions(prefix string) []Suggestion {
	if len(prefix) == 0 {
		return a.commands
	}

	lowerPrefix := strings.ToLower(prefix)
	var matches []Suggestion
	for _, cmd := range a.commands {
		if strings.HasPrefix(strings.ToLower(cmd.Value), lowerPrefix) {
			matches = append(matches, cmd)
		}
	}
	return matches
}

// Reload scans the .claude/commands/ directory and refreshes the command list.
func (a *ClaudeCommandsAutocompleter) Reload() error {
	commandsDir := filepath.Join(a.basePath, ".claude", "commands")

	entries, err := os.ReadDir(commandsDir)
	if err != nil {
		// If directory doesn't exist, just clear commands (not an error)
		a.commands = make([]Suggestion, 0)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	a.commands = make([]Suggestion, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Only process .md files
		if !strings.HasSuffix(name, ".md") {
			continue
		}

		// Remove .md extension to get command name
		cmdName := strings.TrimSuffix(name, ".md")
		a.commands = append(a.commands, Suggestion{
			Value:   "/" + cmdName,
			Display: cmdName,
		})
	}

	return nil
}
