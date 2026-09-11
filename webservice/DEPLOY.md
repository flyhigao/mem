# Mem Web Service 生产部署方案（本机直连 + PROXY protocol + fail2ban 防爆破）

> 本文档记录 **2026-09-12** 在云端服务器的最终落地部署方案，与 [`README.md`](README.md) 的通用安装步骤互补。
>
> 核心特征：
> - 服务**直接部署在云端服务器本机**，不经 frpc 隧道（低延迟要求）；
> - 公网入口用 **8444 专用端口**，通过 stream 层 **PROXY protocol** 将真实客户端 IP 传到 Nginx；
> - Nginx 访问日志记录真实 IP，配合 **fail2ban 按 IP 封禁** 长时间密码爆破；
> - 443 入口只做 301 跳转，不处理登录，堵死绕过 fail2ban 的路径。

---

## 一、方案选型

| 方案 | 链路 | 延迟 | 结论 |
| :--- | :--- | :--- | :--- |
| 内网部署 + frpc 隧道 | 公网 → Nginx → frps → 内网 frpc → 服务 | 多一跳 FRP 转发 | ❌ 延迟敏感，不采用 |
| **本机部署 + 8444 专用入口** | 公网 8444 → stream(PROXY) → 127.0.0.1:18444 → 48081 | 仅本机回环一跳 | ✅ **采用** |

**为什么不用 443 做入口：**

1. 443 上的 stream server 同时转发 xray 的 xhttp(18392)/grpc(29454) 后端，这些后端**不认 PROXY protocol**，在 443 开启会打挂翻墙服务；
2. 不开 PROXY protocol 则真实 IP 在 stream 第一跳就丢失（日志里全是 `127.0.0.1`），fail2ban 无法按 IP 封禁；
3. 因此为 mem 单开 8444，只影响本服务，与 xray / frps 完全隔离。

**443 的角色：** 仅对 mem 做 `301 → https://mem.example.com:8444/`，不处理任何 API/登录请求。攻击者走 443 无法提交密码，也就无法绕过 8444 的 fail2ban。

> ⚠️ 客户端（Android / Linux / 浏览器书签）地址统一改为：**`https://mem.example.com:8444`**

---

## 二、部署环境

| 项 | 值 |
| :--- | :--- |
| 服务器 | 云端服务器 `<SERVER_HOSTNAME>`，公网 IP `<SERVER_PUBLIC_IP>` |
| 系统 | Ubuntu x86_64 + systemd 245 |
| Nginx | **Docker 容器** `nginx-3xui-proxy`（`nginx:1.24-alpine`，**host 网络模式**） |
| Nginx 配置（宿主机） | `/root/nginx-proxy/conf/nginx.conf`（只读挂载进容器） |
| Nginx 证书目录 | `/root/example.com/`（只读挂载到容器 `/etc/nginx/ssl/`） |
| 域名 / DNS | `mem.example.com` → `<SERVER_PUBLIC_IP>`（泛域名证书 `*.example.com` 已覆盖） |
| fail2ban | v0.11.1（已有 sshd/sshd-secure/frp-ssh/recidive 四个 jail） |
| 同机共存服务 | frps（`37000/50080/50443/37500`）、xray / x-ui（`2096/18392`） |

---

## 三、拓扑与请求链路

```text
                     ┌────────────────────────── 云端服务器 ──────────────────────────┐
 https://mem.example.com:8444                                                             │
        │            │  nginx:8444 (stream, proxy_protocol on)                        │
        └────────────┼─▶ PROXY 头携带真实客户端 IP ──▶ 127.0.0.1:18444                  │
                     │          │                                                     │
                     │          ▼  server_name mem.example.com                           │
                     │   set_real_ip_from + real_ip_header proxy_protocol              │
                     │          │  access_log → /root/nginx-proxy/logs/mem.access.log   │
                     │          ▼  （日志首列 = 真实客户端 IP）                          │
                     │   proxy_pass http://127.0.0.1:48081                             │
                     │          │                                                     │
                     │          ▼                                                     │
                     │   mem-server (systemd: mem-webservice，仅监听回环)               │
                     │          └──▶ SQLite WAL: /opt/mem/mem.db                      │
                     │                                                                │
                     │  nginx:443 = 各 *.example.com 的 SNI 分流（含 xray），mem 仅 301    │
                     └────────────────────────────────────────────────────────────────┘

 fail2ban jail「mem-web」
   监控 mem.access.log → 5 次登录失败/10 分钟 → iptables 封禁 8444 + Telegram 通知
```

---

## 四、部署步骤

### 4.1 下载安装（Release 二进制 + 校验）

