package attributes

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix" // For extended attributes
)

// SetMarker sets an extended attribute on a file.
func SetMarker(filePath string, attrName string, attrValue string) error {
	// Attribute names for user-defined attributes should typically start with "user."
	fullAttrName := "user." + attrName
	err := unix.Setxattr(filePath, fullAttrName, []byte(attrValue), 0)
	if err != nil {
		return fmt.Errorf("failed to set xattr '%s' on '%s': %w", fullAttrName, filePath, err)
	}
	log.Printf("Set marker '%s=%s' on file: %s", fullAttrName, attrValue, filePath)
	return nil
}

// GetMarker retrieves an extended attribute from a file.
func GetMarker(filePath string, attrName string) (string, error) {
	fullAttrName := "user." + attrName
	data := make([]byte, 256) // Adjust buffer size as needed, or get size first
	sz, err := unix.Getxattr(filePath, fullAttrName, data)
	if err != nil {
		if err == unix.ENODATA {
			return "", nil // Attribute not found
		}
		return "", fmt.Errorf("failed to get xattr '%s' from '%s': %w", fullAttrName, filePath, err)
	}
	return string(data[:sz]), nil
}

// RemoveMarker removes an extended attribute from a file.
func RemoveMarker(filePath string, attrName string) error {
	fullAttrName := "user." + attrName
	err := unix.Removexattr(filePath, fullAttrName)
	if err != nil {
		if err == unix.ENODATA {
			return nil // Attribute not found, nothing to remove
		}
		return fmt.Errorf("failed to remove xattr '%s' from '%s': %w", fullAttrName, filePath, err)
	}
	log.Printf("Removed marker '%s' from file: %s", fullAttrName, filePath)
	return nil
}

// HasMarker checks if a file has a specific extended attribute set.
// It returns true if the attribute exists and has a non-empty value, false otherwise.
func HasMarker(filePath string, attrName string) (bool, error) {
	fullAttrName := "user." + attrName
	data := make([]byte, 1)

	_, err := unix.Getxattr(filePath, fullAttrName, data)
	if err != nil {
		if err == unix.ENODATA {
			return false, nil
		}
		return false, fmt.Errorf("failed to get xattr '%s' from '%s': %w", fullAttrName, filePath, err)
	}

	valueData := make([]byte, 256)
	sz, err := unix.Getxattr(filePath, fullAttrName, valueData)
	if err != nil {
		if err == unix.ENODATA {
			return false, nil
		}
		return false, fmt.Errorf("failed to get xattr value for '%s' from '%s': %w", fullAttrName, filePath, err)
	}
	return sz > 0, nil
}

// GetFilesWithMarker scans a directory and returns a list of file paths
// for files that have the specified extended attribute (marker).
func GetFilesWithMarker(directory string, attrName string) ([]string, error) {
	markedFiles := []string{}

	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory '%s': %w", directory, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filePath := filepath.Join(directory, entry.Name())
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			log.Printf("Warning: Could not stat file '%s': %v. Skipping.", filePath, err)
			continue
		}
		if !fileInfo.Mode().IsRegular() {
			continue
		}

		hasAttr, err := HasMarker(filePath, attrName)
		if err != nil {
			log.Printf("Warning: Could not check marker for file '%s': %v", filePath, err)
			continue
		}

		if hasAttr {
			markedFiles = append(markedFiles, filePath)
		}
	}
	return markedFiles, nil
}
