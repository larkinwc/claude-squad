package autocomplete

// Suggestion represents an autocomplete suggestion
type Suggestion struct {
	// Value is the full value to insert (e.g., "/0-fix-issue")
	Value string
	// Display is the text shown in the dropdown (e.g., "0-fix-issue")
	Display string
}

// Autocompleter provides autocomplete suggestions
type Autocompleter interface {
	// GetSuggestions returns suggestions matching the given prefix
	GetSuggestions(prefix string) []Suggestion
	// Reload refreshes the available suggestions from disk
	Reload() error
}