```bash
sudo mkdir -p /opt/mem && cd /opt/mem

# 下载最新 amd64 二进制（本次部署版本：v1.0.0）
curl -fL -o /opt/mem/mem-server \
  https://github.com/flyhigao/mem/releases/latest/download/mem-server-linux-amd64
chmod +x /opt/mem/mem-server
```

**校验（SHA256SUMS 只收录压缩包，故用压缩包二次核对）：**

```bash
cd /tmp
curl -fL -O https://github.com/flyhigao/mem/releases/latest/download/mem-server-linux-amd64.tar.gz
curl -fL -O https://github.com/flyhigao/mem/releases/latest/download/SHA256SUMS.txt

grep 'mem-server-linux-amd64.tar.gz' SHA256SUMS.txt          # 期望 e0fc183e…ca3b92
sha256sum mem-server-linux-amd64.tar.gz                       # 对比一致

tar -xzf mem-server-linux-amd64.tar.gz mem-server-linux-amd64
sha256sum mem-server-linux-amd64                              # 期望 0419ff1f…c00768
sha256sum /opt/mem/mem-server                                 # 与包内二进制一致即 OK
```

### 4.2 systemd 守护进程（仅回环监听）

```bash
sudo cat << 'EOF' > /etc/systemd/system/mem-webservice.service
[Unit]
Description=Mem Web Relay Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/mem
ExecStart=/opt/mem/mem-server -addr 127.0.0.1:48081 -db /opt/mem/mem.db
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now mem-webservice.service
sudo systemctl status mem-webservice.service
```

> 关键：`-addr 127.0.0.1:48081`。公网严格不能直连该端口，必须经 Nginx。

### 4.3 Nginx 反代（容器化，宿主机改配置）

**先把两个前提说清楚：**

1. **改哪个文件**：本环境 Nginx 跑在 Docker 容器 `nginx-3xui-proxy`（host 网络），`nginx.conf` 以**只读**方式挂载进容器。所以**必须在宿主机改** `/root/nginx-proxy/conf/nginx.conf`，**不要**进容器改（只读，改不动）。
2. **`conf.d/` 是摆设**：本环境 `nginx.conf` 只 `include` 了 `mime.types`，**没有** `include /etc/nginx/conf.d/*.conf`，往 `conf.d/` 放配置不生效，必须直接改 `nginx.conf`。

#### 4.3.0 改动总览（相对原始配置，共 4 处）

> 原始配置 = 首次部署前的备份 `nginx.conf.bak-2026-09-11_235805`，里面**没有任何 mem 配置**。
> 若从"443 直连旧版"升级到本方案，对应关系是：①③ 为新增，② 为替换（原 18443 反代 → 301），④ 为改一行（跳转目标加 `:8444`）。

| # | 改在哪 | 原始配置 | 改成什么 | 为什么 |
| :-- | :-- | :-- | :-- | :-- |
| ① | `stream {}` 末尾（443 的 server 块之后） | 无 | 新增 `listen 8444` + `proxy_protocol on` → `127.0.0.1:18444` | 443 不能开 PROXY protocol（会打挂 xray 后端），单开 8444 携带真实客户端 IP |
| ② | `http {}` 中，`log.example.com` 块之后 | 无 | 新增 `listen 127.0.0.1:18443`，仅 `return 301` 到 `:8444` | 443 不再处理登录/接口，攻击者无法绕过 8444 的 fail2ban |
| ③ | `http {}` 中，紧接 ② 之后 | 无 | 新增 `listen 127.0.0.1:18444 ssl http2 proxy_protocol`：realip + 独立日志 + 反代 48081 | 还原真实 IP 写日志、反代后端 |
| ④ | `http {}` 中，紧接 ③ 之后（默认 80 块之前） | 无 | 新增 80 端口 server 块，`return 301` 到 `:8444` | `http://` 访问引导到正确入口 |

**明确不改动的部分**（避免误伤其他业务）：

| 部分 | 处理 |
| :-- | :-- |
| `stream {}` 的 `map $ssl_preread_server_name`、443 server 块、各 upstream | **不动**（xray / grpc / SNI 分流不受影响） |
| `http {}` 的 LLM 列表块、`log.example.com` 块、`limit_req` / `limit_conn` 定义 | **不动** |
| 80 端口默认块（`server_name localhost`） | **不动**（新增的 mem 块不会抢默认块） |
| frps / xray / x-ui 配置 | **完全不涉及** |

#### 4.3.1 插入位置示意

