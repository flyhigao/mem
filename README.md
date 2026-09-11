# Mem (跨端文本中转站)

**Mem** 是一套轻量高效的跨端剪贴板/文本中转方案。旨在解决手机端快速输入（悬浮窗/侧边栏）与电脑桌面端（Linux/Mac/Windows）之间无缝流转文本的需求。

---

## 架构设计

```text
  ┌────────────────┐           ┌────────────────────────────────┐           ┌────────────────┐
  │   Android APP  │           │          Web Service           │           │  Linux Client  │
  │  (OnePlus 13)  │ ──POST──> │       (Go + SQLite3 WAL)       │ <──PULL── │  (mem-client)  │
  │ 悬浮输入/历史记录│           │ http://192.168.2.97:8080       │           │ 全局快捷键拉取  │
  └────────────────┘           │ (用户隔离 + Token鉴权 + WebUI)  │           │ 自动入剪贴板/粘贴│
                               └────────────────────────────────┘           └────────────────┘
```

---

## 快速下载 (GitHub Releases)

各组件预编译二进制文件均可在 [Releases 页面](https://github.com/flyhigao/mem/releases/latest) 直接获取：

- **Web 中转服务**：
  - Linux amd64: `curl -fL -o /opt/mem/mem-server https://github.com/flyhigao/mem/releases/latest/download/mem-server-linux-amd64 && chmod +x /opt/mem/mem-server`
  - Linux arm64: [mem-server-linux-arm64.tar.gz](https://github.com/flyhigao/mem/releases/latest/download/mem-server-linux-arm64.tar.gz)
- **Linux 客户端**：
  - Linux amd64: `curl -fL -o ~/.local/bin/mem-client https://github.com/flyhigao/mem/releases/latest/download/mem-client-linux-amd64 && chmod +x ~/.local/bin/mem-client`
- **Android 客户端**：
  - 直接在手机上运行 `androidapp` 编译产物，支持悬浮窗输入与剪贴板同步。

---

## 子目录说明

### 1. `webservice` (中转服务器)
- 详细部署与 Nginx 反代文档请参阅：[`webservice/README.md`](webservice/README.md)
- 技术栈：Go + SQLite (WAL 模式) + 单文件嵌入式 Web UI
- 特性：
  - 用户注册与登录（密码经 bcrypt 哈希）
  - API Token 隔离（各用户独立，支持多设备命名）
  - 自动保留各用户最新 500 条记录
  - 支持自定义监听地址（`-addr 127.0.0.1:48081`），配合 Nginx 反代最佳
  - 网页控制台：快捷输入发送、历史列表倒序展示、一键复制、单条删除、定时自动刷新

### 2. `linuxapp` (Linux 桌面客户端)
- 详细使用与快捷键配置文档请参阅：[`linuxapp/README.md`](linuxapp/README.md)
- 技术栈：Go 单二进制文件（零外部 CGO 依赖）
- 兼容性：
  - 剪贴板写入：自动适配 `wl-clipboard` (Wayland) 或 `xclip` / `xsel` / `python3-tkinter` (X11)
  - 模拟粘贴：支持 `xdotool`、`ydotool`、`wtype` 触发自动粘贴
  - 桌面通知：通过 `notify-send` 弹出内容预览
- 使用方式：
  - 绑定系统快捷键（如 `Ctrl + Alt + V`）执行 `mem-client pull` 即可瞬间获取最新手机内容并自动粘贴。

### 3. `androidapp` (Android 手机客户端)
- 实测设备：**一加 13 (PJZ110 / ColorOS 15 / Android 16)**
- 技术栈：Kotlin + Jetpack Compose + OkHttp + DataStore Preferences
- 功能：
  - 服务端连接配置与连通性测试
  - 悬浮输入窗口：常驻后台前台服务，可随意拖拽，浮窗内直接输入/粘贴/发送
  - 主界面消息历史查看与一键复制
