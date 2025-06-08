# Dashcam - Screen Recorder written in Go

## Description

This project is a Go application that provides continuous screen recording functionality, similar to a dashcam. It records the screen in segments, manages the number of recording files by deleting older ones, and uses `wf-recorder` for the actual screen capture on Wayland-based systems. Recordings are marked with extended file attributes.

## How it Works

*   The application starts and loads its configuration.
*   It enters a loop, recording screen segments of `recording_length_seconds`.
*   Each recorded file is saved to the `recordings_dir`.
*   An extended file attribute (`user.dashcam`) is set on each recording to identify it.
*   Periodically, the application checks the number of marked recording files. If it exceeds `max_files`, the oldest files (based on modification time) are deleted.
*   The recording process uses the `wf-recorder` command-line tool.

## Prerequisites

*   **Go**: Version 1.24 or higher.
*   **wf-recorder**: This application relies on `wf-recorder` to capture the screen. Ensure it is installed and accessible in your system's PATH.
*   **Linux System with Wayland**: As `wf-recorder` is typically used with Wayland.
*   **Extended Attribute Support**: The filesystem where recordings are stored must support extended attributes (xattr) for file marking.

## Configuration

The application uses a JSON configuration file named `dashcam.json` located in the user's home directory (`~/dashcam.json`).

If the `dashcam.json` file does not exist when the application starts, it will be created with default values.

**Configuration Options:**

*   `recordings_dir` (string): The directory where video recordings will be stored.
    *   Default: `~/recordings`
*   `max_files` (int): The maximum number of recording files to keep. Older files will be deleted to maintain this limit.
    *   Default: `60`
*   `recording_length_seconds` (int): The duration of each individual recording segment in seconds.
    *   Default: `60`
*   `extension` (string): The file extension for the recordings.
    *   Default: `.mkv`
*   `codec` (string): The video codec to be used by `wf-recorder` (e.g., `libx264`, `libx265`). If empty, `wf-recorder`'s default is used.
    *   Default: `libx265`
*   `record_audio` (bool): Whether to record audio along with the video. `false` means audio is *not* recorded (as `wf-recorder`'s `-a` flag is *omitted* if `RecordAudio` is `false`).
    *   Default: `false`

**Example `dashcam.json`:**

```
json { 
    "recordings_dir": "/home/user/recordings", 
    "max_files": 100, 
    "recording_length_seconds": 120, 
    "extension": ".mp4", 
    "codec": "libx264", 
    "record_audio": true 
}
```