```text
stream {
    preread_buffer_size ...        # 不动
    map $ssl_preread_server_name   # 不动（*.example.com -> 18443）
    upstream grpc_service          # 不动
    upstream xhttp_service         # 不动
    upstream nginx_local_https     # 不动
    server { listen 443 ... }      # 不动（SNI 分流 + xray）
    ▼━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
     ① 新增：server { listen 8444; proxy_protocol on; }   ← 插在这里
    ▲━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
}

http {
    ...                            # 所有 upstream / limit 定义，不动
    server { ... }                 # LLM 列表块（18443），不动
    server { ... }                 # log.example.com 块（18443），不动
    ▼━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
     ② 新增：mem 443 入口只做 301（listen 127.0.0.1:18443）
     ③ 新增：mem 反代（listen 127.0.0.1:18444 proxy_protocol）
     ④ 新增：mem 80 端口 301（listen 80）
    ▲━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
    server { listen 80; server_name localhost; }   # 默认块，不动
}
```

#### 4.3.2 ① `stream {}` 末尾新增 8444 入口

> 插入位置：现有 443 的 `server { ... }` 块结束 `}` 之后、`stream {}` 的闭合 `}` 之前。

```nginx
    # ================================================================
    # [新增] mem.example.com 专用公网入口：8444（PROXY protocol 版）
    # ----------------------------------------------------------------
    # 为什么单独开端口：
    #   443 的 stream server 同时转发 xray 的 xhttp(18392)/grpc(29454)，
    #   这些后端不认 PROXY protocol，无法在 443 上开启；
    #   本入口仅服务 mem，与 SNI 分流、其他业务完全隔离。
    # 作用：把真实客户端 IP 通过 PROXY 协议带到 127.0.0.1:18444，
    #       使 nginx access log 能记录真实 IP，供 fail2ban 封禁。
    # ================================================================
    server {
        listen 8444 reuseport so_keepalive=60:2:3;
        proxy_pass 127.0.0.1:18444;
        proxy_protocol on;              # 关键：附带 PROXY 头（真实客户端 IP）
        tcp_nodelay on;
        proxy_connect_timeout 5s;
        proxy_timeout 3600s;
    }
```

#### 4.3.3 ② `http {}` 新增 mem 的 443→301 块

> 插入位置：`log.example.com` 的 server 块结束 `}` 之后。此时 mem.example.com 已经过 stream 的 SNI 分流到达 18443，由本块接管并跳转。

```nginx
    # ================================================================
    # [调整] 443 上的 mem.example.com：只做 301 跳转，不再反代
    # ----------------------------------------------------------------
    # 目的：公网 443 不处理任何登录/接口请求，攻击者无法从这里绕过
    #       8444 入口的 fail2ban；浏览器直接访问仍会被平滑引导到
    #       https://mem.example.com:8444/。
    # 注意：不改 stream 的 SNI 分流；18443 仍是各 *.example.com 的回环入口。
    # ================================================================
    server {
        listen 127.0.0.1:18443 ssl http2;
        server_name mem.example.com;

        ssl_certificate     /etc/nginx/ssl/fullchain.cer;
        ssl_certificate_key /etc/nginx/ssl/example.com.key;

        return 301 https://$host:8444$request_uri;
    }
```

#### 4.3.4 ③ `http {}` 新增 18444 反代块（PROXY + realip + 独立日志）

> 插入位置：紧跟 ② 之后。**相比普通反代只多 4 行关键配置**：`proxy_protocol`（listen）、`set_real_ip_from`、`real_ip_header`、`access_log`。

```nginx
    # ================================================================
    # [新增] mem.example.com 反代（PROXY protocol 版，供 fail2ban 使用）
    # ----------------------------------------------------------------
    # 请求链路：
    #   公网 8444 (stream: proxy_protocol on，见文件顶部 stream 块)
    #     -> 127.0.0.1:18444 (本 server，listen ... proxy_protocol)
    #     -> set_real_ip_from + real_ip_header 还原真实客户端 IP
    #     -> proxy_pass 本机 127.0.0.1:48081（systemd: mem-webservice）
    #
    # 重要：本块只接受带 PROXY 头的连接（即必须经 stream 8444 进入），
    #       直连 18444 会被拒绝，这是预期的安全行为。
    # ================================================================
    server {
        listen 127.0.0.1:18444 ssl http2 proxy_protocol;
        server_name mem.example.com;

        # 信任本机 stream 发来的 PROXY 头，把 $remote_addr 还原成真实客户端 IP
        set_real_ip_from 127.0.0.1;
        real_ip_header   proxy_protocol;

        # 独立访问日志：fail2ban 只监控这个文件，不与其他域名混在一起
        access_log /var/log/nginx/mem.access.log;

        # 复用现有泛域名证书：SAN = *.example.com, example.com，已覆盖 mem.example.com
        ssl_certificate     /etc/nginx/ssl/fullchain.cer;
        ssl_certificate_key /etc/nginx/ssl/example.com.key;
        ssl_protocols TLSv1.2 TLSv1.3;
        ssl_ciphers HIGH:!aNULL:!MD5;

        # 单次请求体上限（与 mem 文档一致：10M）
        client_max_body_size 10M;

        # 防慢速攻击：读写超时（30s 无数据即断开）
        client_body_timeout   30s;
        client_header_timeout 30s;

        location / {
            # 本机 systemd 托管的 mem-server，仅监听回环，不直接对外暴露
            proxy_pass http://127.0.0.1:48081;

            # 标准反代头（此时 X-Real-IP 已是真实客户端 IP）
            proxy_set_header Host              $host;
            proxy_set_header X-Real-IP         $remote_addr;
            proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;   # 告知后端真实协议为 https

            # WebSocket / 长连接支持（当前 mem 用轮询，保留以便后续扩展）
            proxy_http_version 1.1;
            proxy_set_header Upgrade    $http_upgrade;
            proxy_set_header Connection "upgrade";

            # 低延迟：关闭反代缓冲，数据立即透传
            proxy_buffering         off;
            proxy_request_buffering off;
            proxy_connect_timeout   10s;
            proxy_read_timeout      60s;
        }
    }
```

