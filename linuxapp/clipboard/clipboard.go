package clipboard

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// SetText writes text to system clipboard using available CLI tools
func SetText(text string) error {
	isWayland := os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland"

	if isWayland {
		if err := writeWlCopy(text); err == nil {
			return nil
		}
	}

	// Try xclip
	if err := writeXclip(text); err == nil {
		return nil
	}

	// Try xsel
	if err := writeXsel(text); err == nil {
		return nil
	}

	// Fallback to wl-copy even if wayland env wasn't explicitly set
	if !isWayland {
		if err := writeWlCopy(text); err == nil {
			return nil
		}
	}

	return fmt.Errorf("no clipboard tool found (please install xclip, xsel, or wl-clipboard)")
}

func writeWlCopy(text string) error {
	cmd := exec.Command("wl-copy")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func writeXclip(text string) error {
	// Write to both CLIPBOARD and PRIMARY selection for best compatibility
	cmd1 := exec.Command("xclip", "-selection", "clipboard")
	cmd1.Stdin = strings.NewReader(text)
	if err := cmd1.Run(); err != nil {
		return err
	}

	// Primary selection (optional)
	cmd2 := exec.Command("xclip", "-selection", "primary")
	cmd2.Stdin = strings.NewReader(text)
	_ = cmd2.Run()

	return nil
}

func writeXsel(text string) error {
	cmd := exec.Command("xsel", "--clipboard", "--input")
	cmd.Stdin = bytes.NewReader([]byte(text))
	return cmd.Run()
}
