package app

import (
	"claude-squad/config"
	"claude-squad/keys"
	"claude-squad/log"
	"claude-squad/session"
	"claude-squad/ui"
	"claude-squad/ui/autocomplete"
	"claude-squad/ui/overlay"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const GlobalInstanceLimit = 10

// Run is the main entrypoint into the application.
func Run(ctx context.Context, program string, autoYes bool) error {
	p := tea.NewProgram(
		newHome(ctx, program, autoYes),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(), // Mouse scroll
	)
	_, err := p.Run()
	return err
}

type state int

const (
	stateDefault state = iota
	// stateNew is the state when the user is creating a new instance.
	stateNew
	// statePrompt is the state when the user is entering a prompt.
	statePrompt
	// stateHelp is the state when a help screen is displayed.
	stateHelp
	// stateConfirm is the state when a confirmation modal is displayed.
	stateConfirm
)

type home struct {
	ctx context.Context

	// -- Storage and Configuration --

	program string
	autoYes bool

	// storage is the interface for saving/loading data to/from the app's state
	storage *session.Storage
	// appConfig stores persistent application configuration
	appConfig *config.Config
	// appState stores persistent application state like seen help screens
	appState config.AppState

	// -- State --

	// state is the current discrete state of the application
	state state
	// newInstanceFinalizer is called when the state is stateNew and then you press enter.
	// It registers the new instance in the list after the instance has been started.
	newInstanceFinalizer func()

	// promptAfterName tracks if we should enter prompt mode after naming
	promptAfterName bool
	// pendingPrompt stores a prompt submitted before instance finished initializing
	pendingPrompt string

	// keySent is used to manage underlining menu items
	keySent bool

	// -- UI Components --

	// list displays the list of instances
	list *ui.List
	// menu displays the bottom menu
	menu *ui.Menu
	// tabbedWindow displays the tabbed window with preview and diff panes
	tabbedWindow *ui.TabbedWindow
	// errBox displays error messages
	errBox *ui.ErrBox
	// global spinner instance. we plumb this down to where it's needed
	spinner spinner.Model
	// textInputOverlay handles text input with state
	textInputOverlay *overlay.TextInputOverlay
	// textOverlay displays text information
	textOverlay *overlay.TextOverlay
	// confirmationOverlay displays confirmation modals
	confirmationOverlay *overlay.ConfirmationOverlay

	// hotkeys maps number keys (1-9) to commands for quick send
	hotkeys config.Hotkeys

	// autocompleter provides command autocomplete for prompt input
	autocompleter autocomplete.Autocompleter
	// autocompleteInputOverlay handles text input with autocomplete support
	autocompleteInputOverlay *overlay.AutocompleteInputOverlay

	// initProgressMessage stores the current progress message for initializing instance
	initProgressMessage string

	// pendingKillInstance stores the instance pending deletion after confirmation
	pendingKillInstance *session.Instance
}

func newHome(ctx context.Context, program string, autoYes bool) *home {
	// Load application config
	appConfig := config.LoadConfig()

	// Load application state
	appState := config.LoadState()

	// Initialize storage
	storage, err := session.NewStorage(appState)
	if err != nil {
		fmt.Printf("Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}

	h := &home{
		ctx:          ctx,
		spinner:      spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		menu:         ui.NewMenu(),
		tabbedWindow: ui.NewTabbedWindow(ui.NewPreviewPane(), ui.NewDiffPane()),
		errBox:       ui.NewErrBox(),
		storage:      storage,
		appConfig:    appConfig,
		program:      program,
		autoYes:      autoYes,
		state:        stateDefault,
		appState:     appState,
	}
	h.list = ui.NewList(&h.spinner, autoYes)

	// Load per-repo hotkeys
	h.hotkeys = config.LoadHotkeys(".")

	// Initialize autocompleter for Claude commands
	h.autocompleter = autocomplete.NewClaudeCommandsAutocompleter(".")

	// Load saved instances
	instances, err := storage.LoadInstances()
	if err != nil {
		fmt.Printf("Failed to load instances: %v\n", err)
		os.Exit(1)
	}

	// Add loaded instances to the list
	for _, instance := range instances {
		// Call the finalizer immediately.
		h.list.AddInstance(instance)()
		if autoYes {
			instance.AutoYes = true
		}
	}

	return h
}

// updateHandleWindowSizeEvent sets the sizes of the components.
// The components will try to render inside their bounds.
func (m *home) updateHandleWindowSizeEvent(msg tea.WindowSizeMsg) {
	// List takes 30% of width, preview takes 70%
	listWidth := int(float32(msg.Width) * 0.3)
	tabsWidth := msg.Width - listWidth

	// Menu takes 10% of height, list and window take 90%
	contentHeight := int(float32(msg.Height) * 0.9)
	menuHeight := msg.Height - contentHeight - 1     // minus 1 for error box
	m.errBox.SetSize(int(float32(msg.Width)*0.9), 1) // error box takes 1 row

	m.tabbedWindow.SetSize(tabsWidth, contentHeight)
	m.list.SetSize(listWidth, contentHeight)

	if m.textInputOverlay != nil {
		m.textInputOverlay.SetSize(int(float32(msg.Width)*0.6), int(float32(msg.Height)*0.4))
	}
	if m.autocompleteInputOverlay != nil {
		m.autocompleteInputOverlay.SetSize(int(float32(msg.Width)*0.6), int(float32(msg.Height)*0.4))
	}
	if m.textOverlay != nil {
		m.textOverlay.SetWidth(int(float32(msg.Width) * 0.6))
	}

	previewWidth, previewHeight := m.tabbedWindow.GetPreviewSize()
	if err := m.list.SetSessionPreviewSize(previewWidth, previewHeight); err != nil {
		log.ErrorLog.Print(err)
	}
	m.menu.SetSize(msg.Width, menuHeight)
}

func (m *home) Init() tea.Cmd {
	// Upon starting, we want to start the spinner. Whenever we get a spinner.TickMsg, we
	// update the spinner, which sends a new spinner.TickMsg. I think this lasts forever lol.
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			time.Sleep(100 * time.Millisecond)
			return previewTickMsg{}
		},
		tickUpdateMetadataCmd,
	)
}

func (m *home) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case hideErrMsg:
		m.errBox.Clear()
	case previewTickMsg:
		cmd := m.instanceChanged()
		return m, tea.Batch(
			cmd,
			func() tea.Msg {
				time.Sleep(100 * time.Millisecond)
				return previewTickMsg{}
			},
		)
	case keyupMsg:
		m.menu.ClearKeydown()
		return m, nil
	case tickUpdateMetadataMessage:
		for _, instance := range m.list.GetInstances() {
			if !instance.Started() || instance.Paused() {
				continue
			}
			updated, prompt := instance.HasUpdated()
			if updated {
				instance.SetStatus(session.Running)
			} else {
				if prompt {
					instance.TapEnter()
				} else {
					instance.SetStatus(session.Ready)
				}
			}
			if err := instance.UpdateDiffStats(); err != nil {
				log.WarningLog.Printf("could not update diff stats: %v", err)
			}
		}
		return m, tickUpdateMetadataCmd
	case tea.MouseMsg:
		// Handle mouse wheel events for scrolling the diff/preview pane
		if msg.Action == tea.MouseActionPress {
			if msg.Button == tea.MouseButtonWheelDown || msg.Button == tea.MouseButtonWheelUp {
				selected := m.list.GetSelectedInstance()
				if selected == nil || selected.Status == session.Paused {
					return m, nil
				}

				switch msg.Button {
				case tea.MouseButtonWheelUp:
					m.tabbedWindow.ScrollUp()
				case tea.MouseButtonWheelDown:
					m.tabbedWindow.ScrollDown()
				}
			}
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKeyPress(msg)
	case tea.WindowSizeMsg:
		m.updateHandleWindowSizeEvent(msg)
		return m, nil
	case error:
		// Handle errors from confirmation actions
		return m, m.handleError(msg)
	case instanceChangedMsg:
		// Handle instance changed after confirmation action
		return m, m.instanceChanged()
	case instanceDeletedMsg:
		// Handle instance deletion completion
		if msg.err != nil {
			// Deletion failed - revert status and show error
			if msg.instance != nil {
				msg.instance.SetStatus(session.Ready)
			}
			return m, m.handleError(msg.err)
		}
		// Successfully deleted - remove from list
		m.list.RemoveInstance(msg.instance)
		return m, m.instanceChanged()
	case instanceProgressMsg:
		// Update progress message and continue listening
		m.initProgressMessage = msg.progress.Message
		return m, listenForProgressCmd(msg.instance, msg.channel, msg.finalizer, msg.promptAfterName)
	case instanceStartCompleteMsg:
		// Clear progress message
		m.initProgressMessage = ""

		if msg.err != nil {
			// Find and remove the failed instance
			for i, inst := range m.list.GetInstances() {
				if inst == msg.instance {
					m.list.SetSelectedInstance(i)
					m.list.Kill()
					break
				}
			}
			// Clear pending prompt on error
			m.pendingPrompt = ""
			// Close prompt overlay if open
			if m.state == statePrompt {
				m.autocompleteInputOverlay = nil
				m.state = stateDefault
				m.menu.SetState(ui.StateDefault)
			}
			return m, m.handleError(msg.err)
		}

		// Save after adding new instance
		if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
			return m, m.handleError(err)
		}

		// Call finalizer if present
		if msg.finalizer != nil {
			msg.finalizer()
		}
		if m.autoYes {
			msg.instance.AutoYes = true
		}

		// Send pending prompt if user submitted while instance was initializing
		if m.pendingPrompt != "" {
			prompt := m.pendingPrompt
			m.pendingPrompt = ""
			// Use async command to wait for input ready before sending
			return m, tea.Batch(
				tea.WindowSize(),
				m.instanceChanged(),
				sendPendingPromptCmd(msg.instance, prompt),
			)
		} else if m.state == statePrompt {
			// Prompt overlay is still open, user is still typing - do nothing
		} else if msg.promptAfterName {
			// Legacy path (shouldn't happen with new flow)
			m.state = statePrompt
			m.menu.SetState(ui.StatePrompt)
			m.autocompleteInputOverlay = overlay.NewAutocompleteInputOverlay("Enter prompt", "", m.autocompleter)
		} else {
			m.showHelpScreen(helpStart(msg.instance), nil)
		}

		return m, tea.Batch(tea.WindowSize(), m.instanceChanged())
	case pendingPromptSentMsg:
		if msg.err != nil {
			return m, m.handleError(msg.err)
		}
		// Show help screen now that prompt has been sent
		m.showHelpScreen(helpStart(msg.instance), nil)
		return m, m.instanceChanged()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *home) handleQuit() (tea.Model, tea.Cmd) {
	if err := m.storage.SaveInstances(m.list.GetInstances()); err != nil {
		return m, m.handleError(err)
	}
	return m, tea.Quit
}