#### 4.3.5 ④ `http {}` 新增 80 端口跳转块

> 插入位置：紧跟 ③ 之后、80 端口默认块（`server_name localhost`）之前。

```nginx
    # ================================================================
    # [新增] mem.example.com 的 80 端口 -> 301 跳转 HTTPS
    # ----------------------------------------------------------------
    # 说明：
    #   1) 现有 80 端口的 server 是 server_name localhost（默认块），本块
    #      只匹配 mem.example.com，不会抢走默认块；
    #   2) 目的是避免用户用 http:// 访问时看到默认欢迎页。
    #   3) 若不需要 HTTP 跳转，删除本 server 块即可（不影响 HTTPS）。
    # ================================================================
    server {
        listen 80;
        listen [::]:80;
        server_name mem.example.com;
        return 301 https://$host:8444$request_uri;
    }
```

#### 4.3.6 完整 diff：原始配置 → 当前配置

<details>
<summary>点击展开完整 diff（原始 nginx.conf 无任何 mem 配置，以下 113 行全部为新增内容）</summary>

```diff
@@ -64,6 +64,25 @@
         # 代理连接维持时间 (设长一点，避免正常使用的长连接被切断)
         proxy_timeout 3600s;
     }
+
+    # ================================================================
+    # [新增] mem.example.com 专用公网入口：8444（PROXY protocol 版）
+    # ----------------------------------------------------------------
+    # 为什么单独开端口：
+    #   443 的 stream server 同时转发 xray 的 xhttp(18392)/grpc(29454)，
+    #   这些后端不认 PROXY protocol，无法在 443 上开启；
+    #   本入口仅服务 mem，与 SNI 分流、其他业务完全隔离。
+    # 作用：把真实客户端 IP 通过 PROXY 协议带到 127.0.0.1:18444，
+    #       使 nginx access log 能记录真实 IP，供 fail2ban 封禁。
+    # ================================================================
+    server {
+        listen 8444 reuseport so_keepalive=60:2:3;
+        proxy_pass 127.0.0.1:18444;
+        proxy_protocol on;              # 关键：附带 PROXY 头（真实客户端 IP）
+        tcp_nodelay on;
+        proxy_connect_timeout 5s;
+        proxy_timeout 3600s;
+    }
 }
 
 http {
@@ -177,6 +196,99 @@
         }
     }
 
+    # ================================================================
+    # [调整] 443 上的 mem.example.com：只做 301 跳转，不再反代
+    # ----------------------------------------------------------------
+    # 目的：公网 443 不处理任何登录/接口请求，攻击者无法从这里绕过
+    #       8444 入口的 fail2ban；浏览器直接访问仍会被平滑引导到
+    #       https://mem.example.com:8444/。
+    # 注意：不改 stream 的 SNI 分流；18443 仍是各 *.example.com 的回环入口。
+    # ================================================================
+    server {
+        listen 127.0.0.1:18443 ssl http2;
+        server_name mem.example.com;
+
+        ssl_certificate     /etc/nginx/ssl/fullchain.cer;
+        ssl_certificate_key /etc/nginx/ssl/example.com.key;
+
+        return 301 https://$host:8444$request_uri;
+    }
+
+    # ================================================================
+    # [新增] mem.example.com 反代（PROXY protocol 版，供 fail2ban 使用）
+    # ----------------------------------------------------------------
+    # 请求链路：
+    #   公网 8444 (stream: proxy_protocol on，见文件顶部 stream 块)
+    #     -> 127.0.0.1:18444 (本 server，listen ... proxy_protocol)
+    #     -> set_real_ip_from + real_ip_header 还原真实客户端 IP
+    #     -> proxy_pass 本机 127.0.0.1:48081（systemd: mem-webservice）
+    #
+    # 重要：本块只接受带 PROXY 头的连接（即必须经 stream 8444 进入），
+    #       直连 18444 会被拒绝，这是预期的安全行为。
+    # ================================================================
+    server {
+        listen 127.0.0.1:18444 ssl http2 proxy_protocol;
+        server_name mem.example.com;
+
+        # 信任本机 stream 发来的 PROXY 头，把 $remote_addr 还原成真实客户端 IP
+        set_real_ip_from 127.0.0.1;
+        real_ip_header   proxy_protocol;
+
+        # 独立访问日志：fail2ban 只监控这个文件，不与其他域名混在一起
+        access_log /var/log/nginx/mem.access.log;
+
+        # 复用现有泛域名证书：SAN = *.example.com, example.com，已覆盖 mem.example.com
+        ssl_certificate     /etc/nginx/ssl/fullchain.cer;
+        ssl_certificate_key /etc/nginx/ssl/example.com.key;
+        ssl_protocols TLSv1.2 TLSv1.3;
+        ssl_ciphers HIGH:!aNULL:!MD5;
+
+        # 单次请求体上限（与 mem 文档一致：10M）
+        client_max_body_size 10M;
+
+        # 防慢速攻击：读写超时（30s 无数据即断开）
+        client_body_timeout   30s;
+        client_header_timeout 30s;
+
+        location / {
+            # 本机 systemd 托管的 mem-server，仅监听回环，不直接对外暴露
+            proxy_pass http://127.0.0.1:48081;
+
+            # 标准反代头（此时 X-Real-IP 已是真实客户端 IP）
+            proxy_set_header Host              $host;
+            proxy_set_header X-Real-IP         $remote_addr;
+            proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
+            proxy_set_header X-Forwarded-Proto $scheme;   # 告知后端真实协议为 https
+
+            # WebSocket / 长连接支持（当前 mem 用轮询，保留以便后续扩展）
+            proxy_http_version 1.1;
+            proxy_set_header Upgrade    $http_upgrade;
+            proxy_set_header Connection "upgrade";
+
+            # 低延迟：关闭反代缓冲，数据立即透传
+            proxy_buffering         off;
+            proxy_request_buffering off;
+            proxy_connect_timeout   10s;
+            proxy_read_timeout      60s;
+        }
+    }
+
+    # ================================================================
+    # [新增] mem.example.com 的 80 端口 -> 301 跳转 HTTPS
+    # ----------------------------------------------------------------
+    # 说明：
+    #   1) 现有 80 端口的 server 是 server_name localhost（默认块），本块
+    #      只匹配 mem.example.com，不会抢走默认块；
+    #   2) 目的是避免用户用 http:// 访问时看到默认欢迎页。
+    #   3) 若不需要 HTTP 跳转，删除本 server 块即可（不影响 HTTPS）。
+    # ================================================================
+    server {
+        listen 80;
+        listen [::]:80;
+        server_name mem.example.com;
+        return 301 https://$host:8444$request_uri;
+    }
+
     server {
         listen 80;
         listen [::]:80;  # [新增] 监听 IPv6
```

