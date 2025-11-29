package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadHotkeys(t *testing.T) {
	t.Run("returns empty map when file doesn't exist", func(t *testing.T) {
		tempDir := t.TempDir()

		hotkeys := LoadHotkeys(tempDir)

		assert.NotNil(t, hotkeys)
		assert.Len(t, hotkeys, 0)
	})

	t.Run("loads valid hotkeys file", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, ".claude-squad")
		err := os.MkdirAll(configDir, 0755)
		require.NoError(t, err)

		hotkeysContent := `{
			"1": "/0-fix-issue",
			"2": "/commit",
			"3": "/review-pr"
		}`
		err = os.WriteFile(filepath.Join(configDir, HotkeysFileName), []byte(hotkeysContent), 0644)
		require.NoError(t, err)

		hotkeys := LoadHotkeys(tempDir)

		assert.NotNil(t, hotkeys)
		assert.Len(t, hotkeys, 3)
		assert.Equal(t, "/0-fix-issue", hotkeys["1"])
		assert.Equal(t, "/commit", hotkeys["2"])
		assert.Equal(t, "/review-pr", hotkeys["3"])
	})

	t.Run("returns empty map on invalid JSON", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, ".claude-squad")
		err := os.MkdirAll(configDir, 0755)
		require.NoError(t, err)

		invalidContent := `{"invalid": json content}`
		err = os.WriteFile(filepath.Join(configDir, HotkeysFileName), []byte(invalidContent), 0644)
		require.NoError(t, err)

		hotkeys := LoadHotkeys(tempDir)

		assert.NotNil(t, hotkeys)
		assert.Len(t, hotkeys, 0)
	})

	t.Run("handles empty hotkeys file", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, ".claude-squad")
		err := os.MkdirAll(configDir, 0755)
		require.NoError(t, err)

		emptyContent := `{}`
		err = os.WriteFile(filepath.Join(configDir, HotkeysFileName), []byte(emptyContent), 0644)
		require.NoError(t, err)

		hotkeys := LoadHotkeys(tempDir)

		assert.NotNil(t, hotkeys)
		assert.Len(t, hotkeys, 0)
	})

	t.Run("handles all number keys 1-9", func(t *testing.T) {
		tempDir := t.TempDir()
		configDir := filepath.Join(tempDir, ".claude-squad")
		err := os.MkdirAll(configDir, 0755)
		require.NoError(t, err)

		hotkeysContent := `{
			"1": "/cmd1",
			"2": "/cmd2",
			"3": "/cmd3",
			"4": "/cmd4",
			"5": "/cmd5",
			"6": "/cmd6",
			"7": "/cmd7",
			"8": "/cmd8",
			"9": "/cmd9"
		}`
		err = os.WriteFile(filepath.Join(configDir, HotkeysFileName), []byte(hotkeysContent), 0644)
		require.NoError(t, err)

		hotkeys := LoadHotkeys(tempDir)

		assert.Len(t, hotkeys, 9)
		for i := 1; i <= 9; i++ {
			key := string(rune('0' + i))
			expected := "/cmd" + key
			assert.Equal(t, expected, hotkeys[key])
		}
	})
}