func (m *home) handleMenuHighlighting(msg tea.KeyMsg) (cmd tea.Cmd, returnEarly bool) {
	// Handle menu highlighting when you press a button. We intercept it here and immediately return to
	// update the ui while re-sending the keypress. Then, on the next call to this, we actually handle the keypress.
	if m.keySent {
		m.keySent = false
		return nil, false
	}
	if m.state == statePrompt || m.state == stateHelp || m.state == stateConfirm {
		return nil, false
	}
	// If it's in the global keymap, we should try to highlight it.
	name, ok := keys.GlobalKeyStringsMap[msg.String()]
	if !ok {
		return nil, false
	}

	if m.list.GetSelectedInstance() != nil && m.list.GetSelectedInstance().Paused() && name == keys.KeyEnter {
		return nil, false
	}
	if name == keys.KeyShiftDown || name == keys.KeyShiftUp {
		return nil, false
	}

	// Skip the menu highlighting if the key is not in the map or we are using the shift up and down keys.
	// TODO: cleanup: when you press enter on stateNew, we use keys.KeySubmitName. We should unify the keymap.
	if name == keys.KeyEnter && m.state == stateNew {
		name = keys.KeySubmitName
	}
	m.keySent = true
	return tea.Batch(
		func() tea.Msg { return msg },
		m.keydownCallback(name)), true
}

