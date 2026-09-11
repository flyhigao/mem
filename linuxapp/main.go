package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"

	"mem/linuxapp/client"
	"mem/linuxapp/clipboard"
	"mem/linuxapp/paste"
)

func loadConfig(configPath string) client.Config {
	cfg := client.Config{
		ServerURL: "http://192.168.2.97:8080",
		Token:     "",
		Hotkey:    "ctrl+alt+v",
		AutoPaste: true,
		Notify:    true,
	}

	searchPaths := []string{}
	if configPath != "" {
		searchPaths = append(searchPaths, configPath)
	}
	searchPaths = append(searchPaths, "config.json")

	if usr, err := user.Current(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(usr.HomeDir, ".config", "mem", "config.json"))
	}
	searchPaths = append(searchPaths, "/etc/mem/config.json")

	for _, p := range searchPaths {
		if data, err := os.ReadFile(p); err == nil {
			if err := json.Unmarshal(data, &cfg); err == nil {
				log.Printf("Loaded config from %s", p)
				break
			}
		}
	}

	return cfg
}

func doPull(cfg client.Config) error {
	c := client.NewMemClient(cfg)
	msg, err := c.FetchLatestMessage()
	if err != nil {
		return fmt.Errorf("fetch error: %w", err)
	}

	// 1. Write to clipboard
	if err := clipboard.SetText(msg.Content); err != nil {
		log.Printf("⚠️ Clipboard warning: %v", err)
	} else {
		log.Printf("📋 Copied to clipboard (%d chars)", len(msg.Content))
	}

	// 2. Desktop notification
	if cfg.Notify {
		preview := msg.Content
		if len(preview) > 60 {
			preview = preview[:60] + "..."
		}
		client.SendNotification("Mem 跨端中转", fmt.Sprintf("已获取最新文本:\n%s", preview))
	}

	// 3. Auto paste if enabled
	if cfg.AutoPaste {
		if err := paste.SimulatePaste(); err != nil {
			log.Printf("⚠️ Auto-paste warning: %v", err)
		} else {
			log.Printf("🚀 Auto-pasted to active window")
		}
	}

	return nil
}

func main() {
	configFlag := flag.String("config", "", "Path to config.json")
	urlFlag := flag.String("url", "", "Server URL (overrides config)")
	tokenFlag := flag.String("token", "", "API Token (overrides config)")
	noPasteFlag := flag.Bool("no-paste", false, "Disable auto-paste (clipboard only)")
	noNotifyFlag := flag.Bool("no-notify", false, "Disable desktop notification")
	pushTextFlag := flag.String("push", "", "Push text to server instead of pulling")
	flag.Parse()

	cfg := loadConfig(*configFlag)
	if *urlFlag != "" {
		cfg.ServerURL = *urlFlag
	}
	if *tokenFlag != "" {
		cfg.Token = *tokenFlag
	}
	if *noPasteFlag {
		cfg.AutoPaste = false
	}
	if *noNotifyFlag {
		cfg.Notify = false
	}

	// Check for subcommand
	args := flag.Args()
	subcmd := "pull"
	if len(args) > 0 {
		subcmd = args[0]
	}

	if *pushTextFlag != "" || subcmd == "push" {
		text := *pushTextFlag
		if text == "" && len(args) > 1 {
			text = args[1]
		}
		if text == "" {
			log.Fatalf("❌ Error: No text provided to push")
		}
		c := client.NewMemClient(cfg)
		msg, err := c.PostMessage(text, "Linux")
		if err != nil {
			log.Fatalf("❌ Push failed: %v", err)
		}
		log.Printf("✅ Sent successfully (ID: %d)", msg.ID)
		return
	}

	// Pull mode (default)
	if cfg.Token == "" {
		log.Fatalf("❌ Error: API Token is empty. Please configure token in config.json or use -token <token>")
	}

	if err := doPull(cfg); err != nil {
		log.Fatalf("❌ %v", err)
	}
}
