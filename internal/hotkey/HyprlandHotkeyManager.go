package hotkey

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// HotkeyCallback represents a callback function for hotkey events
type HotkeyCallback func(hotkey string)

// HotkeyEntry represents a registered hotkey
type HotkeyEntry struct {
	ID       string
	Hotkey   string
	Callback HotkeyCallback
	Active   bool
}

// HyprlandHotkeyManager manages hotkeys for Hyprland
type HyprlandHotkeyManager struct {
	pipePath     string
	hotkeys      map[string]*HotkeyEntry
	hotkeysMutex sync.RWMutex
	listening    bool
	stopChan     chan bool
	instanceSig  string
}

// NewHyprlandHotkeyManager creates a new hotkey manager
func NewHyprlandHotkeyManager() (*HyprlandHotkeyManager, error) {
	// Check if we're running under Hyprland
	instanceSig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	if instanceSig == "" {
		return nil, fmt.Errorf("HYPRLAND_INSTANCE_SIGNATURE not found - are you running under Hyprland?")
	}

	pipePath := "/tmp/hyprland_hotkey_pipe"

	manager := &HyprlandHotkeyManager{
		pipePath:    pipePath,
		hotkeys:     make(map[string]*HotkeyEntry),
		stopChan:    make(chan bool),
		instanceSig: instanceSig,
	}

	// Create the named pipe
	if err := manager.createPipe(); err != nil {
		return nil, fmt.Errorf("failed to create pipe: %v", err)
	}

	return manager, nil
}

// createPipe creates the named pipe for communication
func (hm *HyprlandHotkeyManager) createPipe() error {
	// Remove existing pipe if it exists
	os.Remove(hm.pipePath)

	// Create new pipe
	err := syscall.Mkfifo(hm.pipePath, 0666)
	if err != nil {
		return fmt.Errorf("failed to create named pipe: %v", err)
	}

	log.Printf("Created named pipe at: %s", hm.pipePath)
	return nil
}

// parseHotkey converts common hotkey format to Hyprland format
func (hm *HyprlandHotkeyManager) parseHotkey(hotkey string) (string, string) {
	parts := strings.Split(strings.ToUpper(strings.ReplaceAll(hotkey, " ", "")), "+")

	var mods []string
	var key string

	for _, part := range parts {
		switch part {
		case "CTRL", "CONTROL":
			mods = append(mods, "CTRL")
		case "ALT":
			mods = append(mods, "ALT")
		case "SHIFT":
			mods = append(mods, "SHIFT")
		case "SUPER", "WIN", "WINDOWS", "CMD":
			mods = append(mods, "SUPER")
		case "ENTER", "RETURN":
			key = "Return"
		case "SPACE":
			key = "space"
		case "TAB":
			key = "Tab"
		case "ESC", "ESCAPE":
			key = "Escape"
		case "BACKSPACE":
			key = "BackSpace"
		case "DELETE", "DEL":
			key = "Delete"
		case "HOME":
			key = "Home"
		case "END":
			key = "End"
		case "PAGEUP":
			key = "Prior"
		case "PAGEDOWN":
			key = "Next"
		case "UP":
			key = "Up"
		case "DOWN":
			key = "Down"
		case "LEFT":
			key = "Left"
		case "RIGHT":
			key = "Right"
		default:
			// Function keys
			if strings.HasPrefix(part, "F") && len(part) > 1 {
				key = part
			} else if len(part) == 1 {
				// Single character key
				key = strings.ToLower(part)
			} else {
				key = part
			}
		}
	}

	modStr := strings.Join(mods, " ")
	return modStr, key
}

// generateHotkeyID generates a unique ID for a hotkey
func (hm *HyprlandHotkeyManager) generateHotkeyID(hotkey string) string {
	return fmt.Sprintf("hotkey_%s_%d",
		strings.ReplaceAll(strings.ReplaceAll(hotkey, "+", "_"), " ", ""),
		time.Now().UnixNano())
}

// RegisterHotkey registers a new hotkey with callback
func (hm *HyprlandHotkeyManager) RegisterHotkey(hotkey string, callback HotkeyCallback) (string, error) {
	hm.hotkeysMutex.Lock()
	defer hm.hotkeysMutex.Unlock()

	// Generate unique ID
	id := hm.generateHotkeyID(hotkey)

	// Parse hotkey
	mod, key := hm.parseHotkey(hotkey)
	if key == "" {
		return "", fmt.Errorf("invalid hotkey format: %s", hotkey)
	}

	// Create command that will write to our pipe
	command := fmt.Sprintf("echo '%s' > %s", id, hm.pipePath)

	// Register with Hyprland
	var cmd *exec.Cmd
	if mod != "" {
		cmd = exec.Command("hyprctl", "keyword", "bind", fmt.Sprintf("%s, %s, exec, %s", mod, key, command))
	} else {
		cmd = exec.Command("hyprctl", "keyword", "bind", fmt.Sprintf(", %s, exec, %s", key, command))
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to register hotkey with Hyprland: %v, output: %s", err, output)
	}

	// Store hotkey entry
	entry := &HotkeyEntry{
		ID:       id,
		Hotkey:   hotkey,
		Callback: callback,
		Active:   true,
	}

	hm.hotkeys[id] = entry

	// log.Printf("Registered hotkey: %s (ID: %s) -> %s %s", hotkey, id, mod, key)
	return id, nil
}