func (m *home) handleKeyPress(msg tea.KeyMsg) (mod tea.Model, cmd tea.Cmd) {
	cmd, returnEarly := m.handleMenuHighlighting(msg)
	if returnEarly {
		return m, cmd
	}

	if m.state == stateHelp {
		return m.handleHelpState(msg)
	}

	if m.state == stateNew {
		// Handle quit commands first. Don't handle q because the user might want to type that.
		if msg.String() == "ctrl+c" {
			m.state = stateDefault
			m.promptAfterName = false
			m.list.Kill()
			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
			)
		}

		instance := m.list.GetInstances()[m.list.NumInstances()-1]
		switch msg.Type {
		// Start the instance asynchronously and go back to the main menu state.
		case tea.KeyEnter:
			if len(instance.Title) == 0 {
				return m, m.handleError(fmt.Errorf("title cannot be empty"))
			}

			// Set loading state
			instance.SetStatus(session.Loading)

			// Capture state before clearing
			finalizer := m.newInstanceFinalizer
			promptAfterName := m.promptAfterName
			m.promptAfterName = false
			m.pendingPrompt = ""
			m.initProgressMessage = "Starting..."

			// If prompt after name, show overlay immediately while instance initializes
			if promptAfterName {
				m.state = statePrompt
				m.menu.SetState(ui.StatePrompt)
				m.autocompleteInputOverlay = overlay.NewAutocompleteInputOverlay("Enter prompt", "", m.autocompleter)
				// Start async initialization and trigger window resize to size the overlay
				return m, tea.Batch(startInstanceCmd(instance, finalizer, false), tea.WindowSize())
			}

			m.state = stateDefault
			m.menu.SetState(ui.StateDefault)
			// Start async initialization (pass false for promptAfterName since we handle it above)
			return m, startInstanceCmd(instance, finalizer, false)
		case tea.KeyRunes:
			if len(instance.Title) >= 32 {
				return m, m.handleError(fmt.Errorf("title cannot be longer than 32 characters"))
			}
			if err := instance.SetTitle(instance.Title + string(msg.Runes)); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyBackspace:
			if len(instance.Title) == 0 {
				return m, nil
			}
			if err := instance.SetTitle(instance.Title[:len(instance.Title)-1]); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeySpace:
			if err := instance.SetTitle(instance.Title + " "); err != nil {
				return m, m.handleError(err)
			}
		case tea.KeyEsc:
			m.list.Kill()
			m.state = stateDefault
			m.instanceChanged()

			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					return nil
				},
			)
		default:
		}
		return m, nil
	} else if m.state == statePrompt {
		// Use the AutocompleteInputOverlay component to handle all key events
		shouldClose := m.autocompleteInputOverlay.HandleKeyPress(msg)

		// Check if the form was submitted or canceled
		if shouldClose {
			selected := m.list.GetSelectedInstance()
			// TODO: this should never happen since we set the instance in the previous state.
			if selected == nil {
				return m, nil
			}
			if m.autocompleteInputOverlay.IsSubmitted() {
				prompt := m.autocompleteInputOverlay.GetValue()
				// Try to send prompt - if instance not ready yet, store as pending
				if err := selected.SendPrompt(prompt); err != nil {
					// Instance not ready yet, store prompt for later
					m.pendingPrompt = prompt
				}
			}

			// Close the overlay and reset state
			m.autocompleteInputOverlay = nil
			m.state = stateDefault
			return m, tea.Sequence(
				tea.WindowSize(),
				func() tea.Msg {
					m.menu.SetState(ui.StateDefault)
					// Only show help screen if instance is ready (no pending prompt)
					if m.pendingPrompt == "" {
						m.showHelpScreen(helpStart(selected), nil)
					}
					return nil
				},
			)
		}

		return m, nil
	}

	// Handle confirmation state
	if m.state == stateConfirm {
		keyStr := msg.String()
		confirmed := keyStr == "y"
		cancelled := keyStr == "n" || keyStr == "esc"

		if confirmed || cancelled {
			m.state = stateDefault
			overlay := m.confirmationOverlay
			m.confirmationOverlay = nil

			// Handle kill confirmation (async)
			if confirmed && m.pendingKillInstance != nil {
				instance := m.pendingKillInstance
				m.pendingKillInstance = nil

				// Mark as deleting immediately so user sees feedback
				instance.SetStatus(session.Deleting)

				// Start async deletion
				return m, deleteInstanceCmd(instance, m.storage)
			}

			// Clear pending instance on cancel
			m.pendingKillInstance = nil

			// Handle other confirmations via callbacks (e.g., push)
			if overlay != nil {
				if confirmed && overlay.OnConfirm != nil {
					overlay.OnConfirm()
				} else if cancelled && overlay.OnCancel != nil {
					overlay.OnCancel()
				}
			}

			return m, nil
		}
		return m, nil
	}

	// Exit scrolling mode when ESC is pressed and preview pane is in scrolling mode
	// Check if Escape key was pressed and we're not in the diff tab (meaning we're in preview tab)
	// Always check for escape key first to ensure it doesn't get intercepted elsewhere
	if msg.Type == tea.KeyEsc {
		// If in preview tab and in scroll mode, exit scroll mode
		if !m.tabbedWindow.IsInDiffTab() && m.tabbedWindow.IsPreviewInScrollMode() {
			// Use the selected instance from the list
			selected := m.list.GetSelectedInstance()
			err := m.tabbedWindow.ResetPreviewToNormalMode(selected)
			if err != nil {
				return m, m.handleError(err)
			}
			return m, m.instanceChanged()
		}
	}

	// Handle quit commands first
	if msg.String() == "ctrl+c" || msg.String() == "q" {
		return m.handleQuit()
	}

	// Handle hotkey numbers 1-9 in stateDefault
	keyStr := msg.String()
	if len(keyStr) == 1 && keyStr[0] >= '1' && keyStr[0] <= '9' {
		if command, ok := m.hotkeys[keyStr]; ok {
			selected := m.list.GetSelectedInstance()
			if selected != nil && !selected.Paused() && selected.Started() {
				if err := selected.SendPrompt(command); err != nil {
					return m, m.handleError(err)
				}
				return m, nil
			}
		}
	}

	name, ok := keys.GlobalKeyStringsMap[msg.String()]
	if !ok {
		return m, nil
	}

	switch name {
	case keys.KeyHelp:
		return m.showHelpScreen(helpTypeGeneral{}, nil)
	case keys.KeyPrompt:
		if m.list.NumInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:   "",
			Path:    ".",
			Program: m.program,
		})
		if err != nil {
			return m, m.handleError(err)
		}

		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)
		m.promptAfterName = true

		return m, nil
	case keys.KeyNew:
		if m.list.NumInstances() >= GlobalInstanceLimit {
			return m, m.handleError(
				fmt.Errorf("you can't create more than %d instances", GlobalInstanceLimit))
		}
		instance, err := session.NewInstance(session.InstanceOptions{
			Title:   "",
			Path:    ".",
			Program: m.program,
		})
		if err != nil {
			return m, m.handleError(err)
		}

		m.newInstanceFinalizer = m.list.AddInstance(instance)
		m.list.SetSelectedInstance(m.list.NumInstances() - 1)
		m.state = stateNew
		m.menu.SetState(ui.StateNewInstance)

		return m, nil
	case keys.KeyUp:
		m.list.Up()
		return m, m.instanceChanged()
	case keys.KeyDown:
		m.list.Down()
		return m, m.instanceChanged()
	case keys.KeyShiftUp:
		m.tabbedWindow.ScrollUp()
		return m, m.instanceChanged()
	case keys.KeyShiftDown:
		m.tabbedWindow.ScrollDown()
		return m, m.instanceChanged()
	case keys.KeyTab:
		m.tabbedWindow.Toggle()
		m.menu.SetInDiffTab(m.tabbedWindow.IsInDiffTab())
		return m, m.instanceChanged()
	case keys.KeyKill:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Store the instance for async deletion after confirmation
		m.pendingKillInstance = selected

		// Show confirmation modal
		message := fmt.Sprintf("[!] Kill session '%s'?", selected.Title)
		m.state = stateConfirm
		m.confirmationOverlay = overlay.NewConfirmationOverlay(message)
		m.confirmationOverlay.SetWidth(50)

		return m, nil
	case keys.KeySubmit:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Create the push action as a tea.Cmd
		pushAction := func() tea.Msg {
			// Default commit message with timestamp
			commitMsg := fmt.Sprintf("[claudesquad] update from '%s' on %s", selected.Title, time.Now().Format(time.RFC822))
			worktree, err := selected.GetGitWorktree()
			if err != nil {
				return err
			}
			if err = worktree.PushChanges(commitMsg, true); err != nil {
				return err
			}
			return nil
		}

		// Show confirmation modal
		message := fmt.Sprintf("[!] Push changes from session '%s'?", selected.Title)
		return m, m.confirmAction(message, pushAction)
	case keys.KeyCheckout:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}

		// Show help screen before pausing
		m.showHelpScreen(helpTypeInstanceCheckout{}, func() {
			if err := selected.Pause(); err != nil {
				m.handleError(err)
			}
			m.instanceChanged()
		})
		return m, nil
	case keys.KeyResume:
		selected := m.list.GetSelectedInstance()
		if selected == nil {
			return m, nil
		}
		if err := selected.Resume(); err != nil {
			return m, m.handleError(err)
		}
		return m, tea.WindowSize()
	case keys.KeyEnter:
		if m.list.NumInstances() == 0 {
			return m, nil
		}
		selected := m.list.GetSelectedInstance()
		if selected == nil || selected.Paused() || selected.Status == session.Loading || !selected.TmuxAlive() {
			return m, nil
		}
		// Show help screen before attaching
		m.showHelpScreen(helpTypeInstanceAttach{}, func() {
			ch, err := m.list.Attach()
			if err != nil {
				m.handleError(err)
				return
			}
			<-ch
			m.state = stateDefault
		})
		return m, nil
	default:
		return m, nil
	}
}

