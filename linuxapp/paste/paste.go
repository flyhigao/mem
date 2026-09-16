package paste

import (
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// isPureTerminalWindow checks if the currently active window is a dedicated standalone terminal emulator
func isPureTerminalWindow() bool {
	winIDCmd := exec.Command("xdotool", "getactivewindow")
	winIDBytes, err := winIDCmd.Output()
	if err == nil {
		winID := strings.TrimSpace(string(winIDBytes))
		if winID != "" {
			propCmd := exec.Command("xprop", "-id", winID, "WM_CLASS")
			propBytes, err := propCmd.Output()
			if err == nil {
				classStr := strings.ToLower(string(propBytes))
				// Dedicated standalone terminal emulators that require Ctrl+Shift+V instead of Ctrl+V
				pureTerminals := []string{
					"wezterm",
					"deepin-terminal",
					"gnome-terminal",
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
					"st",
					"sakura",
					"lxterminal",
					"xfce4-terminal",
					"mate-terminal",
				}
				for _, t := range pureTerminals {
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
	time.Sleep(100 * time.Millisecond)

	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		os.Setenv("DISPLAY", ":0")
	}

	isWayland := os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland"

	if isWayland {
		if _, err := exec.LookPath("wtype"); err == nil {
			cmd := exec.Command("wtype", "-M", "ctrl", "-k", "c", "-m", "ctrl")
			if err := cmd.Run(); err == nil {
				time.Sleep(100 * time.Millisecond)
				return nil
			}
		}
		if _, err := exec.LookPath("ydotool"); err == nil {
			cmd := exec.Command("ydotool", "key", "29:1", "46:1", "46:0", "29:0")
			if err := cmd.Run(); err == nil {
				time.Sleep(100 * time.Millisecond)
				return nil
			}
		}
	}

	// Try xdotool (standard X11 tool)
	if _, err := exec.LookPath("xdotool"); err == nil {
		cmd := exec.Command("xdotool", "key", "--clearmodifiers", "ctrl+c")
		if err := cmd.Run(); err == nil {
			time.Sleep(100 * time.Millisecond)
			return nil
		}
	}

	return nil
}

// SimulatePaste sends appropriate paste shortcut to active window
func SimulatePaste() error {
	time.Sleep(100 * time.Millisecond)

	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		os.Setenv("DISPLAY", ":0")
	}

	isWayland := os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland"
	isTerm := isPureTerminalWindow()

	if isWayland {
		if isTerm {
			// Standalone terminal paste: Ctrl+Shift+V
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
			// Standard GUI & IDE paste (Firefox, Chrome, Antigravity, VSCode, etc.): Ctrl+V
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
			// Standalone terminal emulators (WezTerm, Deepin Terminal, GNOME Terminal, etc.): send Ctrl+Shift+V
			cmd := exec.Command("xdotool", "key", "--clearmodifiers", "--delay", "20", "ctrl+shift+v")
			if err := cmd.Run(); err == nil {
				return nil
			}
			cmdShift := exec.Command("xdotool", "key", "--clearmodifiers", "--delay", "20", "shift+Insert")
			if err := cmdShift.Run(); err == nil {
				return nil
			}
		} else {
			// Standard GUI & IDE apps (Firefox, Chrome, Antigravity, VSCode, WizNote, etc.): send Ctrl+V
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
