# Mem Web Service (中转服务器)

基于 Go 开发的跨端文本中转服务器，无任何 CGO 动态库依赖（SQLite 采用纯 Go 驱动），网页前端资源使用 `go:embed` 打包在单个二进制文件中。

---

## 一、下载安装（推荐直接从 Release 获取）

每次发布新版本时，GitHub Actions 会自动编译多架构无依赖二进制文件。

### 1. Linux x86_64 / amd64（主流云服务器 / VPS）
```bash
sudo mkdir -p /opt/mem && cd /opt/mem

# 下载最新版本二进制并赋予权限
sudo curl -fL -o /opt/mem/mem-server https://github.com/flyhigao/mem/releases/latest/download/mem-server-linux-amd64
sudo chmod +x /opt/mem/mem-server
```

### 2. Linux ARM64（甲骨文 ARM、树莓派、鲲鹏等）
```bash
sudo mkdir -p /opt/mem && cd /opt/mem

# 下载最新版本并解压
sudo curl -fL -o mem-server.tar.gz https://github.com/flyhigao/mem/releases/latest/download/mem-server-linux-arm64.tar.gz
sudo tar -xzf mem-server.tar.gz
sudo mv mem-server-linux-arm64 /opt/mem/mem-server
sudo chmod +x /opt/mem/mem-server
sudo rm -f mem-server.tar.gz
```

> **可选：源码编译（备用方式）**
> ```bash
> git clone https://github.com/flyhigao/mem.git
> cd mem/webservice
> CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /opt/mem/mem-server .
> ```

---

## 二、运行参数与环境变量

可直接通过命令行参数或环境变量控制服务监听地址与数据库路径：

| 命令行参数 | 环境变量 | 默认值 | 说明 |
| :--- | :--- | :--- | :--- |
| `-addr` | `ADDR` | `""` | 完整监听地址，例如 `127.0.0.1:48081` 或 `:8080` |
| `-host` | `HOST` | `""` | 监听主机 IP，例如 `127.0.0.1` 或 `0.0.0.0` |
| `-port` | `PORT` | `8080` | 监听端口 |
| `-db` | `DB_PATH` | `mem.db` | SQLite 数据库存储路径 |

### 测试运行（监听 127.0.0.1:48081）
```bash
/opt/mem/mem-server -addr 127.0.0.1:48081 -db /opt/mem/mem.db
```

---

## 三、Systemd 守护进程托管（开机自启）

创建服务文件 `/etc/systemd/system/mem-webservice.service`：

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
```

启动并设置开机自启：
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now mem-webservice.service
sudo systemctl status mem-webservice.service
```

---

## 四、Nginx 反向代理配置样例

在 Nginx 的站点配置中（如 `/etc/nginx/conf.d/mem.conf` 或 `/etc/nginx/sites-available/mem.conf`）添加：

```nginx
server {
    listen 80;
    server_name mem.yourdomain.com; # 替换为你的域名或公网 IP

    # 允许传输的单次请求最大体积
    client_max_body_size 10M;

    location / {
        proxy_pass http://127.0.0.1:48081;
        
        # 传递真实请求头
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket / 长连接支持
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        # 禁用反代缓冲，保证极低延迟
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

部署完成后，即可在外网直接通过 `http://mem.yourdomain.com` 访问中转控制台，并在 Android APP 和 Linux 客户端中配置该地址。
