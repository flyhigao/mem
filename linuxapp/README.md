# Mem Linux Client

Mem 的 Linux 桌面客户端，支持一键拉取最新中转文本、自动写入系统剪贴板并模拟粘贴。

## 功能特性
- **一键拉取 & 自动粘贴**：从 Web 中转服务器拉取最新文本，存入剪贴板，并自动模拟 `Ctrl+V` 粘贴到当前活动输入框。
- **双向支持**：不仅支持 pull（获取），也支持命令行 push（发送）。
- **兼容 X11 & Wayland**：自动检测显示协议，支持 `xclip` / `xdotool` / `wl-clipboard` / `ydotool` / `wtype`。
- **桌面通知**：通过 `notify-send` 显示获取预览。
- **系统原生快捷键集成**：与 GNOME、KDE、XFCE、i3、Sway、Hyprland 完美结合。

## 安装与编译

```bash
cd linuxapp
go build -o mem-client main.go
sudo cp mem-client /usr/local/bin/
```

依赖项安装（推荐）：
- Ubuntu/Debian: `sudo apt install xclip xdotool libnotify-bin` (X11) 或 `sudo apt install wl-clipboard ydotool libnotify-bin` (Wayland)
- Arch Linux: `sudo pacman -S xclip xdotool libnotify` (X11) 或 `wl-clipboard ydotool` (Wayland)

## 配置

创建配置文件 `~/.config/mem/config.json`：
```json
{
  "server_url": "http://192.168.2.97:8080",
  "token": "你的API_TOKEN",
  "auto_paste": true,
  "notify": true
}
```

## 使用方法

### 1. 命令行测试
```bash
# 拉取最新并自动粘贴
mem-client pull

# 仅拉取到剪贴板，不自动粘贴
mem-client -no-paste

# 发送内容到中转服务器
mem-client push "来自 Linux 终端的一段文字"
```

### 2. 绑定系统全局快捷键（推荐）
在你的桌面设置中添加自定义快捷键：
- **名称**：Mem 跨端粘贴
- **命令**：`/usr/local/bin/mem-client`
- **快捷键**：例如 `Ctrl + Alt + V` 或 `Super + V`

这样在任何可输入的地方按下该快捷键，就会瞬间从云端获取最新手机/网页端内容并自动粘贴！