</details>

### 4.4 fail2ban 防爆破

**Filter：`/etc/fail2ban/filter.d/mem-web.conf`**

```ini
[Definition]

failregex = ^<HOST> - - .*"POST /web/api/login HTTP/[0-9.]+" 401 \d+
            ^<HOST> - - .*"POST /web/api/register HTTP/[0-9.]+" (400|429) \d+

ignoreregex =
```

> ⚠️ **本机 fail2ban 0.11.1 的引擎对转义方括号 `\]` 会失配**（`\[[^\]]+\]` 这种日期匹配实测 0 命中），
> 因此日期部分用 `.*` 代替，功能等价且实测可正常命中。

**Jail：`/etc/fail2ban/jail.d/mem-web.conf`**

```ini
[mem-web]
enabled  = true
port     = 8444
filter   = mem-web
logpath  = /root/nginx-proxy/logs/mem.access.log
backend  = auto
maxretry = 5
findtime = 600
bantime  = 86400
usedns   = warn
ignoreip = 127.0.0.1/8 ::1 192.168.0.0/16 <SERVER_PUBLIC_IP> <ADMIN_NET> <ADMIN_IP_1> <ADMIN_IP_2> <ADMIN_IP_3> <ADMIN_IP_4>
action   = iptables-multiport[name=mem-web, port=8444, protocol=tcp]
           telegram-notify[bot_token="...", chat_id="..."]
```