// instanceChanged updates the preview pane, menu, and diff pane based on the selected instance. It returns an error
// Cmd if there was any error.
func (m *home) instanceChanged() tea.Cmd {
	// selected may be nil
	selected := m.list.GetSelectedInstance()

	m.tabbedWindow.UpdateDiff(selected)
	m.tabbedWindow.SetInstance(selected)
	// Update menu with current instance
	m.menu.SetInstance(selected)

	// If there's no selected instance, we don't need to update the preview.
	if err := m.tabbedWindow.UpdatePreview(selected); err != nil {
		return m.handleError(err)
	}
	return nil
}

type keyupMsg struct{}

// keydownCallback clears the menu option highlighting after 500ms.
func (m *home) keydownCallback(name keys.KeyName) tea.Cmd {
	m.menu.Keydown(name)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(500 * time.Millisecond):
		}

		return keyupMsg{}
	}
}

// hideErrMsg implements tea.Msg and clears the error text from the screen.
type hideErrMsg struct{}

// previewTickMsg implements tea.Msg and triggers a preview update
type previewTickMsg struct{}

type tickUpdateMetadataMessage struct{}

type instanceChangedMsg struct{}

// instanceDeletedMsg signals that async instance deletion has completed
type instanceDeletedMsg struct {
	instance *session.Instance
	err      error
}

