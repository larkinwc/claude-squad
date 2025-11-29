package overlay

import (
	"claude-squad/ui/autocomplete"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// AutocompleteInputOverlay extends TextInputOverlay with tab-completion support.
type AutocompleteInputOverlay struct {
	textarea      textarea.Model
	Title         string
	FocusIndex    int // 0 for text input, 1 for enter button
	Submitted     bool
	Canceled      bool
	OnSubmit      func()
	width, height int

	// Autocomplete support
	autocompleter      autocomplete.Autocompleter
	suggestions        []autocomplete.Suggestion
	selectedIndex      int
	showingSuggestions bool
}

// NewAutocompleteInputOverlay creates a new text input overlay with autocomplete support.
func NewAutocompleteInputOverlay(title string, initialValue string, ac autocomplete.Autocompleter) *AutocompleteInputOverlay {
	ti := textarea.New()
	ti.SetValue(initialValue)
	ti.Focus()
	ti.ShowLineNumbers = false
	ti.Prompt = ""
	ti.FocusedStyle.CursorLine = lipgloss.NewStyle()

	// Ensure no character limit
	ti.CharLimit = 0
	// Ensure no maximum height limit
	ti.MaxHeight = 0

	return &AutocompleteInputOverlay{
		textarea:      ti,
		Title:         title,
		FocusIndex:    0,
		Submitted:     false,
		Canceled:      false,
		autocompleter: ac,
		suggestions:   make([]autocomplete.Suggestion, 0),
	}
}

func (a *AutocompleteInputOverlay) SetSize(width, height int) {
	a.textarea.SetHeight(height)
	a.width = width
	a.height = height
}

// HandleKeyPress processes a key press and updates the state accordingly.
// Returns true if the overlay should be closed.
func (a *AutocompleteInputOverlay) HandleKeyPress(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyTab:
		value := a.textarea.Value()

		// If text starts with "/" and we're in the textarea, handle autocomplete
		if a.FocusIndex == 0 && strings.HasPrefix(value, "/") {
			if !a.showingSuggestions {
				// Trigger autocomplete
				a.triggerAutocomplete()
				if len(a.suggestions) > 0 {
					a.applySuggestion()
				}
				return false
			}

			// Cycle through suggestions
			if len(a.suggestions) > 0 {
				a.selectedIndex = (a.selectedIndex + 1) % len(a.suggestions)
				a.applySuggestion()
			}
			return false
		}

		// Normal tab behavior: toggle focus between input and enter button
		a.FocusIndex = (a.FocusIndex + 1) % 2
		if a.FocusIndex == 0 {
			a.textarea.Focus()
		} else {
			a.textarea.Blur()
		}
		a.hideSuggestions()
		return false

	case tea.KeyShiftTab:
		// Toggle focus in reverse
		a.FocusIndex = (a.FocusIndex + 1) % 2
		if a.FocusIndex == 0 {
			a.textarea.Focus()
		} else {
			a.textarea.Blur()
		}
		a.hideSuggestions()
		return false

	case tea.KeyEsc:
		if a.showingSuggestions {
			a.hideSuggestions()
			return false
		}
		a.Canceled = true
		return true

	case tea.KeyEnter:
		if a.FocusIndex == 1 {
			// Enter button is focused, so submit
			a.Submitted = true
			if a.OnSubmit != nil {
				a.OnSubmit()
			}
			return true
		}
		fallthrough // Send enter key to textarea

	default:
		if a.FocusIndex == 0 {
			a.textarea, _ = a.textarea.Update(msg)
			// Hide suggestions when user types (they can re-trigger with Tab)
			a.hideSuggestions()
		}
		return false
	}
}

// triggerAutocomplete loads suggestions based on current input
func (a *AutocompleteInputOverlay) triggerAutocomplete() {
	if a.autocompleter == nil {
		return
	}

	value := a.textarea.Value()
	// Get the prefix up to the first space (command without args)
	prefix := value
	if spaceIdx := strings.Index(value, " "); spaceIdx != -1 {
		prefix = value[:spaceIdx]
	}

	a.suggestions = a.autocompleter.GetSuggestions(prefix)
	a.selectedIndex = 0
	a.showingSuggestions = len(a.suggestions) > 0
}

// applySuggestion applies the currently selected suggestion to the input
func (a *AutocompleteInputOverlay) applySuggestion() {
	if len(a.suggestions) == 0 {
		return
	}

	suggestion := a.suggestions[a.selectedIndex]
	currentValue := a.textarea.Value()

	// Preserve any text after the command (arguments)
	var newValue string
	if spaceIdx := strings.Index(currentValue, " "); spaceIdx != -1 {
		// Keep arguments
		newValue = suggestion.Value + currentValue[spaceIdx:]
	} else {
		// No arguments, add space for convenience
		newValue = suggestion.Value + " "
	}

	a.textarea.SetValue(newValue)
	// Move cursor to end
	a.textarea.CursorEnd()
}

// hideSuggestions hides the autocomplete dropdown
func (a *AutocompleteInputOverlay) hideSuggestions() {
	a.showingSuggestions = false
	a.suggestions = make([]autocomplete.Suggestion, 0)
	a.selectedIndex = 0
}

// GetValue returns the current value of the text input.
func (a *AutocompleteInputOverlay) GetValue() string {
	return a.textarea.Value()
}

// IsSubmitted returns whether the form was submitted.
func (a *AutocompleteInputOverlay) IsSubmitted() bool {
	return a.Submitted
}

// IsCanceled returns whether the form was canceled.
func (a *AutocompleteInputOverlay) IsCanceled() bool {
	return a.Canceled
}

// Render renders the autocomplete input overlay.
func (a *AutocompleteInputOverlay) Render() string {
	// Create styles
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2)

	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("62")).
		Bold(true).
		MarginBottom(1)

	buttonStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("7"))

	focusedButtonStyle := buttonStyle.
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("0"))

	suggestionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("7"))

	selectedSuggestionStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("0"))

	// Set textarea width to fit within the overlay
	a.textarea.SetWidth(a.width - 6) // Account for padding and borders

	// Build the view
	content := titleStyle.Render(a.Title) + "\n"
	content += a.textarea.View() + "\n"

	// Show suggestions dropdown if active
	if a.showingSuggestions && len(a.suggestions) > 0 {
		content += "\n"
		maxShow := 5
		if len(a.suggestions) < maxShow {
			maxShow = len(a.suggestions)
		}
		for i := 0; i < maxShow; i++ {
			line := "  " + a.suggestions[i].Display
			if i == a.selectedIndex {
				line = selectedSuggestionStyle.Render(line)
			} else {
				line = suggestionStyle.Render(line)
			}
			content += line + "\n"
		}
		if len(a.suggestions) > maxShow {
			content += suggestionStyle.Render("  ...") + "\n"
		}
	}

	content += "\n"

	// Render enter button with appropriate style
	enterButton := " Enter "
	if a.FocusIndex == 1 {
		enterButton = focusedButtonStyle.Render(enterButton)
	} else {
		enterButton = buttonStyle.Render(enterButton)
	}
	content += enterButton

	return style.Render(content)
}