**ignoreip 说明：**

| 网段 | 原因 |
| :--- | :--- |
| `127.0.0.1/8 ::1` | 本机 |
| `192.168.0.0/16` | 局域网 |
| `<SERVER_PUBLIC_IP>` | 本机公网 IP（内网用户 NAT 回环访问时源地址就是它，不忽略会误封自己人） |
| `<ADMIN_NET>` 等 | 常用管理出口（与 sshd/recidive jail 保持一致） |

> 外部攻击者无法用上述源 IP 完成 TCP 握手，因此白名单不构成绕过。

**生效：**

```bash
fail2ban-client reload
fail2ban-client status mem-web
```

### 4.5 Nginx 生效（改配置前先备份）

```bash
# 1. 备份（必做）
cd /root/nginx-proxy/conf
cp -a nginx.conf "nginx.conf.bak-$(date +%F_%H%M%S)"

# 2. 语法检查（宿主机改完，容器内立即可见）
docker exec nginx-3xui-proxy nginx -t

# 3. 平滑生效（推荐）
docker exec nginx-3xui-proxy nginx -s reload
```

> **生效方式说明**：优先用 `nginx -s reload`（平滑，不断开现有连接，xray 的 443 入口无感知）。
> `cd ~/nginx-proxy && docker compose restart` 同样能生效，但会瞬断 443 上所有连接（含 xray SNI 分流流量），非必要不用。

---

## 五、验证

### 5.1 链路与真实 IP

```bash
# 1) 本地回环链路（SNI 正确 + PROXY + 反代 + 后端）
curl -sk --resolve mem.example.com:8444:127.0.0.1 https://mem.example.com:8444/api/v1/ping

# 2) 走公网域名（真实公网路径）
curl -s https://mem.example.com:8444/api/v1/ping

# 3) 关键：访问后看日志首列是否为"真实客户端 IP"（而不是 127.0.0.1）
tail -5 /root/nginx-proxy/logs/mem.access.log
```

### 5.2 入口收敛

```bash
# 443 必须只返回 301 到 :8444
curl -sk --resolve mem.example.com:443:127.0.0.1 -o /dev/null -w "443 -> %{http_code} %{redirect_url}\n" https://mem.example.com/

# 80 也必须 301 到 :8444
curl -s -o /dev/null -w "80  -> %{http_code} %{redirect_url}\n" -H 'Host: mem.example.com' http://127.0.0.1/

# 其他域名不受影响（举例）
curl -sk --resolve log.example.com:443:127.0.0.1 -o /dev/null -w "log.example.com -> %{http_code}\n" https://log.example.com/
```

### 5.3 fail2ban 过滤规则

```bash
cat > /tmp/two.log <<'EOF'
203.0.113.66 - - [11/Sep/2026:16:20:01 +0000] "POST /web/api/login HTTP/2.0" 401 53 "-" "curl/7.68.0"
203.0.113.66 - - [11/Sep/2026:16:20:02 +0000] "POST /web/api/register HTTP/2.0" 400 40 "-" "curl/7.68.0"
203.0.113.66 - - [11/Sep/2026:16:20:03 +0000] "GET / HTTP/2.0" 200 25254 "-" "curl/7.68.0"
EOF
fail2ban-regex /tmp/two.log /etc/fail2ban/filter.d/mem-web.conf
# 期望：login 401 与 register 400 各 1 命中，GET 200 不命中
```

### 5.4 自动封禁（端到端）

```bash
# 往真实日志写 5 条当前时间的失败登录（测试网段，不影响真人）
for i in 1 2 3 4 5; do
  ts=$(date -u '+%d/%b/%Y:%H:%M:%S +0000')
  echo "203.0.113.66 - - [$ts] \"POST /web/api/login HTTP/2.0\" 401 53 \"-\" \"curl/7.68.0\"" \
    >> /root/nginx-proxy/logs/mem.access.log
  sleep 1
done
sleep 12
fail2ban-client status mem-web          # 期望 Banned IP list: 203.0.113.66
iptables -L f2b-mem-web -n              # 期望 REJECT 规则

# 验证完解封并清理测试日志
fail2ban-client set mem-web unbanip 203.0.113.66
grep -v '203.0.113.66' /root/nginx-proxy/logs/mem.access.log > /tmp/mem.clean
cat /tmp/mem.clean > /root/nginx-proxy/logs/mem.access.log   # 保持 inode
rm -f /tmp/mem.clean /tmp/two.log
fail2ban-client reload mem-web
```

### 5.5 其他服务回归

