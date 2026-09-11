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
	// If DISPLAY is not set in non-Wayland environment (e.g. invoked from SSH / background), fallback to :0
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		os.Setenv("DISPLAY", ":0")
	}

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

	// Try python3 tkinter (standard on almost all Linux desktop distros including Deepin)
	if err := writePythonTk(text); err == nil {
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
	if _, err := exec.LookPath("wl-copy"); err != nil {
		return err
	}
	cmd := exec.Command("wl-copy")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func writeXclip(text string) error {
	if _, err := exec.LookPath("xclip"); err != nil {
		return err
	}
	cmd1 := exec.Command("xclip", "-selection", "clipboard")
	cmd1.Stdin = strings.NewReader(text)
	if err := cmd1.Run(); err != nil {
		return err
	}

	cmd2 := exec.Command("xclip", "-selection", "primary")
	cmd2.Stdin = strings.NewReader(text)
	_ = cmd2.Run()

	return nil
}

func writeXsel(text string) error {
	if _, err := exec.LookPath("xsel"); err != nil {
		return err
	}
	cmd := exec.Command("xsel", "--clipboard", "--input")
	cmd.Stdin = bytes.NewReader([]byte(text))
	return cmd.Run()
}

func writePythonTk(text string) error {
	if _, err := exec.LookPath("python3"); err != nil {
		return err
	}
	pyScript := `
import sys, tkinter as tk
text = sys.stdin.read()
r = tk.Tk()
r.withdraw()
r.clipboard_clear()
r.clipboard_append(text)
r.update()
r.destroy()
`
	cmd := exec.Command("python3", "-c", pyScript)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