// UnregisterHotkey removes a hotkey registration
func (hm *HyprlandHotkeyManager) UnregisterHotkey(id string) error {
	hm.hotkeysMutex.Lock()
	defer hm.hotkeysMutex.Unlock()

	entry, exists := hm.hotkeys[id]
	if !exists {
		return fmt.Errorf("hotkey with ID %s not found", id)
	}

	// Parse the original hotkey to unbind it
	mod, key := hm.parseHotkey(entry.Hotkey)

	// Unbind from Hyprland by binding to a no-op command
	var cmd *exec.Cmd
	if mod != "" {
		cmd = exec.Command("hyprctl", "keyword", "unbind", fmt.Sprintf("%s, %s", mod, key))
	} else {
		cmd = exec.Command("hyprctl", "keyword", "unbind", fmt.Sprintf(", %s", key))
	}

	if err := cmd.Run(); err != nil {
		log.Printf("Warning: failed to unbind hotkey from Hyprland: %v", err)
	}

	// Remove from our registry
	delete(hm.hotkeys, id)

	// log.Printf("Unregistered hotkey: %s (ID: %s)", entry.Hotkey, id)
	return nil
}

// StartListening starts listening for hotkey events
func (hm *HyprlandHotkeyManager) StartListening() error {
	if hm.listening {
		return fmt.Errorf("already listening")
	}

	hm.listening = true

	go func() {
		// log.Printf("Starting to listen for hotkey events on: %s", hm.pipePath)

		for {
			select {
			case <-hm.stopChan:
				// log.Println("Stopping hotkey listener")
				return
			default:
				// Open pipe for reading (this will block until data is available)
				file, err := os.OpenFile(hm.pipePath, os.O_RDONLY, os.ModeNamedPipe)
				if err != nil {
					log.Printf("Error opening pipe: %v", err)
					time.Sleep(1 * time.Second)
					continue
				}

				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					hotkeyID := strings.TrimSpace(scanner.Text())
					if hotkeyID != "" {
						hm.handleHotkeyEvent(hotkeyID)
					}
				}

				if err := scanner.Err(); err != nil {
					log.Printf("Error reading from pipe: %v", err)
				}

				file.Close()
			}
		}
	}()

	return nil
}

// handleHotkeyEvent processes a hotkey event
func (hm *HyprlandHotkeyManager) handleHotkeyEvent(hotkeyID string) {
	hm.hotkeysMutex.RLock()
	entry, exists := hm.hotkeys[hotkeyID]
	hm.hotkeysMutex.RUnlock()

	if !exists || !entry.Active {
		// log.Printf("Received event for unknown or inactive hotkey ID: %s", hotkeyID)
		return
	}

	log.Printf("Hotkey triggered: %s (ID: %s)", entry.Hotkey, hotkeyID)

	// Execute callback in a goroutine to avoid blocking
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Panic in hotkey callback for %s: %v", entry.Hotkey, r)
			}
		}()

		entry.Callback(entry.Hotkey)
	}()
}

// StopListening stops the hotkey listener
func (hm *HyprlandHotkeyManager) StopListening() {
	if !hm.listening {
		return
	}

	hm.listening = false
	close(hm.stopChan)
}

// GetRegisteredHotkeys returns a list of registered hotkeys
func (hm *HyprlandHotkeyManager) GetRegisteredHotkeys() map[string]*HotkeyEntry {
	hm.hotkeysMutex.RLock()
	defer hm.hotkeysMutex.RUnlock()

	result := make(map[string]*HotkeyEntry)
	for id, entry := range hm.hotkeys {
		result[id] = &HotkeyEntry{
			ID:       entry.ID,
			Hotkey:   entry.Hotkey,
			Callback: entry.Callback,
			Active:   entry.Active,
		}
	}
	return result
}

// SetHotkeyActive enables or disables a hotkey without unregistering it
func (hm *HyprlandHotkeyManager) SetHotkeyActive(id string, active bool) error {
	hm.hotkeysMutex.Lock()
	defer hm.hotkeysMutex.Unlock()

	entry, exists := hm.hotkeys[id]
	if !exists {
		return fmt.Errorf("hotkey with ID %s not found", id)
	}

	entry.Active = active
	log.Printf("Hotkey %s (ID: %s) set to active: %v", entry.Hotkey, id, active)
	return nil
}

// Close cleans up resources
func (hm *HyprlandHotkeyManager) Close() error {
	log.Println("Closing Hyprland hotkey manager...")

	// Stop listening
	hm.StopListening()

	// Unregister all hotkeys
	hm.hotkeysMutex.Lock()
	for id := range hm.hotkeys {
		hm.UnregisterHotkey(id)
	}
	hm.hotkeysMutex.Unlock()

	// Remove pipe
	if err := os.Remove(hm.pipePath); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: failed to remove pipe: %v", err)
	}

	return nil
}
