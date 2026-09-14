package paste

import (
	"log"
	"os"
	"os/exec"
	"time"
)

// SimulateCopy sends Ctrl+C to the active window to copy currently selected/highlighted text
func SimulateCopy() error {
	// Give a slight delay (80ms) for global shortcut keys to be released
	time.Sleep(80 * time.Millisecond)

	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		os.Setenv("DISPLAY", ":0")
	}

	isWayland := os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland"

	if isWayland {
		// Try wtype
		if _, err := exec.LookPath("wtype"); err == nil {
			cmd := exec.Command("wtype", "-M", "ctrl", "-k", "c", "-m", "ctrl")
			if err := cmd.Run(); err == nil {
				time.Sleep(80 * time.Millisecond)
				return nil
			}
		}
		// Try ydotool (key 29: Ctrl, 46: c)
		if _, err := exec.LookPath("ydotool"); err == nil {
			cmd := exec.Command("ydotool", "key", "29:1", "46:1", "46:0", "29:0")
			if err := cmd.Run(); err == nil {
				time.Sleep(80 * time.Millisecond)
				return nil
			}
		}
	}

	// Try xdotool (standard X11 tool)
	if _, err := exec.LookPath("xdotool"); err == nil {
		cmd := exec.Command("xdotool", "key", "--clearmodifiers", "ctrl+c")
		if err := cmd.Run(); err == nil {
			time.Sleep(80 * time.Millisecond)
			return nil
		}
	}

	return nil
}

// SimulatePaste sends Ctrl+V or Shift+Insert to the active window
func SimulatePaste() error {
	// Give a slight delay (50ms) to ensure clipboard buffer is fully registered
	time.Sleep(50 * time.Millisecond)

	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		os.Setenv("DISPLAY", ":0")
	}

	isWayland := os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland"

	if isWayland {
		// Try wtype
		if _, err := exec.LookPath("wtype"); err == nil {
			cmd := exec.Command("wtype", "-M", "ctrl", "-k", "v", "-m", "ctrl")
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
		// Try ydotool (key 29: Ctrl, 47: v)
		if _, err := exec.LookPath("ydotool"); err == nil {
			cmd := exec.Command("ydotool", "key", "29:1", "47:1", "47:0", "29:0")
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
	}

	// Try xdotool (standard X11 tool)
	if _, err := exec.LookPath("xdotool"); err == nil {
		cmd := exec.Command("xdotool", "key", "--clearmodifiers", "ctrl+v")
		if err := cmd.Run(); err == nil {
			return nil
		}
		// Try Shift+Insert fallback
		cmdShift := exec.Command("xdotool", "key", "--clearmodifiers", "shift+Insert")
		if err := cmdShift.Run(); err == nil {
			return nil
		}
	}

	log.Printf("⚠️ Auto-paste note: xdotool/wtype/ydotool not found or failed, text is ready in clipboard for manual Ctrl+V")
	return nil
}
