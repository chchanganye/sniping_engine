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