```bash
docker exec nginx-3xui-proxy tail -20 /var/log/nginx/error.log   # 无 error
systemctl status mem-webservice --no-pager
fail2ban-client status                                            # 5 个 jail 均在
```

---

## 六、日常运维

```bash
systemctl status mem-webservice        # 状态
systemctl restart mem-webservice       # 重启服务
journalctl -u mem-webservice -f        # 实时日志

fail2ban-client status mem-web         # 查看封禁情况
fail2ban-client set mem-web unbanip <IP>   # 手动解封
fail2ban-client set mem-web banip <IP>     # 手动封禁

# 升级：替换二进制后重启（SQLite 数据在 /opt/mem/mem.db，不受影响）
systemctl stop mem-webservice
curl -fL -o /opt/mem/mem-server https://github.com/flyhigao/mem/releases/latest/download/mem-server-linux-amd64
chmod +x /opt/mem/mem-server
systemctl start mem-webservice

# 备份数据
cp /opt/mem/mem.db /opt/mem/mem.db.bak-$(date +%F)
```

---

## 七、回滚

```bash
# ① 完全回滚（回到 443 直连方案）
cd /root/nginx-proxy/conf
cp nginx.conf.bak-2026-09-11_235805 nginx.conf          # 首次部署前的原始配置
docker exec nginx-3xui-proxy nginx -t && docker exec nginx-3xui-proxy nginx -s reload
fail2ban-client stop mem-web 2>/dev/null || true
rm -f /etc/fail2ban/filter.d/mem-web.conf /etc/fail2ban/jail.d/mem-web.conf
fail2ban-client reload

# ② 只回滚 fail2ban（保留 8444 架构）
fail2ban-client set mem-web unbanip --all 2>/dev/null || true
rm -f /etc/fail2ban/filter.d/mem-web.conf /etc/fail2ban/jail.d/mem-web.conf
fail2ban-client reload

# ③ 服务完全卸载
systemctl disable --now mem-webservice
rm -f /etc/systemd/system/mem-webservice.service
systemctl daemon-reload
rm -rf /opt/mem
```

---

## 八、注意事项 / 踩坑记录

1. **客户端地址必须带端口**：`https://mem.example.com:8444`。不带端口走 443 只会拿到 301。
2. **8444 需要在云厂商安全组放行**（本机 iptables 默认 ACCEPT 没问题，但控制台安全组看不到）。
3. **不要给 443 的 stream 开 PROXY protocol**：会把 xray 的 xhttp/grpc 后端打挂。
4. **18444 只接受带 PROXY 头的连接**：这是安全设计，直连 18444 会失败。
5. **配置必须在宿主机改**：容器内 `/etc/nginx/nginx.conf` 是只读挂载。
6. **`conf.d/` 是摆设**：本环境 `nginx.conf` 没有 include 它，往里放配置不生效。
7. **不要复用 frps 的 `server_name` 列表**：那个 server 块把请求交给 frps 按 Host 隧道分发；mem 不走 frpc，只加域名会得到 frps 的 `404 no route found`。
8. **fail2ban 0.11.1 对 `\]` 失配**：filter 里日期部分用 `.*`，不要用 `\[[^\]]+\]`（详见 4.4 注释）。
9. **ignoreip 必须包含 `<SERVER_PUBLIC_IP>`**：内网用户 NAT 回环访问时源地址是该公网 IP，否则会误封自己人。
10. **reload 优于 restart**：`docker compose restart` 会瞬断 443 上所有连接（含 xray）；`nginx -s reload` 平滑无感。
11. **真实 IP 仅在 8444 链路可用**：443 上其它 *.example.com 域名（含 frps 隧道业务）日志里仍是 `127.0.0.1`。
12. **端口占用**：mem 用 `8444`（公网）→ `18444`（回环）→ `48081`（回环），与 frps、xray/x-ui、Nginx 既有端口均无冲突。
13. **安全**：mem-server 永远保持 `127.0.0.1`；公网仅 `https://mem.example.com:8444` 一个入口，带用户注册/Token 鉴权 + fail2ban。

---

## 九、线上配置存档位置

| 内容 | 路径 |
| :--- | :--- |
| mem-server 二进制 | `/opt/mem/mem-server` |
| SQLite 数据库 | `/opt/mem/mem.db`（WAL 模式，另有 `-wal/-shm`） |
| systemd 单元 | `/etc/systemd/system/mem-webservice.service` |
| Nginx 主配置 | `/root/nginx-proxy/conf/nginx.conf` |
| Nginx 备份（首次部署前） | `/root/nginx-proxy/conf/nginx.conf.bak-2026-09-11_235805` |
| Nginx 备份（route A 改造前） | `/root/nginx-proxy/conf/nginx.conf.bak-optionA-2026-09-12_001330` |
| fail2ban filter | `/etc/fail2ban/filter.d/mem-web.conf` |
| fail2ban jail | `/etc/fail2ban/jail.d/mem-web.conf` |
| 8444 访问日志（供 fail2ban） | `/root/nginx-proxy/logs/mem.access.log` |

