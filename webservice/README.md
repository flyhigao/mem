# Mem Web Service (中转服务器)

基于 Go 开发的跨端文本中转服务器，无任何 CGO 动态库依赖（SQLite 采用纯 Go 驱动），网页前端资源使用 `go:embed` 打包在单个二进制文件中。

---

## 一、编译与部署

### 1. 编译静态二进制文件（零外部依赖）

在任意安装了 Go 1.22+ 的机器上编译：

```bash
cd webservice
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o mem-server main.go
```

编译完成后，只需将得到的单个文件 `mem-server` 复制到目标服务器即可，无需安装其他任何环境。

---

## 二、运行参数与环境变量

可以通过命令行参数或环境变量指定监听地址与数据库路径：

| 命令行参数 | 环境变量 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| `-addr` | `ADDR` | `""` | 完整监听地址，例如 `127.0.0.1:48081` 或 `:8080` |
| `-host` | `HOST` | `""` | 监听主机 IP，例如 `127.0.0.1` 或 `0.0.0.0` |
| `-port` | `PORT` | `8080` | 监听端口 |
| `-db` | `DB_PATH` | `mem.db` | SQLite 数据库存储路径 |

### 本地监听示例（用于 Nginx 反向代理）
```bash
./mem-server -addr 127.0.0.1:48081 -db /opt/mem/mem.db
```

---

## 三、使用 Systemd 守护进程托管

推荐使用 systemd 管理进程，确保开机自启与异常重启：

创建服务文件 `/etc/systemd/system/mem-webservice.service`：

```ini
[Unit]
Description=Mem Web Relay Service
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/mem
ExecStart=/opt/mem/mem-server -addr 127.0.0.1:48081 -db /opt/mem/mem.db
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

激活并启动服务：
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now mem-webservice.service
sudo systemctl status mem-webservice.service
```

---

## 四、Nginx 反向代理配置

在目标服务器的 Nginx 配置文件（例如 `/etc/nginx/sites-available/mem.conf` 或 `conf.d/mem.conf`）中添加：

```nginx
server {
    listen 80;
    server_name mem.yourdomain.com; # 替换为你的域名或 IP

    # 如果有 SSL 证书（推荐 HTTPS）：
    # listen 443 ssl http2;
    # ssl_certificate /path/to/fullchain.pem;
    # ssl_certificate_key /path/to/privkey.pem;

    client_max_body_size 10M;

    location / {
        proxy_pass http://127.0.0.1:48081;
        
        # 基础代理标头
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket / 长连接支持
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        # 禁用代理缓冲以确保极低延迟
        proxy_buffering off;
        proxy_read_timeout 60s;
    }
}
```

测试并重载 Nginx：
```bash
sudo nginx -t
sudo nginx -s reload
```