// instanceProgressMsg is sent during async instance initialization to report progress
type instanceProgressMsg struct {
	instance *session.Instance
	progress session.InitProgress
	channel  <-chan session.InitProgress
	// Captured state from when initialization started
	finalizer       func()
	promptAfterName bool
}

// instanceStartCompleteMsg signals that async instance initialization has completed
type instanceStartCompleteMsg struct {
	instance        *session.Instance
	err             error
	finalizer       func()
	promptAfterName bool
}

// pendingPromptSentMsg signals that a pending prompt was sent after waiting for input ready
type pendingPromptSentMsg struct {
	instance *session.Instance
	err      error
}

// sendPendingPromptCmd waits for the instance to be ready and sends the pending prompt
func sendPendingPromptCmd(instance *session.Instance, prompt string) tea.Cmd {
	return func() tea.Msg {
		// Wait for the program to be ready to accept input (up to 5 seconds)
		_ = instance.WaitForInputReady(5 * time.Second)

		// Send the prompt
		err := instance.SendPrompt(prompt)
		return pendingPromptSentMsg{
			instance: instance,
			err:      err,
		}
	}
}

// deleteInstanceCmd performs async instance deletion
func deleteInstanceCmd(instance *session.Instance, storage *session.Storage) tea.Cmd {
	return func() tea.Msg {
		// Check if branch is checked out
		worktree, err := instance.GetGitWorktree()
		if err != nil {
			return instanceDeletedMsg{instance: instance, err: err}
		}

		checkedOut, err := worktree.IsBranchCheckedOut()
		if err != nil {
			return instanceDeletedMsg{instance: instance, err: err}
		}

		if checkedOut {
			return instanceDeletedMsg{
				instance: instance,
				err:      fmt.Errorf("instance %s is currently checked out", instance.Title),
			}
		}

		// Delete from storage first
		if err := storage.DeleteInstance(instance.Title); err != nil {
			return instanceDeletedMsg{instance: instance, err: err}
		}

		// Then kill the instance (tmux session + git worktree cleanup)
		if err := instance.Kill(); err != nil {
			return instanceDeletedMsg{instance: instance, err: err}
		}

		return instanceDeletedMsg{instance: instance, err: nil}
	}
}

