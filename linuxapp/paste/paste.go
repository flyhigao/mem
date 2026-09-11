package paste

import (
	"log"
	"os"
	"os/exec"
	"time"
)

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
		// Try ydotool
		if _, err := exec.LookPath("ydotool"); err == nil {
			// key 29: Ctrl, 47: v
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
