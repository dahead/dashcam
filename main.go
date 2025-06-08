package main

import (
	"context"
	"dashcam/internal/attributes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"
	// "dashcam/internal/attributes"
)

// Config holds the application configuration
type Config struct {
	RecordingsDir   string `json:"recordings_dir"`
	MaxFiles        int    `json:"max_files"`
	RecordingLength int    `json:"recording_length_seconds"`
	Extension       string `json:"extension"`
	Codec           string `json:"codec"`
	RecordAudio     bool   `json:"record_audio"`
	// EmergencyHotkey string `json:"emergency_hotkey"`
}

// Default const config filename
const configFilename = "dashcam.json"
const attributeMarkerName = "dashcam"
const attributeMarkerDefaultValue = "standard_recording" // Indicates a normal, continuous recording segment
// const attributeMarkerEmergencyValue = "emergency_recording"
// var EmergencyKeyPressed = false

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	return Config{
		RecordingsDir:   filepath.Join(homeDir, "recordings"),
		MaxFiles:        60,
		RecordingLength: 60,
		Extension:       ".mkv",
		Codec:           "libx265",
		RecordAudio:     false,
		// EmergencyHotkey: "CTRL+SUPER+E",
	}
}

// LoadConfig loads configuration from the user's home directory
func LoadConfig() (Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return DefaultConfig(), err
	}

	configPath := filepath.Join(homeDir, configFilename)

	// If config file doesn't exist, create it with defaults
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		config := DefaultConfig()
		if err := SaveConfig(config); err != nil {
			log.Printf("Warning: Could not save default config: %v", err)
		}
		return config, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return DefaultConfig(), err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return DefaultConfig(), err
	}

	return config, nil
}

// SaveConfig saves configuration to the user's home directory
func SaveConfig(config Config) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(homeDir, configFilename)

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

// ScreenRecorder handles the screen recording functionality
type ScreenRecorder struct {
	config Config
}

// NewScreenRecorder creates a new screen recorder instance
func NewScreenRecorder(config Config) *ScreenRecorder {
	return &ScreenRecorder{config: config}
}

// ensureRecordingsDir creates the recordings directory if it doesn't exist
func (sr *ScreenRecorder) ensureRecordingsDir() error {
	return os.MkdirAll(sr.config.RecordingsDir, 0755)
}

// generateFilename creates a filename based on current timestamp
func (sr *ScreenRecorder) generateFilename() string {
	timestamp := time.Now().Format("2025-01-02_15-35-05")
	return filepath.Join(sr.config.RecordingsDir, timestamp+sr.config.Extension)
}

// recordScreen records the screen for the specified duration
func (sr *ScreenRecorder) recordScreen(filename string, duration int) error {
	log.Printf("Starting recording: %s (duration: %d seconds)", filename, duration)

	// Create context for the recording
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use wf-recorder with MKV format (native format)
	cmd := exec.CommandContext(ctx, "wf-recorder", "-f", filename)

	// User codec set?
	if sr.config.Codec != "" {
		cmd.Args = append(cmd.Args, "-c", sr.config.Codec)
	}

	// Enable audio recording
	if !sr.config.RecordAudio {
		cmd.Args = append(cmd.Args, "-a")
	}

	// Start the recording
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start wf-recorder: %v", err)
	}

	// Create a timer to stop recording after specified duration
	timer := time.NewTimer(time.Duration(duration) * time.Second)
	defer timer.Stop()

	// Wait for either the timer or process to finish
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-timer.C:
		// Time's up - send SIGINT (Ctrl+C) to wf-recorder for clean shutdown
		log.Printf("Recording duration %d seconds reached, sending Ctrl+C to wf-recorder...", duration)
		if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
			log.Printf("Warning: Could not send SIGINT to wf-recorder: %v", err)
			// Fallback to killing the process
			cmd.Process.Kill()
		}

		// Wait a bit for graceful shutdown
		select {
		case err := <-done:
			if err != nil {
				log.Printf("wf-recorder finished with: %v", err)
			}
		case <-time.After(5 * time.Second):
			log.Printf("wf-recorder didn't respond to SIGINT, killing process...")
			cmd.Process.Kill()
			<-done // Wait for it to actually die
		}
		log.Printf("Recording completed: %s", filename)
	case err := <-done:
		// Process finished on its own
		if err != nil {
			return fmt.Errorf("wf-recorder failed: %v", err)
		}
		log.Printf("Recording completed: %s", filename)
		return nil
	}

	return nil
}

