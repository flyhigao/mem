# Mem 项目未来规划与技术演进方案 (Roadmap)

本文档记录已充分论证并达成共识的未来迭代方案。当前版本（v1.0）保持极简与稳定，待后续有高频/即时跨端提醒需求时，按本方案执行落地。

---

## 规划一：Android 端实时推送与深睡唤醒方案 (SSE Push & Deep Sleep Wakeup)

### 1. 需求与痛点分析
* **轮询的弊端**：短周期 HTTP 轮询会导致手机射频基带（Radio State Machine）频繁进出活跃状态（DCH/FACH/IDLE），尾部功耗（Tail Energy）极高，导致发热与异常耗电。
* **系统统一调度的局限**：Android 的 `WorkManager` / `JobScheduler` 虽然能聚合对齐唤醒，但其系统硬性最小周期为 **15 分钟**，无法满足即时文本流转的秒级时效要求。
* **休眠与飞行模式需求**：夜间开启飞行模式（仅保留 Wi-Fi）以及灭屏静置触发 Android Deep Doze（深层休眠）时，仍需保证新消息能够秒级送达并唤醒提醒。

---

### 2. 核心架构设计

```text
┌─────────────────┐                                  ┌───────────────────────────────┐
│   Web Service   │                                  │          Android App          │
│  (192.168.2.97) │                                  │          (OnePlus 13)         │
└────────┬────────┘                                  └───────────────┬───────────────┘
         │                                                           │
         │  1. 建立 SSE 长连接 GET /api/v1/messages/stream           │
         │ <─────────────────────────────────────────────────────────┤ (基于已存的前台服务)
         │                                                           │
         │  2. 周期心跳 (每 30 秒 :keepalive\n\n)                     │
         │ ────────────────────────────────────────────────────────> │ 维持路由器 NAT 映射表
         │                                                           │ (平时 0 功耗，无数据发送)
         │                                                           │
         │  3. 桌面端/网页端 POST 新消息入库                          │
[新消息到达]                                                          │
         │  4. 立即单向推送到 SSE 通道                                │
         │ ────────────────────────────────────────────────────────> │ Wi-Fi 硬件中断唤醒内核
         │                                                           │ 申请 5s CPU WakeLock
         │                                                           │ 播放系统通知铃声
         │                                                           │ 唤起/更新悬浮窗展示预览
```

---

### 3. 组件改造细则

#### A. 服务端 (Go Web Service)
* **新增接口**：`GET /api/v1/messages/stream` (SSE Server-Sent Events)
* **鉴权**：通过 `Authorization: Bearer <token>` 或 `?token=<token>` 鉴权，并将 Client 注册到该 UserID 对应的广播频道（Channel）。
* **心跳保活**：定时器每 30 秒发送一次 `:keepalive\n\n`，防止中间网络代理、家用路由器超时断开 TCP。
* **消息广播**：当 `POST /api/v1/messages` 收到新文本后，立刻通知该 User 下所有在线的 SSE 连接写入数据。

#### B. 客户端 (Android App)
* **长连接守护**：依托已有的 `FloatingInputService` 前台服务，使用 `okhttp3-sse` (EventSource) 维持连接，断线自动指数退避重连。
* **夜间飞行模式支持**：SSE 基于底层 TCP/IP 协议，飞行模式仅关闭蜂窝射频，Wi-Fi 连接正常工作，不受任何影响。
* **深睡（Deep Doze）穿透机制**：
  1. **电池白名单**：在 App 内通过 `android.permission.REQUEST_IGNORE_BATTERY_OPTIMIZATIONS` 引导用户将 Mem 加入“电池不优化”列表，系统在 Doze 模式下不会切断该 App 的网络。
  2. **WakeLock 局部唤醒**：接收到 SSE 事件回调时，通过 `PowerManager.newWakeLock(PARTIAL_WAKE_LOCK)` 持有 3~5 秒唤醒锁，保证 CPU 及时处理业务。
* **提醒与交互**：
  1. **声音提醒**：使用 `RingtoneManager.getDefaultUri(RingtoneManager.TYPE_NOTIFICATION)` 播放系统默认提示音，遵循免打扰/静音策略。
  2. **悬浮窗即时提醒**：若当前悬浮窗未打开或最小化，自动弹出小巧的悬浮通知卡片，显示发送者标签、文本前 60 字预览，并提供【复制】与【展开】按钮。

---

### 4. 实施准备清单 (待后续开启时执行)
- [ ] `webservice/internal/handlers/sse.go`：实现基于 Go 原生 channel 的发布/订阅事件总线。
- [ ] `webservice/main.go`：挂载 `/api/v1/messages/stream` 路由。
- [ ] `androidapp/app/build.gradle.kts`：引入 `com.squareup.okhttp3:okhttp-sse`。
- [ ] `androidapp/app/src/main/AndroidManifest.xml`：补充 `WAKE_LOCK`、`REQUEST_IGNORE_BATTERY_OPTIMIZATIONS` 权限声明。
- [ ] `androidapp`：实现 SSE 客户端连接、通知音播放与悬浮窗新消息弹出联动。
