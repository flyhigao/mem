package clipboard

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ensureDisplayEnv sets DISPLAY=:0 if both DISPLAY and WAYLAND_DISPLAY are empty
func ensureDisplayEnv() {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		os.Setenv("DISPLAY", ":0")
	}
}

// isWayland returns true if current session is Wayland
func isWayland() bool {
	return os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland"
}

// SetText writes text to system clipboard using available CLI tools
func SetText(text string) error {
	ensureDisplayEnv()
	wayland := isWayland()

	if wayland {
		if err := writeWlCopy(text); err == nil {
			return nil
		}
	}

	// Try xclip (standard X11)
	if err := writeXclip(text); err == nil {
		return nil
	}

	// Try xsel
	if err := writeXsel(text); err == nil {
		return nil
	}

	// Try python3 GTK3 (supports clip.store() for clipboard persistence across process exits)
	if err := writePythonGtk(text); err == nil {
		return nil
	}

	// Try python3 tkinter fallback
	if err := writePythonTk(text); err == nil {
		return nil
	}

	// Fallback to wl-copy even if wayland env wasn't explicitly set
	if !wayland {
		if err := writeWlCopy(text); err == nil {
			return nil
		}
	}

	return fmt.Errorf("no clipboard tool found (please install xclip, xsel, or wl-clipboard)")
}

// GetText reads text from system clipboard using available CLI tools
func GetText() (string, error) {
	ensureDisplayEnv()
	wayland := isWayland()

	if wayland {
		if text, err := readWlPaste(); err == nil && text != "" {
			return text, nil
		}
	}

	// Try xclip
	if text, err := readXclip(); err == nil && text != "" {
		return text, nil
	}

	// Try xsel
	if text, err := readXsel(); err == nil && text != "" {
		return text, nil
	}

	// Try python3 GTK3
	if text, err := readPythonGtk(); err == nil && text != "" {
		return text, nil
	}

	// Try python3 tkinter
	if text, err := readPythonTk(); err == nil && text != "" {
		return text, nil
	}

	// Fallback to wl-paste even if wayland env wasn't explicitly set
	if !wayland {
		if text, err := readWlPaste(); err == nil && text != "" {
			return text, nil
		}
	}

	return "", fmt.Errorf("no clipboard tool found or clipboard is empty (tried wl-paste, xclip, xsel, python3)")
}