// cleanupOldFiles removes old video files to maintain the max file limit
func (sr *ScreenRecorder) cleanupOldFiles() error {
	// Only get files marked with dashcam-attributes
	files, err := attributes.GetFilesWithMarker(sr.config.RecordingsDir, attributeMarkerName)

	if err != nil {
		return err
	}

	if len(files) <= sr.config.MaxFiles {
		return nil
	}

	// Sort files by modification time (oldest first)
	sort.Slice(files, func(i, j int) bool {
		info1, err1 := os.Stat(files[i])
		info2, err2 := os.Stat(files[j])
		if err1 != nil || err2 != nil {
			return false
		}
		return info1.ModTime().Before(info2.ModTime())
	})

	// Remove excess files
	filesToRemove := len(files) - sr.config.MaxFiles
	for i := 0; i < filesToRemove; i++ {
		log.Printf("Removing old recording: %s", filepath.Base(files[i]))
		if err := os.Remove(files[i]); err != nil {
			log.Printf("Warning: Could not remove file %s: %v", files[i], err)
		}
	}

	return nil
}

// Start begins the continuous recording process
func (sr *ScreenRecorder) Start() error {
	if err := sr.ensureRecordingsDir(); err != nil {
		return fmt.Errorf("failed to create recordings directory: %v", err)
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("Screen recorder started.")
	log.Println("Press Ctrl+C to stop recording...")
	loopcounter := 0

	// Channel to signal when to stop
	stopChan := make(chan bool, 1)

	// Goroutine to handle signals
	go func() {
		<-sigChan
		log.Println("Received shutdown signal. Stopping recorder...")
		stopChan <- true
	}()

	// Main recording loop
	for {
		loopcounter += 1

		select {
		case <-stopChan:
			log.Println("Screen recorder stopped.")
			return nil
		default:
			filename := sr.generateFilename()

			// Record screen
			if err := sr.recordScreen(filename, sr.config.RecordingLength); err != nil {
				log.Printf("Recording failed: %v", err)
				// Wait a bit before trying again to avoid rapid failures
				time.Sleep(2 * time.Second)
				continue
			}

			//// Todo: If "Emergency-Hotkey" was pressed, save and mark video under "emergency"
			//attrvalue := attributeMarkerDefaultValue
			//if EmergencyKeyPressed {
			//	attrvalue = attributeMarkerEmergencyValue
			//	EmergencyKeyPressed = false
			//}
			//// Mark file as dashcam recording
			//if err := attributes.SetMarker(filename, attributeMarkerName, attrvalue); err != nil {
			//	log.Printf("Warning: Failed to set marker on file '%s': %v", filename, err)
			//}

			// Mark file as dashcam recording
			if err := attributes.SetMarker(filename, attributeMarkerName, attributeMarkerDefaultValue); err != nil {
				log.Printf("Warning: Failed to set marker on file '%s': %v", filename, err)
			}

			// Cleanup old files
			if loopcounter%10 == 0 {
				if err := sr.cleanupOldFiles(); err != nil {
					log.Printf("Warning: Failed to cleanup old files: %v", err)
				}
			}
		}
	}
}

//func MarkCurrentVideoEmergency() {
//	//exec.Command("kitty").Start()
//	// mark current video as emergency
//	EmergencyKeyPressed = true
//	// fmt.Println("Emergency hotkey pressed!") // print to STDOUT
//	log.Println("Emergency hotkey pressed!")
//}

func main() {
	log.Printf("Loading configuration from %s...\n", configFilename)

	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		log.Printf("Warning: Could not load config, using defaults: %v", err)
		config = DefaultConfig()
	}

	// Display current configuration
	log.Printf("Configuration loaded:")
	log.Printf("  Recordings directory: %s", config.RecordingsDir)
	log.Printf("  Max files to keep: %d", config.MaxFiles)
	log.Printf("  Recording length: %d seconds", config.RecordingLength)
	log.Printf("  Codec: %s", config.Codec)
	log.Printf("  Audio recording enabled: %v", config.RecordAudio)

	// Check if wf-recorder is available
	if _, err := exec.LookPath("wf-recorder"); err != nil {
		log.Fatal("wf-recorder not found. Please install wf-recorder first.")
	}

	//// Hyprland Hotkey Manager (watch for hotkey so  we know its an emergency recording)
	//manager, _ := hotkey.NewHyprlandHotkeyManager()
	//defer manager.Close()
	//
	//// Register hotkeys
	//manager.RegisterHotkey(config.EmergencyHotkey, func(hotkey string) {
	//	MarkCurrentVideoEmergency()
	//})
	//
	//// Start listening
	//manager.StartListening()

	// Create and start screen recorder
	recorder := NewScreenRecorder(config)
	if err := recorder.Start(); err != nil {
		log.Fatalf("Screen recorder failed: %v", err)
	}
}