// tickUpdateMetadataCmd is the callback to update the metadata of the instances every 500ms. Note that we iterate
// overall the instances and capture their output. It's a pretty expensive operation. Let's do it 2x a second only.
var tickUpdateMetadataCmd = func() tea.Msg {
	time.Sleep(500 * time.Millisecond)
	return tickUpdateMetadataMessage{}
}

// handleError handles all errors which get bubbled up to the app. sets the error message. We return a callback tea.Cmd that returns a hideErrMsg message
// which clears the error message after 3 seconds.
func (m *home) handleError(err error) tea.Cmd {
	log.ErrorLog.Printf("%v", err)
	m.errBox.SetError(err)
	return func() tea.Msg {
		select {
		case <-m.ctx.Done():
		case <-time.After(3 * time.Second):
		}

		return hideErrMsg{}
	}
}

// startInstanceCmd starts instance initialization asynchronously and returns the first progress message
func startInstanceCmd(instance *session.Instance, finalizer func(), promptAfterName bool) tea.Cmd {
	return func() tea.Msg {
		progress := make(chan session.InitProgress, 1)
		go instance.StartWithProgress(true, progress)

		// Wait for first progress message
		p := <-progress
		return instanceProgressMsg{
			instance:        instance,
			progress:        p,
			channel:         progress,
			finalizer:       finalizer,
			promptAfterName: promptAfterName,
		}
	}
}

