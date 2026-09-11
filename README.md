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

整个项目包含 3 个核心组件：
- **`webservice/`**：基于 Go 开发的中转服务，内嵌响应式 Web 管理界面，支持用户注册/登录、多设备 API Token 隔离、保留最新 500 条消息、REST API 等。
- **`linuxapp/`**：基于 Go 开发的 Linux 客户端，按下快捷键即可拉取最新数据、自动写入系统剪贴板（兼容 Wayland 与 X11）并直接模拟粘贴。
- **`androidapp/`**：原生 Kotlin + Jetpack Compose 打造的轻量安卓客户端，已在 OnePlus 13 (Android 16) 运行，具备主界面历史管理与轻量浮动输入窗。

---

## 子目录说明

### 1. `webservice` (中转服务器)
- 运行位置：`192.168.2.97:8080` (由 systemd 托管开机自启)
- 技术栈：Go + SQLite (WAL 模式) + 单文件嵌入式 Web UI
- 功能：
  - 用户注册与登录（密码经 bcrypt 哈希）
  - API Token 管理（各用户隔离，多设备互不干扰）
  - 消息保留：自动截断保持每个用户最新 500 条记录
  - 网页控制台：快捷发送、历史消息列表倒序查看、一键复制、删除、定时自动刷新
  - RESTful API 规范：
    - `POST /api/v1/messages` (发送文本)
    - `GET /api/v1/messages/latest` (获取最新一条，支持 `?format=text` 纯文本格式)
    - `GET /api/v1/messages` (分页获取历史记录)

### 2. `linuxapp` (Linux 桌面客户端)
- 运行位置：Linux 桌面终端或绑定系统全局快捷键
- 技术栈：Go 单二进制文件（零外部 CGO 依赖）
- 兼容性：
  - 剪贴板写入：自动识别并调用 `wl-clipboard` (Wayland) 或 `xclip` / `xsel` (X11)
  - 模拟粘贴：支持 `xdotool`、`ydotool`、`wtype`
  - 桌面通知：通过 `notify-send` 弹出内容预览
- 使用方式：
  - `mem-client pull`：获取最新内容并自动粘贴到当前聚焦的输入框
  - `mem-client push "内容"`：终端命令行直接发送内容至中转站
  - 系统快捷键集成：绑定快捷键执行 `mem-client pull` 即可实现一键跨端粘贴

### 3. `androidapp` (Android 手机客户端)
- 设备验证：已在 **一加 13 (PJZ110 / ColorOS 15 / Android 16)** 实测安装运行
- 技术栈：Kotlin + Jetpack Compose + OkHttp + DataStore Preferences
- 功能：
  - 服务配置与网络连通性一键测试
  - 主界面快捷输入与一键粘贴系统剪贴板
  - 历史记录查看与一键复制
  - 悬浮输入窗：常驻前台服务，可任意拖拽移动，浮窗内直接输入并一键推送至云端供桌面端使用