---

## 十、附录：后续新增 FRPC 服务（沿用原流程）

> 适用于：**运行在内网、通过 frpc 隧道**暴露的新服务。
> mem 本身是"本机 + 8444 专用入口"的特例，**不要**按本章操作（详见第八节第 7 条）。

本方案只新增了 mem 自己的独立入口，**没有改动** frps 那条链路，因此新服务仍然沿用原来的老流程。

### 10.1 标准步骤

1. **域名解析**：域名商把新域名 A 记录指向 `<SERVER_PUBLIC_IP>`（若 `*.example.com` 已有泛解析则跳过）。

2. **内网 `frpc.toml` 新增一段 proxy**（`name` 全局唯一）：

   ```toml
   [[proxies]]
   name = "新服务名"
   type = "http"
   localIP = "127.0.0.1"
   localPort = 12345
   customDomains = ["new.example.com"]
   ```

   生效：配了 `webServer` 时用 `frpc reload -c ./frpc.toml` 热加载；否则 `systemctl restart frpc`（会让该 frpc 上**所有**隧道瞬断重连几秒）。

3. **云端 `nginx.conf` 追加域名**：找到 `http {}` 中的 **frps 那个 server 块**（特征：`server_name zxai.example.com claw.example.com ...` 开头、`proxy_pass http://frps_backend;`），把新域名空格追加到 `server_name` 末尾：

   ```nginx
   server_name zxai.example.com claw.example.com aiapi.example.com trans.example.com study.example.com dsh.example.com daka.example.com new.example.com;
   ```

4. **生效（先备份）**：

   ```bash
   cd /root/nginx-proxy/conf
   cp -a nginx.conf "nginx.conf.bak-$(date +%F_%H%M%S)"
   docker exec nginx-3xui-proxy nginx -t
   docker exec nginx-3xui-proxy nginx -s reload     # 不要用 compose restart
   ```

5. **验证**：

   ```bash
   curl -sk --resolve new.example.com:443:127.0.0.1 https://new.example.com/ -o /dev/null -w '%{http_code}\n'
   # frps Dashboard（127.0.0.1:37500）确认新 proxy 已注册
   journalctl -u frps -n 20 | grep new.example.com    # 云端 frps 侧日志
   ```

### 10.2 四个前提 / 注意事项

| # | 事项 | 说明 |
| :-- | :-- | :-- |
| 1 | **域名必须是 `*.example.com`** | 443 的 SNI map 只把 `~.*\.codet\.net$` 路由到 18443（frps 这套）；其它域名会命中 `default → xhttp_service`（xray），需要额外加 stream map 条目 + 证书 + server 块 |
| 2 | **共享该块的全局限速** | 该块有 `client_max_body_size 128m`、`limit_req zone=llm_api burst=80 nodelay`（30r/s）、`limit_conn llm_conn 40`。因 443 未传 PROXY protocol，18443 看到的客户端恒为 `127.0.0.1`，这些限制实际上是**所有域名、所有客户端共用一个桶**；新服务流量大时会和 LLM 服务互相挤占（429/503） |
| 3 | **拿不到真实客户端 IP** | 这条链路（443→18443）日志恒为 `127.0.0.1`，**无法做按 IP 的 fail2ban**；要真实 IP 就仿照 mem 走"专用端口 + PROXY protocol"方案（第四、五章） |
| 4 | **生效一律 reload** | 443 的 stream 同时承载 xray 分流，`docker compose restart` 会瞬断全部 443 连接 |

### 10.3 什么时候不要只加 `server_name`

| 新服务特征 | 建议做法 |
| :-- | :-- |
| 普通轻量服务（`*.example.com`、走 frps、流量小） | ✅ 按 10.1 老流程，加域名即可 |
| 大流量 / 上传大文件 / 并发高 | ⚠️ 单独建一个 server 块（参考 4.3.4，去掉 `limit_req`/`limit_conn`），避免与 LLM 抢额度 |
| 需要真实客户端 IP / 需要 fail2ban 按 IP 封禁 | ⚠️ 走 mem 同款"专用端口 + PROXY protocol + 独立 access_log + fail2ban"方案 |
| 非 `*.example.com` 域名 | ⚠️ 需改 stream map + 证书 + server 块，改动较大，先评估 |
