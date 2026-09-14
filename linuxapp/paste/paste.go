package paste

import (
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// isTerminalWindow checks if the currently active window is a terminal emulator
func isTerminalWindow() bool {
	winIDCmd := exec.Command("xdotool", "getactivewindow")
	winIDBytes, err := winIDCmd.Output()
	if err == nil {
		winID := strings.TrimSpace(string(winIDBytes))
		if winID != "" {
			propCmd := exec.Command("xprop", "-id", winID, "WM_CLASS")
			propBytes, err := propCmd.Output()
			if err == nil {
				classStr := strings.ToLower(string(propBytes))
				terminals := []string{
					"wezterm",
					"terminal",
					"konsole",
					"kitty",
					"alacritty",
					"xterm",
					"urxvt",
					"rxvt",
					"tilix",
					"foot",
					"ghostty",
					"qterminal",
					"terminator",
					"st-256color",
				}
				for _, t := range terminals {
					if strings.Contains(classStr, t) {
						return true
					}
				}
			}
		}
	}

	return false
}

// SimulateCopy sends Ctrl+C to the active window to copy currently selected/highlighted text
func SimulateCopy() error {
	time.Sleep(80 * time.Millisecond)

	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		os.Setenv("DISPLAY", ":0")
	}

	isWayland := os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland"

	if isWayland {
		if _, err := exec.LookPath("wtype"); err == nil {
			cmd := exec.Command("wtype", "-M", "ctrl", "-k", "c", "-m", "ctrl")
			if err := cmd.Run(); err == nil {
				time.Sleep(80 * time.Millisecond)
				return nil
			}
		}
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

// SimulatePaste sends appropriate paste shortcut (Ctrl+Shift+V for terminals, Ctrl+V for GUI) to active window
func SimulatePaste() error {
	time.Sleep(80 * time.Millisecond)

	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		os.Setenv("DISPLAY", ":0")
	}

	isWayland := os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland"
	isTerm := isTerminalWindow()

	if isWayland {
		if isTerm {
			if _, err := exec.LookPath("wtype"); err == nil {
				cmd := exec.Command("wtype", "-M", "ctrl", "-M", "shift", "-k", "v", "-m", "shift", "-m", "ctrl")
				if err := cmd.Run(); err == nil {
					return nil
				}
			}
			if _, err := exec.LookPath("ydotool"); err == nil {
				cmd := exec.Command("ydotool", "key", "29:1", "42:1", "47:1", "47:0", "42:0", "29:0")
				if err := cmd.Run(); err == nil {
					return nil
				}
			}
		} else {
			if _, err := exec.LookPath("wtype"); err == nil {
				cmd := exec.Command("wtype", "-M", "ctrl", "-k", "v", "-m", "ctrl")
				if err := cmd.Run(); err == nil {
					return nil
				}
			}
			if _, err := exec.LookPath("ydotool"); err == nil {
				cmd := exec.Command("ydotool", "key", "29:1", "47:1", "47:0", "29:0")
				if err := cmd.Run(); err == nil {
					return nil
				}
			}
		}
	}

	// Try xdotool (standard X11 tool)
	if _, err := exec.LookPath("xdotool"); err == nil {
		if isTerm {
			// In terminal emulators (WezTerm, Deepin Terminal, GNOME Terminal, etc.),
			// send Ctrl+Shift+V to paste from CLIPBOARD
			cmd := exec.Command("xdotool", "key", "--clearmodifiers", "--delay", "20", "ctrl+shift+v")
			if err := cmd.Run(); err == nil {
				return nil
			}
			cmdShift := exec.Command("xdotool", "key", "--clearmodifiers", "--delay", "20", "shift+Insert")
			if err := cmdShift.Run(); err == nil {
				return nil
			}
		} else {
			// Standard GUI apps (Firefox, Chrome, Text Editor, etc.): send Ctrl+V
			cmd := exec.Command("xdotool", "key", "--clearmodifiers", "--delay", "20", "ctrl+v")
			if err := cmd.Run(); err == nil {
				return nil
			}
			cmdShift := exec.Command("xdotool", "key", "--clearmodifiers", "--delay", "20", "shift+Insert")
			if err := cmdShift.Run(); err == nil {
				return nil
			}
		}
	}

	log.Printf("⚠️ Auto-paste note: xdotool/wtype/ydotool not found or failed, text is ready in clipboard for manual paste")
	return nil
}