// GetPrimaryText reads text from system PRIMARY selection (mouse highlighted text)
func GetPrimaryText() (string, error) {
	ensureDisplayEnv()
	wayland := isWayland()

	if wayland {
		if text, err := readWlPastePrimary(); err == nil && text != "" {
			return text, nil
		}
	}

	// Try xclip
	if text, err := readXclipPrimary(); err == nil && text != "" {
		return text, nil
	}

	// Try xsel
	if text, err := readXselPrimary(); err == nil && text != "" {
		return text, nil
	}

	// Try python3 GTK3
	if text, err := readPythonGtkPrimary(); err == nil && text != "" {
		return text, nil
	}

	if !wayland {
		if text, err := readWlPastePrimary(); err == nil && text != "" {
			return text, nil
		}
	}

	return "", fmt.Errorf("no primary selection available")
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
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func writePythonGtk(text string) error {
	if _, err := exec.LookPath("python3"); err != nil {
		return err
	}
	pyScript := `
import sys, os
try:
    null_fd = os.open(os.devnull, os.O_WRONLY)
    os.dup2(null_fd, 2)
    os.close(null_fd)
    import gi
    gi.require_version('Gtk', '3.0')
    from gi.repository import Gtk, Gdk
    text = sys.stdin.read()
    clip = Gtk.Clipboard.get(Gdk.SELECTION_CLIPBOARD)
    clip.set_text(text, -1)
    clip.store()
except Exception:
    sys.exit(1)
`
	cmd := exec.Command("python3", "-c", pyScript)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func writePythonTk(text string) error {
	if _, err := exec.LookPath("python3"); err != nil {
		return err
	}
	pyScript := `
import sys, os
try:
    null_fd = os.open(os.devnull, os.O_WRONLY)
    os.dup2(null_fd, 2)
    os.close(null_fd)
    import tkinter as tk
    text = sys.stdin.read()
    r = tk.Tk()
    r.withdraw()
    r.clipboard_clear()
    r.clipboard_append(text)
    r.update()
    r.destroy()
except Exception:
    sys.exit(1)
`
	cmd := exec.Command("python3", "-c", pyScript)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func readWlPaste() (string, error) {
	if _, err := exec.LookPath("wl-paste"); err != nil {
		return "", err
	}
	cmd := exec.Command("wl-paste", "--no-newline")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

func readWlPastePrimary() (string, error) {
	if _, err := exec.LookPath("wl-paste"); err != nil {
		return "", err
	}
	cmd := exec.Command("wl-paste", "--primary", "--no-newline")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

func readXclip() (string, error) {
	if _, err := exec.LookPath("xclip"); err != nil {
		return "", err
	}
	cmd := exec.Command("xclip", "-selection", "clipboard", "-out")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

func readXclipPrimary() (string, error) {
	if _, err := exec.LookPath("xclip"); err != nil {
		return "", err
	}
	cmd := exec.Command("xclip", "-selection", "primary", "-out")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

func readXsel() (string, error) {
	if _, err := exec.LookPath("xsel"); err != nil {
		return "", err
	}
	cmd := exec.Command("xsel", "--clipboard", "--output")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

func readXselPrimary() (string, error) {
	if _, err := exec.LookPath("xsel"); err != nil {
		return "", err
	}
	cmd := exec.Command("xsel", "--primary", "--output")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

func readPythonGtk() (string, error) {
	if _, err := exec.LookPath("python3"); err != nil {
		return "", err
	}
	pyScript := `
import sys, os
try:
    null_fd = os.open(os.devnull, os.O_WRONLY)
    os.dup2(null_fd, 2)
    os.close(null_fd)
    import gi
    gi.require_version('Gtk', '3.0')
    from gi.repository import Gtk, Gdk
    clip = Gtk.Clipboard.get(Gdk.SELECTION_CLIPBOARD)
    t = clip.wait_for_text()
    if t is not None:
        sys.stdout.write(t)
    else:
        sys.exit(1)
except Exception:
    sys.exit(1)
`
	cmd := exec.Command("python3", "-c", pyScript)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

func readPythonGtkPrimary() (string, error) {
	if _, err := exec.LookPath("python3"); err != nil {
		return "", err
	}
	pyScript := `
import sys, os
try:
    null_fd = os.open(os.devnull, os.O_WRONLY)
    os.dup2(null_fd, 2)
    os.close(null_fd)
    import gi
    gi.require_version('Gtk', '3.0')
    from gi.repository import Gtk, Gdk
    primary = Gtk.Clipboard.get(Gdk.SELECTION_PRIMARY)
    t = primary.wait_for_text()
    if t is not None:
        sys.stdout.write(t)
    else:
        sys.exit(1)
except Exception:
    sys.exit(1)
`
	cmd := exec.Command("python3", "-c", pyScript)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}

func readPythonTk() (string, error) {
	if _, err := exec.LookPath("python3"); err != nil {
		return "", err
	}
	pyScript := `
import sys, os
try:
    null_fd = os.open(os.devnull, os.O_WRONLY)
    os.dup2(null_fd, 2)
    os.close(null_fd)
    import tkinter as tk
    r = tk.Tk()
    r.withdraw()
    try:
        t = r.clipboard_get()
        sys.stdout.write(t)
    except Exception:
        sys.exit(1)
    finally:
        r.destroy()
except Exception:
    sys.exit(1)
`
	cmd := exec.Command("python3", "-c", pyScript)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}
