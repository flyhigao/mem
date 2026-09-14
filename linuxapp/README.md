# Mem Linux Client

Mem 的 Linux 桌面端客户端，采用纯 Go 开发（零 CGO 动态库依赖），支持一键跨端拉取中转文本、自动写入系统剪贴板、模拟粘贴以及剪贴板反向推送。

---

## 一、功能特性

- **一键拉取 & 自动粘贴 (Pull)**：从 Web 中转服务器拉取最新文本，存入系统剪贴板，并自动模拟 `Ctrl+V` 键粘贴到当前活动输入框。
- **全方位反向推送 (Push)**：
  - **剪贴板一键推送**：不带参数执行 `mem-client push`，自动读取系统剪贴板内容并推送到云端/手机。
  - **命令行直接发送**：`mem-client push "内容"`。
  - **终端管道输入**：`cat file.txt | mem-client push`。
- **历史记录浏览与管理 (List & Show & Delete)**：支持查看历史记录、查看指定单条内容、删除单条记录。
- **后台守护模式 (Daemon / Watch)**：常驻后台监听云端新消息，手机发出的新内容秒级自动同步到 Linux 剪贴板并桌面弹窗提醒。
- **双协议兼容 (X11 & Wayland)**：
  - **Wayland**：优先支持 `wl-copy` / `wl-paste`，模拟粘贴支持 `wtype` / `ydotool`。
  - **X11**：支持 `xclip` / `xsel`，模拟粘贴支持 `xdotool`；在未安装额外 CLI 工具的发行版（如 Deepin/Ubuntu）上，自动回退至系统内置 `python3-gi (GTK)` / `tkinter` 剪贴板引擎。
- **桌面通知提醒**：通过 `notify-send` 弹出内容预览卡片。

---

## 二、安装与编译

### 1. 从源码编译
```bash
cd linuxapp
go build -o mem-client main.go
sudo cp mem-client /usr/local/bin/
```

### 2. 依赖项推荐（可选）
Mem 客户端内置对常见环境的自适应探测与 Python GTK/Tkinter 回退机制。如需最佳体验与速度，推荐安装：
- **Ubuntu / Debian / Deepin**：
  - X11: `sudo apt install xdotool libnotify-bin` (如需独立剪贴板工具可装 `xclip`)
  - Wayland: `sudo apt install wl-clipboard ydotool libnotify-bin`
- **Arch Linux**：
  - X11: `sudo pacman -S xclip xdotool libnotify`
  - Wayland: `sudo pacman -S wl-clipboard ydotool libnotify`

---

## 三、配置说明

配置文件路径：`~/.config/mem/config.json`（亦支持当前目录 `config.json` 或 `/etc/mem/config.json`）。

可通过命令行快速初始化与配置：
```bash
# 查看当前加载的配置
mem-client config

# 设置服务器地址
mem-client config set server_url https://mem.codet.net:8444

# 设置 API Token
mem-client config set token 你的API_TOKEN
```

配置文件完整字段说明：
```json
{
  "server_url": "https://mem.codet.net:8444",
  "token": "mem_aa3a3f33353e2f6af8eb0a02f37d07b31106764634c937d3",
  "source": "Linux",
  "hotkey": "ctrl+alt+v",
  "auto_paste": true,
  "notify": true,
  "poll_interval": 3
}
```

| 字段 | 类型 | 说明 |
| :--- | :--- | :--- |
| `server_url` | string | Web 中转服务地址（如 `https://mem.codet.net:8444`） |
| `token` | string | API 鉴权 Token |
| `source` | string | 设备来源标签（默认: `Linux`） |
| `auto_paste` | bool | 拉取后是否自动模拟 `Ctrl+V` 粘贴到活动窗口 |
| `notify` | bool | 是否通过桌面通知 (`notify-send`) 弹出预览提醒 |
| `poll_interval` | int | 守护进程模式 (`daemon`) 下轮询同步间隔（秒） |

---

## 四、使用方法

### 1. 基础命令

#### ① 拉取最新并粘贴 (Pull)
```bash
# 默认动作：拉取最新文本 -> 存入剪贴板 -> 模拟 Ctrl+V 自动粘贴
mem-client

# 显式执行 pull
mem-client pull

# 仅存入剪贴板，不自动粘贴
mem-client pull --no-paste

# 仅输出原始文本内容（无额外日志，适合 Shell 脚本组合）
mem-client pull -raw
```

#### ② 推送文本到云端 (Push)
```bash
# 方式 A：将当前 Linux 剪贴板内容一键推送至云端/手机
mem-client push

# 方式 B：命令行直接发送文字
mem-client push "来自 Linux 终端的一段文字"

# 方式 C：从终端管道或文件重定向发送
cat ~/.ssh/id_rsa.pub | mem-client push
git diff | mem-client push
```

#### ③ 历史记录查看与管理
```bash
# 查看最近 10 条历史记录
mem-client list

# 查看指定数量（例如 20 条）或输出 JSON
mem-client list -n 20
mem-client list -json

# 查看指定 ID 消息的完整文本（支持 -c 复制到剪贴板）
mem-client show 5 -c

# 删除指定 ID 的消息
mem-client delete 5
```

#### ④ 服务状态与连通性检查
```bash
mem-client status
```
测试与服务器的 Ping 延迟、Token 鉴权有效性以及当前系统的剪贴板环境探测结果。

#### ⑤ 后台守护模式 (Daemon)
```bash
mem-client daemon
```
持续在后台轮询中转服务。当手机或 Web 端有新文本发送时，自动写入 Linux 剪贴板并桌面弹窗提醒，实现类似“双向实时同步”体验。

---

## 五、绑定系统全局快捷键（推荐）

在你的 Linux 桌面设置（如 Deepin、GNOME、KDE、XFCE、Sway、Hyprland 等）的“快捷键设置”中添加两条自定义快捷键：

### 快捷键 1：跨端拉取并粘贴 (手机 -> 电脑)
- **快捷键名称**：`Mem 跨端粘贴`
- **快捷键**：`Super + V` (即 Win + V)
- **命令**：`/usr/local/bin/mem-client`

在任意输入框（聊天软件、终端、浏览器）按下该快捷键，将自动把手机最新发送的文本填入光标所在位置。

### 快捷键 2：跨端推送剪贴板 (电脑 -> 手机)
- **快捷键名称**：`Mem 推送剪贴板`
- **快捷键**：`Super + C` (即 Win + C)
- **命令**：`/usr/local/bin/mem-client push`

在电脑端选中文字按下 `Ctrl+C` 复制后，再按该快捷键即可瞬间把内容推送到云端与手机。

---

## 六、Systemd 用户服务托管（可选常驻后台）

如希望开机自动启动守护模式同步手机文本，可创建 Systemd 用户级服务文件：

创建 `~/.config/systemd/user/mem-client.service`：
```ini
[Unit]
Description=Mem Linux Client Sync Daemon
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mem-client daemon
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
```

启用并启动：
```bash
systemctl --user daemon-reload
systemctl --user enable --now mem-client.service
systemctl --user status mem-client.service
```
