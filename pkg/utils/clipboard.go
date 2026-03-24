package utils

import (
	"github.com/atotto/clipboard"
)

// CopyToClipboard copies text to the system clipboard
func CopyToClipboard(text string) error {
	return clipboard.WriteAll(text)
}

// IsClipboardAvailable checks if the clipboard is available on the system
func IsClipboardAvailable() bool {
	// The atotto/clipboard library handles this internally for each platform
	// Just try to read from clipboard to check if it's working
	_, err := clipboard.ReadAll()
	return err == nil
}
