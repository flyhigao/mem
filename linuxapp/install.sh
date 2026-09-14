#!/usr/bin/env bash
# ==============================================================================
# Mem Linux Client 一键安装与部署脚本
# 支持：Debian / Ubuntu / Deepin / Arch / Fedora 等主流 Linux 发行版
# ==============================================================================

set -e

COLOR_GREEN="\033[32m"
COLOR_YELLOW="\033[33m"
COLOR_RED="\033[31m"
COLOR_CYAN="\033[36m"
COLOR_RESET="\033[0m"

log_info() {
    echo -e "${COLOR_GREEN}[INFO]${COLOR_RESET} $1"
}

log_warn() {
    echo -e "${COLOR_YELLOW}[WARN]${COLOR_RESET} $1"
}

log_error() {
    echo -e "${COLOR_RED}[ERROR]${COLOR_RESET} $1"
}

log_step() {
    echo -e "\n${COLOR_CYAN}==>${COLOR_RESET} ${COLOR_GREEN}$1${COLOR_RESET}"
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="$HOME/.local/bin"
CONFIG_DIR="$HOME/.config/mem"
SYSTEMD_USER_DIR="$HOME/.config/systemd/user"

# ------------------------------------------------------------------------------
# 卸载模式
# ------------------------------------------------------------------------------
if [ "$1" == "--uninstall" ] || [ "$1" == "uninstall" ]; then
    log_step "正在卸载 Mem Linux Client..."
    
    if command -v systemctl >/dev/null 2>&1 && systemctl --user is-active --quiet mem-client.service 2>/dev/null; then
        log_info "停止后台守护服务..."
        systemctl --user stop mem-client.service || true
    fi
    
    if [ -f "$SYSTEMD_USER_DIR/mem-client.service" ]; then
        log_info "注销开机自启服务..."
        systemctl --user disable mem-client.service 2>/dev/null || true
        rm -f "$SYSTEMD_USER_DIR/mem-client.service"
        systemctl --user daemon-reload 2>/dev/null || true
    fi
    
    log_info "清理二进制与快捷脚本..."
    rm -f "$BIN_DIR/mem-client" "$BIN_DIR/mem-paste.sh" "$BIN_DIR/mem-push.sh"
    
    log_info "卸载完成！配置文件保留在 $CONFIG_DIR（如需彻底删除可手动删除该目录）。"
    exit 0
fi

# ------------------------------------------------------------------------------
# 1. 基础环境与依赖探测
# ------------------------------------------------------------------------------
log_step "1. 检测运行环境与依赖工具"

mkdir -p "$BIN_DIR" "$CONFIG_DIR" "$SYSTEMD_USER_DIR"

if [[ ":$PATH:" != *":$BIN_DIR:"* ]]; then
    log_warn "$BIN_DIR 不在当前 PATH 环境变量中。"
    log_warn "建议在 ~/.bashrc 或 ~/.profile 中追加: export PATH=\"\$HOME/.local/bin:\$PATH\""
fi

if [ -n "$WAYLAND_DISPLAY" ]; then
    log_info "检测到显示环境: Wayland ($WAYLAND_DISPLAY)"
    if ! command -v wl-copy >/dev/null 2>&1; then
        log_warn "未检测到 wl-clipboard，推荐安装: sudo apt install wl-clipboard (或 pacman -S wl-clipboard)"
    fi
else
    log_info "检测到显示环境: X11 (DISPLAY=${DISPLAY:-:0})"
    if ! command -v xdotool >/dev/null 2>&1; then
        log_warn "未检测到 xdotool (自动上屏依赖项)，推荐安装: sudo apt install xdotool"
    else
        log_info "模拟键入工具: xdotool 已安装"
    fi
fi

if command -v notify-send >/dev/null 2>&1; then
    log_info "桌面通知支持: notify-send 已安装"
else
    log_warn "未检测到 notify-send，桌面弹窗将静默跳过，推荐安装: sudo apt install libnotify-bin"
fi

# ------------------------------------------------------------------------------
# 2. 编译或安装二进制
# ------------------------------------------------------------------------------
log_step "2. 部署客户端二进制 (mem-client)"

TARGET_BIN="$BIN_DIR/mem-client"
INSTALLED=false

if [ -f "$SCRIPT_DIR/mem-client" ] && file "$SCRIPT_DIR/mem-client" | grep -q "ELF"; then
    log_info "发现本地已编译的二进制，直接安装..."
    cp -f "$SCRIPT_DIR/mem-client" "$TARGET_BIN"
    chmod +x "$TARGET_BIN"
    INSTALLED=true
fi

if [ "$INSTALLED" = false ]; then
    GO_CMD=""
    if command -v go >/dev/null 2>&1; then
        GO_CMD="go"
    elif [ -x "$HOME/.go/bin/go" ]; then
        GO_CMD="$HOME/.go/bin/go"
    fi

    if [ -n "$GO_CMD" ]; then
        log_info "使用本地 Go 环境 ($GO_CMD) 从源码编译..."
        (cd "$SCRIPT_DIR" && CGO_ENABLED=0 "$GO_CMD" build -ldflags="-s -w" -o "$TARGET_BIN" .)
        chmod +x "$TARGET_BIN"
        INSTALLED=true
    fi
fi

if [ "$INSTALLED" = false ]; then
    ARCH="$(uname -m)"
    if [ "$ARCH" == "x86_64" ]; then
        RELEASE_ARCH="amd64"
    elif [ "$ARCH" == "aarch64" ] || [ "$ARCH" == "arm64" ]; then
        RELEASE_ARCH="arm64"
    else
        RELEASE_ARCH="amd64"
    fi
    
    DOWNLOAD_URL="https://github.com/flyhigao/mem/releases/latest/download/mem-client-linux-$RELEASE_ARCH"
    log_info "正在从 GitHub Releases 获取最新预编译二进制 ($RELEASE_ARCH)..."
    curl -fL -o "$TARGET_BIN" "$DOWNLOAD_URL"
    chmod +x "$TARGET_BIN"
    INSTALLED=true
fi

if [ -x "$TARGET_BIN" ]; then
    log_info "二进制安装成功: $TARGET_BIN"
else
    log_error "二进制安装失败，请检查网络或本地 Go 编译环境！"
    exit 1
fi

# ------------------------------------------------------------------------------
# 3. 部署快捷键执行脚本
# ------------------------------------------------------------------------------
log_step "3. 部署快捷键包装脚本"

printf '#!/usr/bin/env bash\nexport DISPLAY="${DISPLAY:-:0}"\nexport XDG_SESSION_TYPE="${XDG_SESSION_TYPE:-x11}"\nexec "$HOME/.local/bin/mem-client" "$@"\n' > "$BIN_DIR/mem-paste.sh"
chmod +x "$BIN_DIR/mem-paste.sh"
log_info "已创建粘贴上屏脚本: $BIN_DIR/mem-paste.sh"

printf '#!/usr/bin/env bash\nexport DISPLAY="${DISPLAY:-:0}"\nexport XDG_SESSION_TYPE="${XDG_SESSION_TYPE:-x11}"\nexec "$HOME/.local/bin/mem-client" push "$@"\n' > "$BIN_DIR/mem-push.sh"
chmod +x "$BIN_DIR/mem-push.sh"
log_info "已创建剪贴板推送脚本: $BIN_DIR/mem-push.sh"

# ------------------------------------------------------------------------------
# 4. 默认配置文件检查与初始化
# ------------------------------------------------------------------------------
log_step "4. 初始化配置文件 (~/.config/mem/config.json)"

CONFIG_FILE="$CONFIG_DIR/config.json"
if [ ! -f "$CONFIG_FILE" ]; then
    log_info "未检测到现有配置，生成默认配置文件..."
    cat > "$CONFIG_FILE" << 'JSON_EOF'
{
  "server_url": "https://mem.codet.net:8444",
  "token": "YOUR_API_TOKEN_HERE",
  "source": "Linux",
  "hotkey": "super+v",
  "auto_paste": true,
  "notify": true,
  "poll_interval": 2
}
JSON_EOF
    chmod 0600 "$CONFIG_FILE"
    log_info "配置文件已创建: $CONFIG_FILE"
    log_warn "请使用命令配置您的真实 Token: mem-client config set token <你的Token>"
else
    log_info "已存在有效配置文件: $CONFIG_FILE"
fi

# ------------------------------------------------------------------------------
# 5. Deepin DDE 桌面全局快捷键自动注册
# ------------------------------------------------------------------------------
log_step "5. 配置系统全局快捷键 (Super+V 与 Super+C)"

if command -v busctl >/dev/null 2>&1 && busctl --user list 2>/dev/null | grep -q "org.deepin.dde.Keybinding1"; then
    log_info "检测到 Deepin DDE 桌面环境，通过 DBus 自动注册全局快捷键..."
    
    busctl --user call org.deepin.dde.Keybinding1 /org/deepin/dde/Keybinding1 org.deepin.dde.Keybinding1         AddCustomShortcut sss "Mem 跨端粘贴" "$BIN_DIR/mem-paste.sh" "<Super>V" >/dev/null 2>&1 || true
    
    busctl --user call org.deepin.dde.Keybinding1 /org/deepin/dde/Keybinding1 org.deepin.dde.Keybinding1         AddCustomShortcut sss "Mem 跨端推送" "$BIN_DIR/mem-push.sh" "<Super>C" >/dev/null 2>&1 || true
        
    log_info "Deepin 全局快捷键注册完成并已生效："
    log_info "  - Win + V (Super+V): 拉取最新文本并自动上屏"
    log_info "  - Win + C (Super+C): 划选复制并实时推送至手机"
else
    log_info "其他桌面环境请在系统设置 -> 快捷键中添加自定义快捷键："
    log_info "  - Super+V (Win+V): 命令填 $BIN_DIR/mem-paste.sh"
    log_info "  - Super+C (Win+C): 命令填 $BIN_DIR/mem-push.sh"
fi

# ------------------------------------------------------------------------------
# 6. 配置 Systemd 用户守护进程 (开机自启)
# ------------------------------------------------------------------------------
log_step "6. 配置 Systemd 后台常驻守护服务 (实时 WebSocket 流)"

SERVICE_FILE="$SYSTEMD_USER_DIR/mem-client.service"
cat > "$SERVICE_FILE" << SERVICE_EOF
[Unit]
Description=Mem Linux Client Sync Daemon
After=network.target

[Service]
Type=simple
Environment=DISPLAY=:0
Environment=XDG_SESSION_TYPE=x11
ExecStart=$TARGET_BIN daemon
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
SERVICE_EOF

if command -v systemctl >/dev/null 2>&1; then
    systemctl --user daemon-reload
    systemctl --user enable --now mem-client.service
    log_info "Systemd 用户级守护服务已启动并设置为开机自启 (mem-client.service)"
fi

# ------------------------------------------------------------------------------
# 7. 验证连通性
# ------------------------------------------------------------------------------
log_step "7. 客户端服务状态检查"

if [ -x "$TARGET_BIN" ]; then
    "$TARGET_BIN" status || true
fi

echo -e "\n${COLOR_GREEN}======================================================${COLOR_RESET}"
echo -e "${COLOR_GREEN}🎉 Mem Linux 客户端安装与配置全部完成！${COLOR_RESET}"
echo -e "  - 核心命令:   mem-client (查看全部功能输入 mem-client help)"
echo -e "  - 快捷操作:   Win + V (拉取上屏)  |  Win + C (复制并推送)"
echo -e "  - 后台守护:   systemctl --user status mem-client.service"
echo -e "  - 配置文件:   ~/.config/mem/config.json"
echo -e "${COLOR_GREEN}======================================================${COLOR_RESET}\n"
