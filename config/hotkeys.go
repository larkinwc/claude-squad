package config

import (
	"claude-squad/log"
	"encoding/json"
	"os"
	"path/filepath"
)

const HotkeysFileName = "hotkeys.json"

// Hotkeys maps number keys (1-9) to commands
type Hotkeys map[string]string

// LoadHotkeys loads hotkey configuration from .claude-squad/hotkeys.json in the given repo path.
// Returns an empty map if the file doesn't exist or cannot be parsed (not an error).
func LoadHotkeys(repoPath string) Hotkeys {
	configPath := filepath.Join(repoPath, ".claude-squad", HotkeysFileName)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.WarningLog.Printf("failed to read hotkeys file: %v", err)
		}
		return make(Hotkeys)
	}

	var hotkeys Hotkeys
	if err := json.Unmarshal(data, &hotkeys); err != nil {
		log.WarningLog.Printf("failed to parse hotkeys file: %v", err)
		return make(Hotkeys)
	}

	return hotkeys
}
