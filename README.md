# sniping_engine (monorepo)

本仓库采用 Monorepo 结构：

- `frontend/`：Vue3 + Vite 前端
- `backend/`：Go 后端（HTTP + WebSocket + SQLite）

## 开发

### 前端

```bash
cd frontend
npm install
npm run dev
```

### 后端

```bash
cd backend
go run ./cmd/mock
go run ./cmd/server -config ./config.yaml
```

### 通知设置

- 前端「通知设置」页面：配置 SMTP 后，抢购成功会自动发邮件（由 Go 后端发送）。

## Linux 上用 Docker 快速部署

前提：已安装 Docker + Docker Compose（插件版：`docker compose`）。

1) 拉起服务

```bash
docker compose up -d --build
```

2) 访问

- 前端：`http://<你的服务器IP>:8080`
- 后端健康检查：`http://<你的服务器IP>:8090/health`
  - 如果想直接用 80 端口，把 `docker-compose.yml` 里前端端口改成 `80:80`，并在安全组/防火墙放行对应端口（如 `ufw allow 80/tcp`）

3) 配置调整（强烈建议按需修改）

- 后端配置文件使用 `backend/config.docker.yaml`（compose 已挂载到容器内 `/app/config.yaml`）
  - `server.cors.allowOrigins`：改成你的实际访问域名/端口（否则 WebSocket 可能连不上）
  - `proxy.global`：容器里通常不能用 `127.0.0.1:7897` 这种宿主机代理；不需要就留空，或改成可达的代理地址
- SQLite 数据持久化在 compose volume：`backend-data`（容器内路径 `/app/data`）

常用命令：

```bash
docker compose logs -f backend
docker compose logs -f frontend
docker compose down
```

如果构建阶段出现 `go mod download` 超时（常见于无法访问 `proxy.golang.org`）：

- 已在 `backend/Dockerfile` 默认设置：
  - `GOPROXY=https://goproxy.cn,direct`
  - `GOSUMDB=sum.golang.google.cn`
- 若你有自己的网络代理，也可以改成公司/自建的 Go Module 代理地址后重新 `docker compose up -d --build`

如果构建阶段出现 `apt-get update` 超时（常见于无法访问 `deb.debian.org`）：

- 已在 `backend/Dockerfile` 默认使用 `APT_MIRROR=mirrors.aliyun.com`
- 你也可以在 `docker-compose.yml` 的 `backend.build.args.APT_MIRROR` 改成你所在网络可访问的 Debian 镜像源

如果构建阶段出现 `npm` 依赖下载慢/超时：

- 已在 `frontend/Dockerfile` 默认使用 `NPM_REGISTRY=https://registry.npmmirror.com`
