package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mem/linuxapp/client"
	"mem/linuxapp/clipboard"
	"mem/linuxapp/paste"
)

const AppVersion = "v1.0.0"

type ConfigWithMeta struct {
	client.Config
	LoadedFrom string
}

func getDefaultConfigPath() string {
	if usr, err := user.Current(); err == nil {
		return filepath.Join(usr.HomeDir, ".config", "mem", "config.json")
	}
	return "config.json"
}

func loadConfig(configPath string) ConfigWithMeta {
	cfg := client.Config{
		ServerURL:    "https://mem.codet.net:8444",
		Token:        "",
		Source:       "Linux",
		Hotkey:       "ctrl+alt+v",
		AutoPaste:    true,
		Notify:       true,
		PollInterval: 3,
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

	loadedFrom := ""
	for _, p := range searchPaths {
		if data, err := os.ReadFile(p); err == nil {
			if err := json.Unmarshal(data, &cfg); err == nil {
				loadedFrom = p
				break
			}
		}
	}

	return ConfigWithMeta{
		Config:     cfg,
		LoadedFrom: loadedFrom,
	}
}

func saveConfig(path string, cfg client.Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(path, append(data, '\n'), 0600)
}

func maskToken(t string) string {
	if len(t) <= 8 {
		return "******"
	}
	return t[:8] + "..." + t[len(t)-4:]
}

func doPull(cfg client.Config, rawOutput bool) error {
	c := client.NewMemClient(cfg)
	msg, err := c.FetchLatestMessage()
	if err != nil {
		return fmt.Errorf("fetch error: %w", err)
	}

	if rawOutput {
		fmt.Print(msg.Content)
		return nil
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
		sourceLabel := msg.Source
		if sourceLabel == "" {
			sourceLabel = "云端"
		}
		client.SendNotification("Mem 跨端中转", fmt.Sprintf("已从 [%s] 获取最新文本:\n%s", sourceLabel, preview))
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

func doPush(cfg client.Config, text string, source string, forceClipboard bool) error {
	// If text is not explicitly given and not forced clipboard, check stdin
	if text == "" && !forceClipboard {
		stat, err := os.Stdin.Stat()
		// If stdin is a pipe or redirected file, read from it
		if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
			buf, err := io.ReadAll(os.Stdin)
			if err == nil && len(buf) > 0 {
				text = string(buf)
			}
		}
	}

	// If still empty, read from system clipboard!
	if text == "" || forceClipboard {
		clipText, err := clipboard.GetText()
		if err != nil {
			return fmt.Errorf("no text provided and failed to read clipboard: %w", err)
		}
		text = clipText
	}

	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return fmt.Errorf("cannot push empty text")
	}

	if source == "" {
		source = cfg.GetSource()
	}

	c := client.NewMemClient(cfg)
	msg, err := c.PostMessage(text, source)
	if err != nil {
		return fmt.Errorf("push failed: %w", err)
	}

	log.Printf("✅ Sent successfully (ID: %d, %d chars, source: %s)", msg.ID, len(text), source)

	if cfg.Notify {
		preview := text
		if len(preview) > 60 {
			preview = preview[:60] + "..."
		}
		client.SendNotification("Mem 跨端发送", fmt.Sprintf("已成功发送至云端 (ID: %d):\n%s", msg.ID, preview))
	}

	return nil
}

func doList(cfg client.Config, limit int, asJSON bool) error {
	if limit <= 0 {
		limit = 10
	}
	c := client.NewMemClient(cfg)
	msgs, err := c.FetchMessages(limit, 0, 0)
	if err != nil {
		return fmt.Errorf("fetch messages failed: %w", err)
	}

	if asJSON {
		data, _ := json.MarshalIndent(msgs, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if len(msgs) == 0 {
		fmt.Println("No messages found in history.")
		return nil
	}

	fmt.Printf("%-6s %-12s %-20s %s\n", "ID", "SOURCE", "TIME", "CONTENT")
	fmt.Println(strings.Repeat("-", 75))
	for _, m := range msgs {
		preview := strings.ReplaceAll(m.Content, "\n", " ↵ ")
		if len(preview) > 48 {
			preview = preview[:48] + "..."
		}
		timeStr := m.CreatedAt.Local().Format("2006-01-02 15:04:05")
		fmt.Printf("%-6d %-12s %-20s %s\n", m.ID, m.Source, timeStr, preview)
	}
	return nil
}

func doShow(cfg client.Config, id int64, copyToClipboard bool, autoPaste bool) error {
	c := client.NewMemClient(cfg)
	msgs, err := c.FetchMessages(100, 0, 0)
	if err != nil {
		return fmt.Errorf("fetch history failed: %w", err)
	}

	var target *client.Message
	for _, m := range msgs {
		if m.ID == id {
			target = &m
			break
		}
	}

	if target == nil {
		return fmt.Errorf("message with ID %d not found in recent history", id)
	}

	fmt.Println(target.Content)

	if copyToClipboard {
		if err := clipboard.SetText(target.Content); err != nil {
			log.Printf("⚠️ Clipboard warning: %v", err)
		} else {
			log.Printf("📋 Copied message #%d to clipboard", id)
		}
	}

	if autoPaste {
		if err := paste.SimulatePaste(); err != nil {
			log.Printf("⚠️ Auto-paste warning: %v", err)
		} else {
			log.Printf("🚀 Auto-pasted message #%d", id)
		}
	}

	return nil
}

func doDelete(cfg client.Config, id int64) error {
	c := client.NewMemClient(cfg)
	if err := c.DeleteMessage(id); err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}
	log.Printf("✅ Message #%d deleted successfully", id)
	return nil
}

func doStatus(cfg client.Config, loadedPath string) error {
	fmt.Printf("🔍 Checking Mem Server Connection...\n")
	if loadedPath != "" {
		fmt.Printf("   Config file: %s\n", loadedPath)
	} else {
		fmt.Printf("   Config file: [None found, using defaults]\n")
	}
	fmt.Printf("   Server URL:  %s\n", cfg.ServerURL)
	fmt.Printf("   API Token:   %s\n", maskToken(cfg.Token))
	fmt.Printf("   Device Name: %s\n", cfg.GetSource())
	fmt.Printf("   Auto-Paste:  %v\n", cfg.AutoPaste)
	fmt.Printf("   Notify:      %v\n", cfg.Notify)

	if cfg.Token == "" {
		fmt.Printf("\n⚠️ Warning: Token is empty. Run 'mem-client config set token <TOKEN>' to configure.\n")
		return nil
	}

	c := client.NewMemClient(cfg)
	latency, err := c.Ping()
	if err != nil {
		fmt.Printf("\n❌ Server Ping Failed: %v\n", err)
		return err
	}
	fmt.Printf("\n✅ Server Reachable (round-trip latency: %v)\n", latency.Round(time.Millisecond))

	msg, err := c.FetchLatestMessage()
	if err != nil {
		if strings.Contains(err.Error(), "no messages available") {
			fmt.Printf("✅ Token Authenticated Successfully (History is currently empty)\n")
		} else {
			fmt.Printf("❌ Token Auth Failed: %v\n", err)
			return err
		}
	} else {
		fmt.Printf("✅ Token Authenticated Successfully!\n")
		fmt.Printf("   Latest Message ID: #%d (from [%s] at %s)\n",
			msg.ID, msg.Source, msg.CreatedAt.Local().Format("2006-01-02 15:04:05"))
		preview := strings.ReplaceAll(msg.Content, "\n", " ↵ ")
		if len(preview) > 50 {
			preview = preview[:50] + "..."
		}
		fmt.Printf("   Preview: %s\n", preview)
	}

	// Check local clipboard tool
	fmt.Printf("\n📋 Local Desktop Environment:\n")
	isWayland := os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("XDG_SESSION_TYPE") == "wayland"
	if isWayland {
		fmt.Printf("   Display Protocol: Wayland (%s)\n", os.Getenv("WAYLAND_DISPLAY"))
	} else {
		disp := os.Getenv("DISPLAY")
		if disp == "" {
			disp = ":0 (auto-fallback)"
		}
		fmt.Printf("   Display Protocol: X11 (DISPLAY=%s)\n", disp)
	}

	clipText, err := clipboard.GetText()
	if err == nil {
		clipPreview := strings.ReplaceAll(clipText, "\n", " ↵ ")
		if len(clipPreview) > 40 {
			clipPreview = clipPreview[:40] + "..."
		}
		fmt.Printf("   Clipboard Read:   OK (%d chars: %s)\n", len(clipText), clipPreview)
	} else {
		fmt.Printf("   Clipboard Read:   %v\n", err)
	}

	return nil
}

func doDaemon(cfg client.Config, interval time.Duration, noCopy bool, noNotify bool) error {
	c := client.NewMemClient(cfg)
	if interval <= 0 {
		interval = cfg.GetPollInterval()
	}

	log.Printf("📡 Mem Daemon started. Polling %s every %v", cfg.ServerURL, interval)
	log.Printf("   Device Source: [%s] | Auto-Copy: %v | Notification: %v",
		cfg.GetSource(), !noCopy, !noNotify && cfg.Notify)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Fetch baseline latest message ID
	var lastSeenID int64 = 0
	if latest, err := c.FetchLatestMessage(); err == nil && latest != nil {
		lastSeenID = latest.ID
		log.Printf("   Baseline latest message ID: #%d", lastSeenID)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			log.Println("🛑 Mem Daemon gracefully stopped.")
			return nil
		case <-ticker.C:
			latest, err := c.FetchLatestMessage()
			if err != nil {
				continue
			}
			if latest == nil || latest.ID <= lastSeenID {
				continue
			}

			lastSeenID = latest.ID

			// Skip messages published by this same device
			if latest.Source == cfg.GetSource() {
				continue
			}

			log.Printf("📥 New message #%d received from [%s] (%d chars)",
				latest.ID, latest.Source, len(latest.Content))

			if !noCopy {
				if err := clipboard.SetText(latest.Content); err != nil {
					log.Printf("⚠️ Failed to write clipboard: %v", err)
				} else {
					log.Printf("📋 Auto-copied to clipboard")
				}
			}

			if !noNotify && cfg.Notify {
				preview := latest.Content
				if len(preview) > 60 {
					preview = preview[:60] + "..."
				}
				client.SendNotification("Mem 跨端中转",
					fmt.Sprintf("收到来自 [%s] 的文本:\n%s", latest.Source, preview))
			}
		}
	}
}

func doConfigCmd(cfgMeta ConfigWithMeta, args []string) error {
	sub := "show"
	if len(args) > 0 {
		sub = args[0]
	}

	targetPath := cfgMeta.LoadedFrom
	if targetPath == "" {
		targetPath = getDefaultConfigPath()
	}

	switch sub {
	case "show":
		fmt.Printf("Active Configuration:\n")
		if cfgMeta.LoadedFrom != "" {
			fmt.Printf("  Loaded from: %s\n", cfgMeta.LoadedFrom)
		} else {
			fmt.Printf("  Loaded from: [Defaults, file not yet created at %s]\n", targetPath)
		}
		fmt.Printf("  server_url:    %s\n", cfgMeta.ServerURL)
		fmt.Printf("  token:         %s\n", maskToken(cfgMeta.Token))
		fmt.Printf("  source:        %s\n", cfgMeta.GetSource())
		fmt.Printf("  hotkey:        %s\n", cfgMeta.Hotkey)
		fmt.Printf("  auto_paste:    %v\n", cfgMeta.AutoPaste)
		fmt.Printf("  notify:        %v\n", cfgMeta.Notify)
		fmt.Printf("  poll_interval: %d\n", cfgMeta.PollInterval)
		return nil

	case "init":
		if _, err := os.Stat(targetPath); err == nil {
			fmt.Printf("Config file already exists at: %s\n", targetPath)
			return nil
		}
		if err := saveConfig(targetPath, cfgMeta.Config); err != nil {
			return err
		}
		fmt.Printf("✅ Initialized default config file at: %s\n", targetPath)
		return nil

	case "set":
		if len(args) < 3 {
			return fmt.Errorf("usage: mem-client config set <key> <value>\nKeys: server_url, token, source, auto_paste, notify, poll_interval")
		}
		key := strings.ToLower(args[1])
		val := args[2]

		newCfg := cfgMeta.Config
		switch key {
		case "server_url", "url":
			newCfg.ServerURL = val
		case "token":
			newCfg.Token = val
		case "source", "device_name":
			newCfg.Source = val
		case "hotkey":
			newCfg.Hotkey = val
		case "auto_paste", "paste":
			newCfg.AutoPaste = (val == "true" || val == "1" || val == "yes")
		case "notify":
			newCfg.Notify = (val == "true" || val == "1" || val == "yes")
		case "poll_interval", "interval":
			if iv, err := strconv.Atoi(val); err == nil && iv > 0 {
				newCfg.PollInterval = iv
			} else {
				return fmt.Errorf("invalid poll_interval number: %s", val)
			}
		default:
			return fmt.Errorf("unknown config key %q", key)
		}

		if err := saveConfig(targetPath, newCfg); err != nil {
			return err
		}
		fmt.Printf("✅ Saved %s = %s to %s\n", key, val, targetPath)
		return nil

	default:
		return fmt.Errorf("unknown config subcommand %q (use 'show', 'init', or 'set')", sub)
	}
}

func printHelp() {
	helpText := fmt.Sprintf(`Mem Linux Client (%s) - 跨端剪贴板/文本中转站

用法:
  mem-client [flags] [command] [arguments...]

基础命令:
  pull                     拉取最新文本写入系统剪贴板并模拟粘贴（默认动作）
  push [text]              推送文本至中转站（支持参数、管道标准输入、或直接读取剪贴板）
  list, history            查看最近历史文本列表
  show <id>                查看指定 ID 的完整消息文本（支持 -c 复制，-p 粘贴）
  delete, del <id>         从服务器删除指定 ID 的消息
  status, check, ping      测试与服务器的连通性及 Token 认证有效性
  daemon, watch            前台/后台守护模式，实时同步其他设备推送的文本并提醒
  config [show|init|set]   查看、初始化或修改客户端配置文件 (~/.config/mem/config.json)
  version                  显示版本信息
  help                     显示本帮助信息

常用示例:
  # 一键拉取最新并自动粘贴 (适合绑定全局快捷键 Super+V 或 Ctrl+Alt+V)
  mem-client

  # 仅拉取到剪贴板，不自动模拟粘贴
  mem-client pull --no-paste

  # 一键推送当前系统剪贴板内容到手机 (适合绑定全局快捷键 Ctrl+Alt+C)
  mem-client push

  # 发送指定文字或从终端管道发送
  mem-client push "来自 Linux 终端的一段文字"
  cat id_rsa.pub | mem-client push

  # 查看服务器最近 10 条消息
  mem-client list

  # 查看指定 ID 并写入剪贴板
  mem-client show 5 -c

  # 服务健康状态检查
  mem-client status

  # 启动后台守护监听（收到手机新消息自动同步到剪贴板并弹窗提醒）
  mem-client daemon

全局参数 (Flags):
  -config <path>           指定自定义配置文件路径
  -url <url>               覆盖服务器地址 (如 https://mem.codet.net:8444)
  -token <token>           覆盖 API Token
  -source <name>           指定设备来源标签 (默认: Linux)
  -no-paste                禁用自动模拟粘贴
  -no-notify               禁用桌面弹窗通知
  -raw                     (用于 pull) 仅输出原始内容到标准输出，无任何额外日志
  -c, -clipboard           (用于 push/show) 从系统剪贴板读取 / 复制到剪贴板
  -limit, -n <n>           (用于 list) 指定拉取消息数量 (默认: 10)
  -json                    (用于 list) 以 JSON 格式输出
  -i, -interval <duration> (用于 daemon) 轮询间隔 (例如 3s)
`, AppVersion)
	fmt.Print(helpText)
}

func main() {
	rawArgs := os.Args[1:]

	// Separate global options and subcommand
	// Known subcommands:
	subcommands := map[string]bool{
		"pull":    true,
		"push":    true,
		"list":    true,
		"history": true,
		"show":    true,
		"get":     true,
		"delete":  true,
		"del":     true,
		"rm":      true,
		"status":  true,
		"check":   true,
		"ping":    true,
		"daemon":  true,
		"watch":   true,
		"config":  true,
		"version": true,
		"help":    true,
	}

	var subcmd string
	var globalArgs []string
	var cmdArgs []string

	// Find the first subcommand in rawArgs
	subcmdIdx := -1
	for i, arg := range rawArgs {
		if !strings.HasPrefix(arg, "-") && subcommands[arg] {
			subcmd = arg
			subcmdIdx = i
			break
		}
	}

	if subcmdIdx != -1 {
		globalArgs = rawArgs[:subcmdIdx]
		cmdArgs = rawArgs[subcmdIdx+1:]
	} else {
		// No subcommand found, all args are flags or default to pull
		globalArgs = rawArgs
		subcmd = "pull"
	}

	// Global flagset
	gfs := flag.NewFlagSet("mem-client", flag.ContinueOnError)
	gfs.SetOutput(io.Discard)

	configFlag := gfs.String("config", "", "Path to config.json")
	urlFlag := gfs.String("url", "", "Server URL")
	tokenFlag := gfs.String("token", "", "API Token")
	sourceFlag := gfs.String("source", "", "Device source name")
	noPasteFlag := gfs.Bool("no-paste", false, "Disable auto-paste")
	noNotifyFlag := gfs.Bool("no-notify", false, "Disable desktop notification")
	pushTextFlag := gfs.String("push", "", "Push text")
	rawFlag := gfs.Bool("raw", false, "Output raw text only")
	versionFlag := gfs.Bool("version", false, "Print version")
	gfs.BoolVar(versionFlag, "v", false, "Print version")
	helpFlag := gfs.Bool("help", false, "Print help")
	gfs.BoolVar(helpFlag, "h", false, "Print help")

	_ = gfs.Parse(globalArgs)

	if *versionFlag || subcmd == "version" {
		fmt.Printf("mem-client %s (linux/amd64)\n", AppVersion)
		return
	}
	if *helpFlag || subcmd == "help" {
		printHelp()
		return
	}

	cfgMeta := loadConfig(*configFlag)
	cfg := cfgMeta.Config

	if *urlFlag != "" {
		cfg.ServerURL = *urlFlag
	}
	if *tokenFlag != "" {
		cfg.Token = *tokenFlag
	}
	if *sourceFlag != "" {
		cfg.Source = *sourceFlag
	}
	if *noPasteFlag {
		cfg.AutoPaste = false
	}
	if *noNotifyFlag {
		cfg.Notify = false
	}

	// Handle legacy flag: -push "some text"
	if *pushTextFlag != "" {
		subcmd = "push"
		cmdArgs = append([]string{*pushTextFlag}, cmdArgs...)
	}

	// Now dispatch according to subcommand and parse any subcommand-specific flags
	switch subcmd {
	case "pull":
		pullFs := flag.NewFlagSet("pull", flag.ContinueOnError)
		pullFs.SetOutput(io.Discard)
		pullNoPaste := pullFs.Bool("no-paste", !cfg.AutoPaste, "")
		pullCopyOnly := pullFs.Bool("copy-only", false, "")
		pullNoNotify := pullFs.Bool("no-notify", !cfg.Notify, "")
		pullRaw := pullFs.Bool("raw", *rawFlag, "")
		_ = pullFs.Parse(cmdArgs)

		if *pullNoPaste || *pullCopyOnly {
			cfg.AutoPaste = false
		}
		if *pullNoNotify {
			cfg.Notify = false
		}

		if cfg.Token == "" {
			log.Fatalf("❌ Error: API Token is empty. Please run 'mem-client config set token <TOKEN>' or use -token <TOKEN>")
		}
		if err := doPull(cfg, *pullRaw); err != nil {
			log.Fatalf("❌ %v", err)
		}

	case "push":
		pushFs := flag.NewFlagSet("push", flag.ContinueOnError)
		pushFs.SetOutput(io.Discard)
		pushClip := pushFs.Bool("c", false, "")
		pushFs.BoolVar(pushClip, "clipboard", false, "")
		pushSource := pushFs.String("s", cfg.GetSource(), "")
		pushFs.StringVar(pushSource, "source", cfg.GetSource(), "")
		pushNoNotify := pushFs.Bool("no-notify", !cfg.Notify, "")
		_ = pushFs.Parse(cmdArgs)

		if *pushNoNotify {
			cfg.Notify = false
		}

		if cfg.Token == "" {
			log.Fatalf("❌ Error: API Token is empty. Please run 'mem-client config set token <TOKEN>' or use -token <TOKEN>")
		}

		pushText := strings.Join(pushFs.Args(), " ")
		if err := doPush(cfg, pushText, *pushSource, *pushClip); err != nil {
			log.Fatalf("❌ %v", err)
		}

	case "list", "history":
		listFs := flag.NewFlagSet("list", flag.ContinueOnError)
		listFs.SetOutput(io.Discard)
		listLimit := listFs.Int("limit", 10, "")
		listFs.IntVar(listLimit, "n", 10, "")
		listJSON := listFs.Bool("json", false, "")
		_ = listFs.Parse(cmdArgs)

		if cfg.Token == "" {
			log.Fatalf("❌ Error: API Token is empty. Please run 'mem-client config set token <TOKEN>' or use -token <TOKEN>")
		}
		if err := doList(cfg, *listLimit, *listJSON); err != nil {
			log.Fatalf("❌ %v", err)
		}

	case "show", "get":
		showFs := flag.NewFlagSet("show", flag.ContinueOnError)
		showFs.SetOutput(io.Discard)
		showCopy := showFs.Bool("c", false, "")
		showFs.BoolVar(showCopy, "copy", false, "")
		showPaste := showFs.Bool("p", false, "")
		showFs.BoolVar(showPaste, "paste", false, "")
		_ = showFs.Parse(cmdArgs)

		posArgs := showFs.Args()
		if len(posArgs) == 0 {
			log.Fatalf("❌ Error: Please specify message ID, e.g. 'mem-client show 5'")
		}
		id, err := strconv.ParseInt(posArgs[0], 10, 64)
		if err != nil {
			log.Fatalf("❌ Error: Invalid message ID: %s", posArgs[0])
		}
		if err := doShow(cfg, id, *showCopy, *showPaste); err != nil {
			log.Fatalf("❌ %v", err)
		}

	case "delete", "del", "rm":
		delFs := flag.NewFlagSet("delete", flag.ContinueOnError)
		delFs.SetOutput(io.Discard)
		_ = delFs.Parse(cmdArgs)

		posArgs := delFs.Args()
		if len(posArgs) == 0 {
			log.Fatalf("❌ Error: Please specify message ID, e.g. 'mem-client delete 5'")
		}
		id, err := strconv.ParseInt(posArgs[0], 10, 64)
		if err != nil {
			log.Fatalf("❌ Error: Invalid message ID: %s", posArgs[0])
		}
		if err := doDelete(cfg, id); err != nil {
			log.Fatalf("❌ %v", err)
		}

	case "status", "check", "ping":
		if err := doStatus(cfg, cfgMeta.LoadedFrom); err != nil {
			os.Exit(1)
		}

	case "daemon", "watch":
		daemonFs := flag.NewFlagSet("daemon", flag.ContinueOnError)
		daemonFs.SetOutput(io.Discard)
		daemonInterval := daemonFs.Duration("interval", 0, "")
		daemonFs.DurationVar(daemonInterval, "i", 0, "")
		daemonNoCopy := daemonFs.Bool("no-copy", false, "")
		daemonNoNotify := daemonFs.Bool("no-notify", !cfg.Notify, "")
		_ = daemonFs.Parse(cmdArgs)

		if cfg.Token == "" {
			log.Fatalf("❌ Error: API Token is empty. Please run 'mem-client config set token <TOKEN>' or use -token <TOKEN>")
		}
		if err := doDaemon(cfg, *daemonInterval, *daemonNoCopy, *daemonNoNotify); err != nil {
			log.Fatalf("❌ %v", err)
		}

	case "config":
		cfgMeta.Config = cfg
		if err := doConfigCmd(cfgMeta, cmdArgs); err != nil {
			log.Fatalf("❌ %v", err)
		}

	default:
		fmt.Fprintf(os.Stderr, "❌ Unknown command %q\nRun 'mem-client help' for usage.\n", subcmd)
		os.Exit(1)
	}
}