// listenForProgressCmd continues listening for progress updates from the channel
func listenForProgressCmd(instance *session.Instance, ch <-chan session.InitProgress, finalizer func(), promptAfterName bool) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			// Channel closed, initialization complete
			return instanceStartCompleteMsg{
				instance:        instance,
				finalizer:       finalizer,
				promptAfterName: promptAfterName,
			}
		}

		if p.Stage == session.StageComplete {
			return instanceStartCompleteMsg{
				instance:        instance,
				finalizer:       finalizer,
				promptAfterName: promptAfterName,
			}
		}

		if p.Stage == session.StageFailed {
			return instanceStartCompleteMsg{
				instance: instance,
				err:      p.Error,
			}
		}

		return instanceProgressMsg{
			instance:        instance,
			progress:        p,
			channel:         ch,
			finalizer:       finalizer,
			promptAfterName: promptAfterName,
		}
	}
}

// confirmAction shows a confirmation modal and stores the action to execute on confirm
func (m *home) confirmAction(message string, action tea.Cmd) tea.Cmd {
	m.state = stateConfirm

	// Create and show the confirmation overlay using ConfirmationOverlay
	m.confirmationOverlay = overlay.NewConfirmationOverlay(message)
	// Set a fixed width for consistent appearance
	m.confirmationOverlay.SetWidth(50)

	// Set callbacks for confirmation and cancellation
	m.confirmationOverlay.OnConfirm = func() {
		m.state = stateDefault
		// Execute the action if it exists
		if action != nil {
			_ = action()
		}
	}

	m.confirmationOverlay.OnCancel = func() {
		m.state = stateDefault
	}

	return nil
}

func (m *home) View() string {
	listWithPadding := lipgloss.NewStyle().PaddingTop(1).Render(m.list.String())
	previewWithPadding := lipgloss.NewStyle().PaddingTop(1).Render(m.tabbedWindow.String())
	listAndPreview := lipgloss.JoinHorizontal(lipgloss.Top, listWithPadding, previewWithPadding)

	// Show init progress message if present
	var statusLine string
	if m.initProgressMessage != "" {
		statusStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true)
		statusLine = statusStyle.Render(fmt.Sprintf("  %s %s", m.spinner.View(), m.initProgressMessage))
	}

	mainView := lipgloss.JoinVertical(
		lipgloss.Center,
		listAndPreview,
		statusLine,
		m.menu.String(),
		m.errBox.String(),
	)

	if m.state == statePrompt {
		if m.autocompleteInputOverlay == nil {
			log.ErrorLog.Printf("autocomplete input overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.autocompleteInputOverlay.Render(), mainView, true, true)
	} else if m.state == stateHelp {
		if m.textOverlay == nil {
			log.ErrorLog.Printf("text overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.textOverlay.Render(), mainView, true, true)
	} else if m.state == stateConfirm {
		if m.confirmationOverlay == nil {
			log.ErrorLog.Printf("confirmation overlay is nil")
		}
		return overlay.PlaceOverlay(0, 0, m.confirmationOverlay.Render(), mainView, true, true)
	}

	return mainView
}
